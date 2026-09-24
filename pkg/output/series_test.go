package output

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// tsRecord builds a DQL timeseries record: one numeric column over a
// one-minute interval starting at 12:00 UTC.
func tsRecord(field string, values ...interface{}) map[string]interface{} {
	return map[string]interface{}{
		"host.name": "web-01",
		field:       values,
		"interval":  "60000000000",
		"timeframe": map[string]interface{}{
			"start": "2026-01-01T12:00:00.000000000Z",
			"end":   "2026-01-01T13:00:00.000000000Z",
		},
	}
}

func floats(vs ...float64) []interface{} {
	out := make([]interface{}, len(vs))
	for i, v := range vs {
		out[i] = v
	}
	return out
}

func TestParseSeriesMode(t *testing.T) {
	valid := map[string]SeriesMode{
		"":              {Kind: SeriesFull},
		"full":          {Kind: SeriesFull},
		"summary":       {Kind: SeriesSummary},
		"downsample:30": {Kind: SeriesDownsample, Points: 30},
		"downsample:2":  {Kind: SeriesDownsample, Points: 2},
		"Summary":       {Kind: SeriesSummary},
	}
	for in, want := range valid {
		got, err := ParseSeriesMode(in)
		if err != nil {
			t.Errorf("ParseSeriesMode(%q) unexpected error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseSeriesMode(%q) = %+v, want %+v", in, got, want)
		}
	}

	for _, in := range []string{"bogus", "downsample", "downsample:", "downsample:1", "downsample:0", "downsample:-4", "downsample:x", "summary:3"} {
		if _, err := ParseSeriesMode(in); err == nil {
			t.Errorf("ParseSeriesMode(%q) expected an error", in)
		}
	}
}

func TestRoundSignificant(t *testing.T) {
	tests := []struct {
		in     float64
		digits int
		want   float64
	}{
		{3.10276124773992, 3, 3.1},
		{84.52341, 3, 84.5},
		{409.3, 3, 409},
		// The integer part is never rounded away: counts and byte sizes keep
		// their magnitude exactly.
		{12345.678, 3, 12346},
		{194414758, 3, 194414758},
		{0.0012345, 3, 0.00123},
		{-15.749, 3, -15.7},
		{0, 3, 0},
		{2.5, 1, 3},
		{1.23456, 5, 1.2346},
	}
	for _, tt := range tests {
		if got := roundSignificant(tt.in, tt.digits); got != tt.want {
			t.Errorf("roundSignificant(%v, %d) = %v, want %v", tt.in, tt.digits, got, tt.want)
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := roundSignificant(v, 3); !(math.IsNaN(v) && math.IsNaN(got)) && got != v {
			t.Errorf("roundSignificant(%v) = %v, want unchanged", v, got)
		}
	}
}

func TestRoundNumbers_RecursesAndLeavesNonFloatsAlone(t *testing.T) {
	in := []map[string]interface{}{{
		"avg":     84.52341,
		"count":   "194414758", // DQL long delivered as a string: never touched
		"name":    "web-01",
		"series":  []interface{}{1.23456, nil, 2.0},
		"nested":  map[string]interface{}{"p": 0.0012345},
		"records": []map[string]interface{}{{"x": 9.87654}},
	}}
	got := RoundNumbers(in, 3)

	want := map[string]interface{}{
		"avg":     84.5,
		"count":   "194414758",
		"name":    "web-01",
		"series":  []interface{}{1.23, nil, 2.0},
		"nested":  map[string]interface{}{"p": 0.00123},
		"records": []map[string]interface{}{{"x": 9.88}},
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("RoundNumbers =\n%#v\nwant\n%#v", got[0], want)
	}
	if in[0]["avg"] != 84.52341 {
		t.Errorf("RoundNumbers mutated its input: %v", in[0]["avg"])
	}
	if got := RoundNumbers(in, 0); !reflect.DeepEqual(got, in) {
		t.Errorf("digits=0 must be a no-op")
	}
}

func TestApplySeriesMode_FullIsNoOp(t *testing.T) {
	in := []map[string]interface{}{tsRecord("cpu", floats(1.123456, 2.2)...)}
	got := ApplySeriesMode(in, SeriesMode{Kind: SeriesFull}, 0, false)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("full mode changed records: %#v", got)
	}
}

func TestApplySeriesMode_Summary(t *testing.T) {
	// 10 points with a null gap; the peak is at index 7 (12:07), the trough at
	// index 1 (12:01), and the last value is a null so "last" must skip it.
	rec := tsRecord("cpu", 10.0, 2.0, 11.0, nil, 12.0, 9.0, 10.0, 409.3333, 11.0, nil)
	got := ApplySeriesMode([]map[string]interface{}{rec}, SeriesMode{Kind: SeriesSummary}, 0, false)

	sum, ok := got[0]["cpu"].(map[string]interface{})
	if !ok {
		t.Fatalf("cpu should be a summary object, got %#v", got[0]["cpu"])
	}
	checks := map[string]interface{}{
		"n":      10,
		"nulls":  2,
		"min":    2.0,
		"max":    409.0, // rounded to 3 significant digits by default
		"avg":    59.3,  // (10+2+11+12+9+10+409.3333+11)/8 = 59.29
		"last":   11.0,
		"p95":    409.0,
		"min_at": "2026-01-01T12:01:00Z",
		"max_at": "2026-01-01T12:07:00Z",
	}
	for k, want := range checks {
		if !reflect.DeepEqual(sum[k], want) {
			t.Errorf("summary[%q] = %#v, want %#v", k, sum[k], want)
		}
	}
	spark, _ := sum["spark"].(string)
	if utf8.RuneCountInString(spark) != 10 {
		t.Errorf("spark should have one cell per point for short series, got %q", spark)
	}
	runes := []rune(spark)
	if runes[7] != '█' || runes[1] != '▁' || runes[3] != ' ' || runes[9] != ' ' {
		t.Errorf("spark should peak at the max, bottom at the min and gap on nulls, got %q", spark)
	}

	// Dimensions and timeseries metadata stay; the input is not mutated.
	if got[0]["host.name"] != "web-01" || got[0]["interval"] != "60000000000" || got[0]["timeframe"] == nil {
		t.Errorf("non-series fields must be preserved: %#v", got[0])
	}
	if _, ok := rec["cpu"].([]interface{}); !ok {
		t.Errorf("ApplySeriesMode mutated its input")
	}
}

func TestApplySeriesMode_SummaryPrecisionOverride(t *testing.T) {
	rec := tsRecord("cpu", floats(1.23456, 2.34567)...)
	got := ApplySeriesMode([]map[string]interface{}{rec}, SeriesMode{Kind: SeriesSummary}, 5, false)
	sum := got[0]["cpu"].(map[string]interface{})
	if sum["max"] != 2.3457 {
		t.Errorf("--precision should override the summary default, got max=%v", sum["max"])
	}
}

func TestApplySeriesMode_SummaryLongSeriesSparkIsBounded(t *testing.T) {
	vals := make([]interface{}, 121)
	for i := range vals {
		vals[i] = float64(i % 7)
	}
	vals[60] = 1000.0 // a single-point spike must survive the spark's bucketing
	got := ApplySeriesMode([]map[string]interface{}{tsRecord("cpu", vals...)}, SeriesMode{Kind: SeriesSummary}, 0, false)
	spark := got[0]["cpu"].(map[string]interface{})["spark"].(string)
	if n := utf8.RuneCountInString(spark); n > summarySparkWidth {
		t.Errorf("spark has %d cells, want at most %d", n, summarySparkWidth)
	}
	if !strings.ContainsRune(spark, '█') {
		t.Errorf("a one-point spike was averaged out of the spark: %q", spark)
	}
}

func TestApplySeriesMode_SummaryAllNull(t *testing.T) {
	got := ApplySeriesMode([]map[string]interface{}{tsRecord("cpu", nil, nil, nil)}, SeriesMode{Kind: SeriesSummary}, 0, false)
	sum := got[0]["cpu"].(map[string]interface{})
	if sum["n"] != 3 || sum["nulls"] != 3 {
		t.Errorf("all-null series: n/nulls wrong: %#v", sum)
	}
	for _, k := range []string{"min", "max", "avg", "p95", "last", "spark", "step"} {
		if _, ok := sum[k]; ok {
			t.Errorf("all-null series should carry no %q, got %#v", k, sum)
		}
	}
}

func TestApplySeriesMode_SummaryStep(t *testing.T) {
	step := make([]interface{}, 0, 20)
	for i := 0; i < 20; i++ {
		v := 10.0 + float64(i%2)*0.2
		if i >= 12 {
			v = 40.0 + float64(i%2)*0.2
		}
		step = append(step, v)
	}
	got := ApplySeriesMode([]map[string]interface{}{tsRecord("cpu", step...)}, SeriesMode{Kind: SeriesSummary}, 0, false)
	s, ok := got[0]["cpu"].(map[string]interface{})["step"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected a step hint for a level shift, got %#v", got[0]["cpu"])
	}
	if s["at"] != "2026-01-01T12:12:00Z" || s["from"] != 10.1 || s["to"] != 40.1 {
		t.Errorf("step = %#v, want at 12:12 from 10.1 to 40.1", s)
	}

	noStep := map[string][]interface{}{
		"ramp":  {},
		"flat":  {},
		"noise": floats(10, 12, 9, 11, 10, 13, 9, 10, 12, 11, 10, 9),
		"short": floats(1, 100),
	}
	for i := 0; i < 20; i++ {
		noStep["ramp"] = append(noStep["ramp"], float64(i*5))
		noStep["flat"] = append(noStep["flat"], 7.0)
	}
	for name, vals := range noStep {
		got := ApplySeriesMode([]map[string]interface{}{tsRecord("cpu", vals...)}, SeriesMode{Kind: SeriesSummary}, 0, false)
		if s, ok := got[0]["cpu"].(map[string]interface{})["step"]; ok {
			t.Errorf("%s: unexpected step hint %#v", name, s)
		}
	}
}

func TestApplySeriesMode_SummaryWithoutTimeframeOmitsTimestamps(t *testing.T) {
	rec := map[string]interface{}{"cpu": floats(1, 5, 2), "interval": "60000000000", "timeframe": map[string]interface{}{}}
	got := ApplySeriesMode([]map[string]interface{}{rec}, SeriesMode{Kind: SeriesSummary}, 0, false)
	sum := got[0]["cpu"].(map[string]interface{})
	if sum["max"] != 5.0 {
		t.Fatalf("stats should still be computed: %#v", sum)
	}
	if _, ok := sum["max_at"]; ok {
		t.Errorf("max_at needs a resolvable timeframe, got %#v", sum)
	}
}

func TestApplySeriesMode_LeavesNonTimeseriesAlone(t *testing.T) {
	in := []map[string]interface{}{
		// A fetch/summarize result: arrays, but no timeframe+interval.
		{"hosts": []interface{}{1.0, 2.0, 3.0}, "avg": 3.14159},
		// A timeseries record with a non-numeric array dimension.
		func() map[string]interface{} {
			r := tsRecord("cpu", floats(1, 2)...)
			r["tags"] = []interface{}{"a", "b"}
			return r
		}(),
	}
	got := ApplySeriesMode(in, SeriesMode{Kind: SeriesSummary}, 0, false)
	if !reflect.DeepEqual(got[0], in[0]) {
		t.Errorf("non-timeseries record changed: %#v", got[0])
	}
	if !reflect.DeepEqual(got[1]["tags"], []interface{}{"a", "b"}) {
		t.Errorf("non-numeric array changed: %#v", got[1]["tags"])
	}
	if _, ok := got[1]["cpu"].(map[string]interface{}); !ok {
		t.Errorf("numeric series next to it should still be summarized")
	}
}

func TestApplySeriesMode_SummaryTabular(t *testing.T) {
	rec := tsRecord("cpu", floats(15.7, 84.5, 409, 36.3)...)
	got := ApplySeriesMode([]map[string]interface{}{rec}, SeriesMode{Kind: SeriesSummary}, 0, true)
	s, ok := got[0]["cpu"].(string)
	if !ok {
		t.Fatalf("tabular summary should be a single cell string, got %#v", got[0]["cpu"])
	}
	for _, part := range []string{"min=15.7", "avg=136", "max=409", "last=36.3", "▁"} {
		if !strings.Contains(s, part) {
			t.Errorf("tabular summary %q missing %q", s, part)
		}
	}
}

func TestApplySeriesMode_Downsample(t *testing.T) {
	// 121 points: a slow wave with a single-point spike and a single-point dip
	// in the same neighborhood, plus a null gap.
	vals := make([]interface{}, 121)
	for i := range vals {
		vals[i] = 50.0 + float64(i%10)
	}
	vals[33] = 999.0
	vals[35] = -5.0
	vals[80] = nil
	rec := tsRecord("cpu", vals...)
	rec["mem"] = append([]interface{}(nil), vals...)

	got := ApplySeriesMode([]map[string]interface{}{rec}, SeriesMode{Kind: SeriesDownsample, Points: 30}, 0, false)
	out := got[0]["cpu"].([]interface{})
	if len(out) > 30 || len(out) < 20 {
		t.Fatalf("downsample:30 produced %d points", len(out))
	}
	var sawMax, sawMin bool
	for _, v := range out {
		switch v {
		case 999.0:
			sawMax = true
		case -5.0:
			sawMin = true
		}
	}
	if !sawMax || !sawMin {
		t.Errorf("downsampling must keep both extremes, got %v", out)
	}
	// Extremes stay in time order: the spike (idx 33) precedes the dip (idx 35).
	var iMax, iMin int
	for i, v := range out {
		if v == 999.0 {
			iMax = i
		}
		if v == -5.0 {
			iMin = i
		}
	}
	if iMax > iMin {
		t.Errorf("extremes out of time order: max at %d, min at %d", iMax, iMin)
	}
	// 121 points in buckets of 9 source points (15 buckets would fit 30, 14 are
	// needed) -> 2 points per bucket, so each output point spans 4.5 minutes.
	if got[0]["interval"] != "270000000000" {
		t.Errorf("interval should be rescaled to the output spacing, got %v", got[0]["interval"])
	}
	if len(got[0]["mem"].([]interface{})) != len(out) {
		t.Errorf("all series in a record must share one length")
	}
	if rec["interval"] != "60000000000" || len(rec["cpu"].([]interface{})) != 121 {
		t.Errorf("downsample mutated its input")
	}
}

func TestApplySeriesMode_DownsampleNullBuckets(t *testing.T) {
	vals := floats(1, 2, 3, 4, 5, 6, 7, 8)
	vals[2], vals[3] = nil, nil
	got := ApplySeriesMode([]map[string]interface{}{tsRecord("cpu", vals...)}, SeriesMode{Kind: SeriesDownsample, Points: 4}, 0, false)
	// 8 points into 2 buckets of 4: [1 2 nil nil] -> 1,2 ; [5 6 7 8] -> 5,8.
	want := []interface{}{1.0, 2.0, 5.0, 8.0}
	if !reflect.DeepEqual(got[0]["cpu"], want) {
		t.Errorf("got %v, want %v", got[0]["cpu"], want)
	}

	allNull := []interface{}{nil, nil, nil, nil, 1.0, 2.0, 3.0, 4.0}
	got = ApplySeriesMode([]map[string]interface{}{tsRecord("cpu", allNull...)}, SeriesMode{Kind: SeriesDownsample, Points: 4}, 0, false)
	want = []interface{}{nil, nil, 1.0, 4.0}
	if !reflect.DeepEqual(got[0]["cpu"], want) {
		t.Errorf("an all-null bucket should stay a gap: got %v, want %v", got[0]["cpu"], want)
	}
}

func TestApplySeriesMode_DownsampleShortSeriesUnchanged(t *testing.T) {
	in := []map[string]interface{}{tsRecord("cpu", floats(1, 2, 3)...)}
	got := ApplySeriesMode(in, SeriesMode{Kind: SeriesDownsample, Points: 10}, 0, false)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("a series already within the budget must not change: %#v", got)
	}
}

func TestApplySeriesMode_DownsampleWithPrecision(t *testing.T) {
	got := ApplySeriesMode([]map[string]interface{}{tsRecord("cpu", floats(1.23456, 2.34567, 3.45678, 4.56789)...)}, SeriesMode{Kind: SeriesDownsample, Points: 2}, 3, false)
	want := []interface{}{1.23, 4.57}
	if !reflect.DeepEqual(got[0]["cpu"], want) {
		t.Errorf("got %v, want %v", got[0]["cpu"], want)
	}
}

// --precision is result-wide by design: a plain fetch/summarize record (no
// timeframe/interval) is rounded too, while --series leaves it alone.
func TestApplySeriesMode_PrecisionAppliesToNonTimeseriesRecords(t *testing.T) {
	in := []map[string]interface{}{{"avg": 3.14159, "host": "web-01", "count": "42"}}
	for _, mode := range []SeriesMode{{Kind: SeriesFull}, {Kind: SeriesSummary}, {Kind: SeriesDownsample, Points: 4}} {
		got := ApplySeriesMode(in, mode, 3, false)
		want := map[string]interface{}{"avg": 3.14, "host": "web-01", "count": "42"}
		if !reflect.DeepEqual(got[0], want) {
			t.Errorf("mode %+v: got %#v, want %#v", mode, got[0], want)
		}
	}
	if in[0]["avg"] != 3.14159 {
		t.Errorf("input mutated")
	}
}

func TestApplySeriesModeWithEffect_ReportsWhatChanged(t *testing.T) {
	ts := []map[string]interface{}{tsRecord("cpu", floats(1.23456, 2.0)...)}
	exact := []map[string]interface{}{{"avg": 2.5, "count": "42"}}
	noisy := []map[string]interface{}{{"avg": 3.14159}}

	tests := []struct {
		name   string
		in     []map[string]interface{}
		mode   SeriesMode
		digits int
		want   SeriesEffect
	}{
		{"summary of a timeseries", ts, SeriesMode{Kind: SeriesSummary}, 4, SeriesEffect{Summarized: true}},
		{"summary with no timeseries, exact floats", exact, SeriesMode{Kind: SeriesSummary}, 4, SeriesEffect{}},
		{"rounding a noisy float", noisy, SeriesMode{Kind: SeriesFull}, 4, SeriesEffect{Rounded: true}},
		{"rounding that changes nothing", exact, SeriesMode{Kind: SeriesFull}, 4, SeriesEffect{}},
		{"rounding off", noisy, SeriesMode{Kind: SeriesFull}, 0, SeriesEffect{}},
		{"downsample within budget", ts, SeriesMode{Kind: SeriesDownsample, Points: 10}, 0, SeriesEffect{}},
		{"rounding raw series points", ts, SeriesMode{Kind: SeriesFull}, 3, SeriesEffect{Rounded: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := ApplySeriesModeWithEffect(tt.in, tt.mode, tt.digits, false)
			if got != tt.want {
				t.Errorf("effect = %+v, want %+v", got, tt.want)
			}
		})
	}
}
