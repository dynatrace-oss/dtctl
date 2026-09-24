package output

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SeriesKind selects how `query --series` renders the numeric arrays of a DQL
// timeseries result.
type SeriesKind int

const (
	// SeriesFull emits every datapoint unchanged (default).
	SeriesFull SeriesKind = iota
	// SeriesSummary replaces each series with per-series statistics and a sparkline.
	SeriesSummary
	// SeriesDownsample reduces each series to at most N points, keeping extremes.
	SeriesDownsample
)

// SeriesMode is a parsed --series value.
type SeriesMode struct {
	Kind   SeriesKind
	Points int // target point budget for SeriesDownsample
}

// summarySparkWidth caps the sparkline in a series summary. Longer series are
// bucketed into this many cells.
const summarySparkWidth = 24

// defaultSummaryDigits is the significant-digit precision of summary
// statistics when --precision is not set.
const defaultSummaryDigits = 3

// ParseSeriesMode parses a --series value: "full", "summary" or "downsample:N"
// with N >= 2. The empty string means full.
func ParseSeriesMode(s string) (SeriesMode, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "", "full":
		return SeriesMode{Kind: SeriesFull}, nil
	case "summary":
		return SeriesMode{Kind: SeriesSummary}, nil
	}
	if rest, ok := strings.CutPrefix(v, "downsample:"); ok {
		n, err := strconv.Atoi(rest)
		if err != nil || n < 2 {
			return SeriesMode{}, fmt.Errorf("invalid --series %q: downsample needs a point count of at least 2, e.g. downsample:30", s)
		}
		return SeriesMode{Kind: SeriesDownsample, Points: n}, nil
	}
	return SeriesMode{}, fmt.Errorf("invalid --series %q (use full, summary or downsample:N)", s)
}

// ApplySeriesMode compacts the numeric series of DQL timeseries records (those
// carrying `timeframe` and `interval`) according to mode, and rounds every
// float in the result — timeseries or not — to `digits` significant digits
// (0 = no rounding; summary statistics default to 3). With tabular set, a
// summary is rendered as one human-readable cell instead of an object, for
// table and CSV output.
//
// Records without timeframe/interval, and arrays that are not purely numeric
// (nulls allowed), pass through unchanged. The input is never mutated.
func ApplySeriesMode(records []map[string]interface{}, mode SeriesMode, digits int, tabular bool) []map[string]interface{} {
	out, _ := ApplySeriesModeWithEffect(records, mode, digits, tabular)
	return out
}

// SeriesEffect reports what ApplySeriesModeWithEffect actually changed, so a
// caller that applied a lossy default can tell the user only when it mattered.
type SeriesEffect struct {
	Summarized bool // at least one series was replaced by a summary
	Rounded    bool // at least one float value changed through rounding
}

// ApplySeriesModeWithEffect is ApplySeriesMode that also reports its effect.
func ApplySeriesModeWithEffect(records []map[string]interface{}, mode SeriesMode, digits int, tabular bool) ([]map[string]interface{}, SeriesEffect) {
	var eff SeriesEffect
	if mode.Kind == SeriesFull {
		return roundRecords(records, digits, &eff.Rounded), eff
	}

	out := make([]map[string]interface{}, len(records))
	for i, rec := range records {
		ts, ok := parseSeriesTimebase(rec)
		if !ok {
			out[i] = rec
			continue
		}
		cp := make(map[string]interface{}, len(rec))
		for k, v := range rec {
			cp[k] = v
		}
		switch mode.Kind {
		case SeriesSummary:
			if summarizeRecord(cp, ts, digits, tabular) {
				eff.Summarized = true
			}
		case SeriesDownsample:
			downsampleRecord(cp, ts, mode.Points)
		}
		out[i] = cp
	}
	return roundRecords(out, digits, &eff.Rounded), eff
}

// seriesTimebase is the time axis shared by all series of one record.
type seriesTimebase struct {
	start    time.Time // zero if the timeframe is not parseable
	interval time.Duration
}

func (tb seriesTimebase) at(idx int) (string, bool) {
	if tb.start.IsZero() || tb.interval <= 0 {
		return "", false
	}
	return tb.start.Add(time.Duration(idx) * tb.interval).UTC().Format(time.RFC3339), true
}

// parseSeriesTimebase reports whether rec is a DQL timeseries record and, if
// so, its start and interval as far as they can be parsed.
func parseSeriesTimebase(rec map[string]interface{}) (seriesTimebase, bool) {
	tf, hasTF := rec["timeframe"].(map[string]interface{})
	raw, hasInterval := rec["interval"]
	if !hasTF || !hasInterval {
		return seriesTimebase{}, false
	}
	var tb seriesTimebase
	if ns, ok := intervalNanos(raw); ok {
		tb.interval = time.Duration(ns)
	}
	if start, _, err := parseTimeframe(tf); err == nil {
		tb.start = start
	}
	return tb, true
}

func intervalNanos(raw interface{}) (int64, bool) {
	switch v := raw.(type) {
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil && n > 0
	case float64:
		return int64(v), v > 0
	case int64:
		return v, v > 0
	case int:
		return int64(v), v > 0
	}
	return 0, false
}

// numericSeries returns the values of a purely numeric array (nil entries
// become NaN). ok is false for anything else, including an empty array.
func numericSeries(v interface{}) ([]float64, bool) {
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		return nil, false
	}
	out := make([]float64, len(arr))
	for i, item := range arr {
		switch n := item.(type) {
		case nil:
			out[i] = math.NaN()
		case float64:
			out[i] = n
		case float32:
			out[i] = float64(n)
		case int:
			out[i] = float64(n)
		case int64:
			out[i] = float64(n)
		case json.Number:
			f, err := n.Float64()
			if err != nil {
				return nil, false
			}
			out[i] = f
		default:
			return nil, false
		}
	}
	return out, true
}

// summarizeRecord replaces every numeric series of rec with its summary and
// reports whether it found any.
func summarizeRecord(rec map[string]interface{}, tb seriesTimebase, digits int, tabular bool) bool {
	found := false
	if digits <= 0 {
		digits = defaultSummaryDigits
	}
	for k, v := range rec {
		if k == "timeframe" || k == "interval" {
			continue
		}
		vals, ok := numericSeries(v)
		if !ok {
			continue
		}
		found = true
		sum := roundValue(summarizeSeries(vals, tb), digits, nil).(map[string]interface{})
		if tabular {
			rec[k] = summaryCell(sum)
		} else {
			rec[k] = sum
		}
	}
	return found
}

// summarizeSeries computes the per-series summary object. Keys that need at
// least one non-null value are omitted for an all-null series.
func summarizeSeries(vals []float64, tb seriesTimebase) map[string]interface{} {
	sum := map[string]interface{}{"n": len(vals)}

	var present []float64
	minIdx, maxIdx, lastIdx := -1, -1, -1
	total := 0.0
	for i, v := range vals {
		if math.IsNaN(v) {
			continue
		}
		present = append(present, v)
		total += v
		if minIdx < 0 || v < vals[minIdx] {
			minIdx = i
		}
		if maxIdx < 0 || v > vals[maxIdx] {
			maxIdx = i
		}
		lastIdx = i
	}
	if nulls := len(vals) - len(present); nulls > 0 {
		sum["nulls"] = nulls
	}
	if len(present) == 0 {
		return sum
	}

	sum["min"] = vals[minIdx]
	sum["max"] = vals[maxIdx]
	sum["avg"] = total / float64(len(present))
	sum["last"] = vals[lastIdx]
	sum["p95"] = percentile(present, 0.95)
	if at, ok := tb.at(minIdx); ok {
		sum["min_at"] = at
	}
	if at, ok := tb.at(maxIdx); ok {
		sum["max_at"] = at
	}
	sum["spark"] = summarySpark(vals, vals[minIdx], vals[maxIdx], total/float64(len(present)))
	if step, ok := detectStep(vals, tb); ok {
		sum["step"] = step
	}
	return sum
}

// percentile uses the nearest-rank method, so the result is always an observed value.
func percentile(present []float64, p float64) float64 {
	sorted := append([]float64(nil), present...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

// summarySpark renders at most summarySparkWidth cells. When a series is
// longer, each cell shows the value in its bucket that lies furthest from the
// series mean, so a one-point spike or dip is not averaged away. A cell with
// no data is a space.
func summarySpark(vals []float64, lo, hi, mean float64) string {
	cells := vals
	if len(vals) > summarySparkWidth {
		cells = make([]float64, summarySparkWidth)
		for b := range cells {
			from := b * len(vals) / summarySparkWidth
			to := (b + 1) * len(vals) / summarySparkWidth
			cells[b] = math.NaN()
			for _, v := range vals[from:to] {
				if math.IsNaN(v) {
					continue
				}
				if math.IsNaN(cells[b]) || math.Abs(v-mean) > math.Abs(cells[b]-mean) {
					cells[b] = v
				}
			}
		}
	}

	var sb strings.Builder
	for _, v := range cells {
		switch {
		case math.IsNaN(v):
			sb.WriteRune(' ')
		case hi == lo:
			sb.WriteRune(sparkChars[len(sparkChars)/2])
		default:
			idx := int(math.Round((v - lo) / (hi - lo) * float64(len(sparkChars)-1)))
			sb.WriteRune(sparkChars[idx])
		}
	}
	return sb.String()
}

// minStepSegment is the fewest non-null points either side of a step.
const minStepSegment = 3

// detectStep looks for a single level shift: the split that best fits the
// series as two constant segments. It is reported only when the jump clearly
// exceeds the variation inside both segments (so a ramp or noise does not
// qualify) and is at least 10% of the larger level.
func detectStep(vals []float64, tb seriesTimebase) (map[string]interface{}, bool) {
	var idx []int
	var xs []float64
	for i, v := range vals {
		if !math.IsNaN(v) {
			idx = append(idx, i)
			xs = append(xs, v)
		}
	}
	n := len(xs)
	if n < 2*minStepSegment {
		return nil, false
	}

	prefix := make([]float64, n+1)
	prefixSq := make([]float64, n+1)
	for i, x := range xs {
		prefix[i+1] = prefix[i] + x
		prefixSq[i+1] = prefixSq[i] + x*x
	}
	sse := func(from, to int) float64 { // sum of squared deviations of xs[from:to]
		cnt := float64(to - from)
		s := prefix[to] - prefix[from]
		return (prefixSq[to] - prefixSq[from]) - s*s/cnt
	}

	best, bestCost := -1, math.Inf(1)
	for k := minStepSegment; k <= n-minStepSegment; k++ {
		if c := sse(0, k) + sse(k, n); c < bestCost {
			best, bestCost = k, c
		}
	}

	m1 := prefix[best] / float64(best)
	m2 := (prefix[n] - prefix[best]) / float64(n-best)
	sd1 := math.Sqrt(math.Max(sse(0, best), 0) / float64(best))
	sd2 := math.Sqrt(math.Max(sse(best, n), 0) / float64(n-best))
	jump := math.Abs(m2 - m1)
	if jump == 0 || jump <= 2*(sd1+sd2) || jump < 0.1*math.Max(math.Abs(m1), math.Abs(m2)) {
		return nil, false
	}

	step := map[string]interface{}{"from": m1, "to": m2}
	if at, ok := tb.at(idx[best]); ok {
		step["at"] = at
	} else {
		step["index"] = idx[best]
	}
	return step, true
}

// summaryCell renders a summary object as one table/CSV cell.
func summaryCell(sum map[string]interface{}) string {
	parts := []string{}
	if spark, ok := sum["spark"].(string); ok {
		parts = append(parts, spark)
	}
	for _, k := range []string{"min", "avg", "max", "last"} {
		if v, ok := sum[k]; ok {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	parts = append(parts, fmt.Sprintf("n=%v", sum["n"]))
	if nulls, ok := sum["nulls"]; ok {
		parts = append(parts, fmt.Sprintf("nulls=%v", nulls))
	}
	return strings.Join(parts, " ")
}

// downsampleRecord reduces every numeric series of rec to at most `points`
// values. The series is split into points/2 equal buckets and each bucket
// contributes its minimum and maximum in time order, so every local extreme
// (and therefore the global min and max) survives. An all-null bucket stays a
// gap. The record's interval is rescaled so the output is still a regular
// timeseries over the same timeframe.
func downsampleRecord(rec map[string]interface{}, tb seriesTimebase, points int) {
	series := map[string][]float64{}
	length := 0
	for k, v := range rec {
		if k == "timeframe" || k == "interval" {
			continue
		}
		if vals, ok := numericSeries(v); ok {
			series[k] = vals
			length = max(length, len(vals))
		}
	}
	if length <= points || len(series) == 0 {
		return
	}

	buckets := points / 2
	width := (length + buckets - 1) / buckets
	for k, vals := range series {
		out := make([]interface{}, 0, points)
		for from := 0; from < length; from += width {
			to := min(from+width, len(vals))
			lo, hi := -1, -1
			for i := from; i < to; i++ {
				if math.IsNaN(vals[i]) {
					continue
				}
				if lo < 0 || vals[i] < vals[lo] {
					lo = i
				}
				if hi < 0 || vals[i] > vals[hi] {
					hi = i
				}
			}
			switch {
			case lo < 0:
				out = append(out, nil, nil)
			case lo <= hi:
				out = append(out, vals[lo], vals[hi])
			default:
				out = append(out, vals[hi], vals[lo])
			}
		}
		rec[k] = out
	}

	if tb.interval > 0 {
		rec["interval"] = strconv.FormatInt(int64(tb.interval)*int64(width)/2, 10)
	}
}

// RoundNumbers returns a copy of records with every float64 (recursively,
// through maps and slices) rounded to `digits` significant digits. digits <= 0
// returns records unchanged. Strings — including DQL longs, which the API
// encodes as strings — are never touched.
func RoundNumbers(records []map[string]interface{}, digits int) []map[string]interface{} {
	return roundRecords(records, digits, nil)
}

// roundRecords is RoundNumbers that sets *changed when any value changed.
func roundRecords(records []map[string]interface{}, digits int, changed *bool) []map[string]interface{} {
	if digits <= 0 {
		return records
	}
	out := make([]map[string]interface{}, len(records))
	for i, r := range records {
		out[i] = roundValue(r, digits, changed).(map[string]interface{})
	}
	return out
}

func roundValue(v interface{}, digits int, changed *bool) interface{} {
	switch t := v.(type) {
	case float64:
		r := roundSignificant(t, digits)
		if changed != nil && r != t {
			*changed = true
		}
		return r
	case map[string]interface{}:
		cp := make(map[string]interface{}, len(t))
		for k, x := range t {
			cp[k] = roundValue(x, digits, changed)
		}
		return cp
	case []interface{}:
		cp := make([]interface{}, len(t))
		for i, x := range t {
			cp[i] = roundValue(x, digits, changed)
		}
		return cp
	case []map[string]interface{}:
		cp := make([]map[string]interface{}, len(t))
		for i, x := range t {
			cp[i] = roundValue(x, digits, changed).(map[string]interface{})
		}
		return cp
	default:
		return v
	}
}

// roundSignificant rounds v to `digits` significant digits, but never into the
// integer part: 12345.678 at 3 digits is 12346, not 12300, so counts and sizes
// keep their magnitude exactly.
func roundSignificant(v float64, digits int) float64 {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	intDigits := int(math.Floor(math.Log10(math.Abs(v)))) + 1
	decimals := max(digits-intDigits, 0)
	if decimals <= 15 {
		// Round half away from zero; dividing by an exact power of ten yields
		// the double closest to the decimal result.
		p := math.Pow10(decimals)
		return math.Round(v*p) / p
	}
	r, err := strconv.ParseFloat(strconv.FormatFloat(v, 'f', decimals, 64), 64)
	if err != nil {
		return v
	}
	return r
}
