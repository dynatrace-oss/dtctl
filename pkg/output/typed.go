package output

import (
	"encoding/json"
	"math"
	"strconv"
)

// ApplyTrueTypes rewrites scalar record cells from their wire form into native
// Go scalar types, driven by the DQL column type metadata. It exists because the
// Grail query API deliberately serialises integer-valued columns (`long`,
// `duration`) as JSON strings to preserve int64 precision for JavaScript/
// TypeScript consumers (see GRAIL-545). dtctl's JSON/YAML output faithfully
// passes those strings through, which is honest to the wire but surprises
// jq/pandas/DuckDB users, for whom a `count()` reading `"42"` is a string.
//
// This transform is opt-in (the `--typed` flag). It is precision-safe on the
// dtctl side: a DQL `long` is an int64, and both encoding/json and yaml.v3
// marshal a Go int64 as its full decimal digits, unquoted and lossless — never
// via a lossy float64. The only precision risk lives in a downstream consumer
// that parses JSON numbers as 64-bit floats (browser JSON.parse, old jq), which
// is exactly why the default output stays string-faithful and this is opt-in.
//
// The rewrite is in place: cells are terminal at the point this is called, so no
// second copy of the (potentially large) result set is held. Columns not present
// in types, non-scalar values, nulls, and any value that does not cleanly parse
// to its declared type are left untouched — the transform never fails an export
// and never fabricates a value.
func ApplyTrueTypes(records []map[string]interface{}, types []ColumnTypeMapping) {
	if len(records) == 0 || len(types) == 0 {
		return
	}
	dqlTypes := make(map[string]string, len(types))
	for _, t := range types {
		dqlTypes[t.Name] = t.Type
	}
	for _, rec := range records {
		for name, dqlType := range dqlTypes {
			v, ok := rec[name]
			if !ok || v == nil {
				continue
			}
			if cast, ok := castScalar(v, dqlType); ok {
				rec[name] = cast
			}
		}
	}
}

// castScalar converts a single wire value to the native Go type implied by its
// DQL type. It returns ok=false (leaving the original value in place) for any
// value it cannot losslessly and safely represent — including non-finite doubles,
// which encoding/json cannot marshal and which must therefore stay strings.
//
// Timestamps are intentionally NOT cast: JSON/YAML have no native temporal type,
// and the RFC3339 string is the honest, portable representation. string/ip/
// timeframe and nested record/array values (already real JSON structures) are
// likewise left as-is.
func castScalar(v interface{}, dqlType string) (interface{}, bool) {
	switch dqlType {
	case "long", "duration":
		// Integer-valued. DQL delivers these as JSON strings; the float64 and
		// json.Number arms are defensive (a value decoded as a number still casts
		// cleanly) and never truncate a fractional or non-finite value.
		switch n := v.(type) {
		case string:
			if i, err := strconv.ParseInt(n, 10, 64); err == nil {
				return i, true
			}
		case json.Number:
			if i, err := n.Int64(); err == nil {
				return i, true
			}
		case float64:
			if !math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n) &&
				n >= math.MinInt64 && n < 9223372036854775808.0 {
				return int64(n), true
			}
		}
		return nil, false

	case "double":
		switch n := v.(type) {
		case float64:
			return n, true
		case json.Number:
			if f, err := n.Float64(); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
				return f, true
			}
		case string:
			// A non-finite double (DQL sends "NaN"/"Infinity" as strings) must stay
			// a string: encoding/json refuses to marshal NaN/Inf and would fail the
			// whole output.
			if f, err := strconv.ParseFloat(n, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
				return f, true
			}
		}
		return nil, false

	case "boolean":
		switch b := v.(type) {
		case bool:
			return b, true
		case string:
			if b == "true" {
				return true, true
			}
			if b == "false" {
				return false, true
			}
		}
		return nil, false

	default:
		// string, timestamp, ip, timeframe, record, array, variant, unknown → as-is.
		return nil, false
	}
}
