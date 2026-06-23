package output

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Default knobs for the lean Layer-1 stats (D14). Kept small and Go-computed;
// richer profiling (quantiles, wide top-K) is deferred to the Layer 2
// `inspect --stats` primitive.
const (
	// DefaultStatsTopK is how many top values are reported for low-cardinality
	// string columns.
	DefaultStatsTopK = 10
	// DefaultStatsMaxDistinct bounds exact distinct tracking per column. Above
	// this a column is flagged high-cardinality and exact distinct/top are
	// dropped (the buffered Layer-1 path stays O(distinct) per column, which is
	// bounded by this constant).
	DefaultStatsMaxDistinct = 1000
	// DefaultSampleRows is how many leading rows are embedded in the manifest.
	DefaultSampleRows = 3
)

// Column type discriminators reported in stats. Mirrors the lean set in D14;
// anything non-scalar (nested record/array) or type-mixed is reported as
// "complex" and skips min/max so we never crash on a variant column.
const (
	colTypeNull      = "null"
	colTypeBoolean   = "boolean"
	colTypeLong      = "long"
	colTypeDouble    = "double"
	colTypeString    = "string"
	colTypeTimestamp = "timestamp"
	colTypeComplex   = "complex"
)

// TopValue is one entry of a low-cardinality column's top-K list.
type TopValue struct {
	V interface{} `json:"v"`
	N int         `json:"n"`
}

// ColumnStats is the lean per-column profile embedded in a spill manifest.
// Optional numeric fields are pointers/interfaces so they are omitted (rather
// than emitted as a misleading zero) for columns where they don't apply.
type ColumnStats struct {
	Name            string      `json:"name"`
	Type            string      `json:"type"`
	Nulls           int         `json:"nulls"`
	Distinct        *int        `json:"distinct,omitempty"`
	HighCardinality bool        `json:"high_cardinality,omitempty"`
	Min             interface{} `json:"min,omitempty"`
	Max             interface{} `json:"max,omitempty"`
	Mean            *float64    `json:"mean,omitempty"`
	Top             []TopValue  `json:"top,omitempty"`
	// Basis is set to "sample" when the underlying result was sampled by Grail
	// (D23) so an agent reading a column figure cannot miss that it is a
	// sample-based estimate, not a population truth.
	Basis string `json:"basis,omitempty"`
}

// columnAccumulator folds records for a single column in one pass.
type columnAccumulator struct {
	name string

	count int // non-null observations
	nulls int

	// type tracking: the set of scalar kinds seen. If more than one distinct
	// scalar kind appears (ignoring null), or any nested value appears, the
	// column is reported as complex.
	sawBool     bool
	sawNumber   bool
	sawString   bool
	sawComplex  bool
	allIntegral bool // numbers only: true while every number seen is integral

	// numeric min/max/sum over finite values only (NaN/Inf are excluded from
	// aggregates so they can never poison mean/min/max or break JSON marshalling).
	haveNum  bool
	numCount int
	numMin   float64
	numMax   float64
	numSum   float64

	// string handling: distinct counts (bounded by maxDistinct) and timestamp
	// detection. A column of strings that all parse as RFC3339 is reported as a
	// timestamp column. (Plain strings report distinct/top-K, not min/max — D14
	// scopes min/max to numerics & timestamps — so no lexical min/max is kept.)
	distinct      map[string]int
	overflowed    bool // exceeded maxDistinct -> high cardinality
	allTimestamps bool // strings only: true while every string parses as a timestamp
	tsMin         time.Time
	tsMax         time.Time
	haveStr       bool
}

func newColumnAccumulator(name string) *columnAccumulator {
	return &columnAccumulator{
		name:          name,
		allIntegral:   true,
		allTimestamps: true,
		distinct:      make(map[string]int),
	}
}

// ComputeColumnStats computes the lean Layer-1 column profile over the buffered
// records in a single pass (PR2 is buffered; the same accumulator shape is what
// PR3 turns into online sketches). Columns are reported in deterministic
// (alphabetical) order. When sampled is true every column carries
// basis:"sample" per D23.
func ComputeColumnStats(records []map[string]interface{}, sampled bool, topK, maxDistinct int) []ColumnStats {
	if topK <= 0 {
		topK = DefaultStatsTopK
	}
	if maxDistinct <= 0 {
		maxDistinct = DefaultStatsMaxDistinct
	}

	accs := make(map[string]*columnAccumulator)
	var order []string
	ensure := func(name string) *columnAccumulator {
		acc, ok := accs[name]
		if !ok {
			acc = newColumnAccumulator(name)
			accs[name] = acc
			order = append(order, name)
		}
		return acc
	}

	for _, rec := range records {
		for name, val := range rec {
			ensure(name).observe(val, maxDistinct)
		}
	}
	// A record that lacks a column entirely counts as a null for that column.
	// Back-fill those missing observations: a column seen in `count+nulls`
	// records is implicitly null in the remaining `len(records)-(count+nulls)`.
	for _, acc := range accs {
		seen := acc.count + acc.nulls
		if missing := len(records) - seen; missing > 0 {
			acc.nulls += missing
		}
	}

	sort.Strings(order)
	out := make([]ColumnStats, 0, len(order))
	for _, name := range order {
		out = append(out, accs[name].finalize(sampled, topK))
	}
	return out
}

func (a *columnAccumulator) observe(val interface{}, maxDistinct int) {
	if val == nil {
		a.nulls++
		return
	}
	a.count++

	switch v := val.(type) {
	case bool:
		a.sawBool = true
	case float64:
		a.observeNumber(v)
	case int:
		a.observeNumber(float64(v))
	case int64:
		a.observeNumber(float64(v))
	case int32:
		a.observeNumber(float64(v))
	case float32:
		a.observeNumber(float64(v))
	case string:
		a.sawString = true
		a.observeString(v, maxDistinct)
	default:
		// maps, slices, and anything else are non-scalar.
		a.sawComplex = true
	}
}

func (a *columnAccumulator) observeNumber(v float64) {
	a.sawNumber = true
	// NaN/Inf are still numbers (so the column stays numeric) but are excluded
	// from min/max/sum/mean: folding them in poisons the mean to NaN, makes max
	// +Inf, and — fatally — encoding/json rejects NaN/Inf, which would error the
	// whole envelope emit.
	if math.IsInf(v, 0) || math.IsNaN(v) {
		a.allIntegral = false
		return
	}
	if v != math.Trunc(v) {
		a.allIntegral = false
	}
	if a.haveNum {
		if v < a.numMin {
			a.numMin = v
		}
		if v > a.numMax {
			a.numMax = v
		}
	} else {
		a.numMin, a.numMax = v, v
		a.haveNum = true
	}
	a.numSum += v
	a.numCount++
}

func (a *columnAccumulator) observeString(s string, maxDistinct int) {
	// timestamp detection
	if a.allTimestamps {
		if t, err := parseTimestamp(s); err == nil {
			if a.haveStr {
				if t.Before(a.tsMin) {
					a.tsMin = t
				}
				if t.After(a.tsMax) {
					a.tsMax = t
				}
			} else {
				a.tsMin, a.tsMax = t, t
			}
		} else {
			a.allTimestamps = false
		}
	}
	a.haveStr = true

	// bounded distinct tracking
	if !a.overflowed {
		if _, ok := a.distinct[s]; !ok && len(a.distinct) >= maxDistinct {
			a.overflowed = true
			a.distinct = nil // drop exact tracking to bound memory
		} else if !a.overflowed {
			a.distinct[s]++
		}
	}
}

func (a *columnAccumulator) finalize(sampled bool, topK int) ColumnStats {
	cs := ColumnStats{Name: a.name, Nulls: a.nulls}
	if sampled {
		cs.Basis = "sample"
	}

	cs.Type = a.resolveType()

	switch cs.Type {
	case colTypeLong, colTypeDouble:
		// Only finite values contribute; a column of all-NaN/Inf has numCount==0
		// and reports no min/max/mean rather than fabricated figures.
		if a.haveNum && a.numCount > 0 {
			cs.Min = numForType(cs.Type, a.numMin)
			cs.Max = numForType(cs.Type, a.numMax)
			mean := a.numSum / float64(a.numCount)
			cs.Mean = &mean
		}
	case colTypeTimestamp:
		if a.haveStr {
			cs.Min = a.tsMin.UTC().Format(time.RFC3339)
			cs.Max = a.tsMax.UTC().Format(time.RFC3339)
		}
	case colTypeString:
		if a.overflowed {
			cs.HighCardinality = true
		} else {
			d := len(a.distinct)
			cs.Distinct = &d
			cs.Top = a.topValues(topK)
		}
	case colTypeBoolean, colTypeComplex, colTypeNull:
		// booleans: no min/max/mean; complex: skip everything (never crash);
		// null: column was entirely null.
	}
	return cs
}

// resolveType collapses the observed kinds to a single reported type. Mixed
// scalar kinds (or any nested value) report complex so downstream consumers and
// min/max logic never see an ambiguous column.
func (a *columnAccumulator) resolveType() string {
	if a.count == 0 {
		return colTypeNull
	}
	kinds := 0
	if a.sawBool {
		kinds++
	}
	if a.sawNumber {
		kinds++
	}
	if a.sawString {
		kinds++
	}
	if a.sawComplex || kinds > 1 {
		return colTypeComplex
	}
	switch {
	case a.sawBool:
		return colTypeBoolean
	case a.sawNumber:
		if a.allIntegral {
			return colTypeLong
		}
		return colTypeDouble
	case a.sawString:
		if a.allTimestamps {
			return colTypeTimestamp
		}
		return colTypeString
	default:
		return colTypeComplex
	}
}

func (a *columnAccumulator) topValues(topK int) []TopValue {
	if len(a.distinct) == 0 {
		return nil
	}
	tv := make([]TopValue, 0, len(a.distinct))
	for v, n := range a.distinct {
		tv = append(tv, TopValue{V: v, N: n})
	}
	sort.Slice(tv, func(i, j int) bool {
		if tv[i].N != tv[j].N {
			return tv[i].N > tv[j].N
		}
		// tie-break on value for determinism
		return less(tv[i].V, tv[j].V)
	})
	if len(tv) > topK {
		tv = tv[:topK]
	}
	return tv
}

// maxExactInt is the largest magnitude at which a float64 represents every
// integer exactly (2^53). Beyond it, converting to int64 would either lose
// precision or, past int64's range, overflow to a fabricated value — so we keep
// the float64 form instead.
const maxExactInt = float64(1 << 53)

// numForType renders a numeric min/max as an int64 for long columns (so the
// JSON shows 500 not 500.0) and as float64 for double columns. Integral values
// outside the exactly-representable range stay float64 to avoid int64
// overflow/precision loss.
func numForType(t string, f float64) interface{} {
	if t == colTypeLong && f >= -maxExactInt && f <= maxExactInt {
		return int64(f)
	}
	return f
}

// less orders top-K tie-breaks deterministically. Values are strings today, but
// fall back to a stringified compare so the ordering is stable for any type.
func less(a, b interface{}) bool {
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return as < bs
	}
	return fmt.Sprintf("%v", a) < fmt.Sprintf("%v", b)
}

// parseTimestamp accepts the timestamp encodings DQL commonly emits.
func parseTimestamp(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04Z07:00",
	}
	var lastErr error
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

// SampleRows returns up to n leading records as the manifest's sample_rows,
// sanitised so they can always be JSON-encoded into the envelope. A single
// non-finite float (NaN/±Inf) in a sampled value would otherwise be rejected by
// encoding/json and fail the whole envelope emit — the same hazard the column
// stats already guard against (see observeNumber). The originals are never
// mutated (the file writer serialises the full untouched rows).
func SampleRows(records []map[string]interface{}, n int) []map[string]interface{} {
	if n <= 0 {
		n = DefaultSampleRows
	}
	if len(records) < n {
		n = len(records)
	}
	out := make([]map[string]interface{}, n)
	for i := 0; i < n; i++ {
		out[i], _ = sanitizeForJSON(records[i]).(map[string]interface{})
	}
	return out
}

// sanitizeForJSON returns a copy of v with every non-finite float (NaN, +Inf,
// -Inf) — which encoding/json refuses to marshal — replaced by nil, recursing
// into nested records and arrays. Values with nothing to fix are returned as-is.
func sanitizeForJSON(v interface{}) interface{} {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil
		}
		return x
	case float32:
		if f := float64(x); math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return x
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, val := range x {
			out[k] = sanitizeForJSON(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, val := range x {
			out[i] = sanitizeForJSON(val)
		}
		return out
	default:
		return v
	}
}
