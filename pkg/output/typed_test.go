package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestApplyTrueTypes_ScalarCasts(t *testing.T) {
	types := []ColumnTypeMapping{
		{Name: "count", Type: "long"},
		{Name: "dur", Type: "duration"},
		{Name: "ratio", Type: "double"},
		{Name: "ok", Type: "boolean"},
		{Name: "host", Type: "string"},
		{Name: "ts", Type: "timestamp"},
	}
	records := []map[string]interface{}{
		{
			"count": "194414758",            // long as JSON string → int64
			"dur":   "598600",               // duration ns string → int64
			"ratio": float64(0.5),           // double already a number → unchanged
			"ok":    "true",                 // boolean as string → bool
			"host":  "web-01",               // string → unchanged
			"ts":    "2025-03-15T10:30:00Z", // timestamp → left as RFC3339 string
		},
	}
	ApplyTrueTypes(records, types)

	r := records[0]
	if got, ok := r["count"].(int64); !ok || got != 194414758 {
		t.Errorf("count = %#v, want int64(194414758)", r["count"])
	}
	if got, ok := r["dur"].(int64); !ok || got != 598600 {
		t.Errorf("dur = %#v, want int64(598600)", r["dur"])
	}
	if got, ok := r["ratio"].(float64); !ok || got != 0.5 {
		t.Errorf("ratio = %#v, want float64(0.5)", r["ratio"])
	}
	if got, ok := r["ok"].(bool); !ok || got != true {
		t.Errorf("ok = %#v, want bool(true)", r["ok"])
	}
	if r["host"] != "web-01" {
		t.Errorf("host = %#v, want string unchanged", r["host"])
	}
	// Timestamps are intentionally left as RFC3339 strings (JSON has no date type).
	if r["ts"] != "2025-03-15T10:30:00Z" {
		t.Errorf("ts = %#v, want RFC3339 string unchanged", r["ts"])
	}
}

func TestApplyTrueTypes_BigIntPrecisionThroughJSONAndYAML(t *testing.T) {
	// The whole point: a full-range int64 must survive casting AND marshaling
	// unquoted and lossless — never routed through a float64.
	types := []ColumnTypeMapping{{Name: "count", Type: "long"}}
	records := []map[string]interface{}{{"count": "9223372036854775807"}} // math.MaxInt64
	ApplyTrueTypes(records, types)

	var jb bytes.Buffer
	if err := json.NewEncoder(&jb).Encode(records[0]); err != nil {
		t.Fatalf("json encode: %v", err)
	}
	if got, want := jb.String(), "{\"count\":9223372036854775807}\n"; got != want {
		t.Errorf("JSON = %q, want %q (unquoted, lossless)", got, want)
	}

	var yb bytes.Buffer
	ye := yaml.NewEncoder(&yb)
	ye.SetIndent(2)
	if err := ye.Encode(records[0]); err != nil {
		t.Fatalf("yaml encode: %v", err)
	}
	if got, want := yb.String(), "count: 9223372036854775807\n"; got != want {
		t.Errorf("YAML = %q, want %q (unquoted, lossless)", got, want)
	}
}

func TestApplyTrueTypes_NonFiniteDoubleStaysString(t *testing.T) {
	// encoding/json cannot marshal NaN/Inf; a double delivered as "NaN"/"Infinity"
	// must stay a string so the whole output does not fail.
	types := []ColumnTypeMapping{{Name: "d", Type: "double"}}
	for _, s := range []string{"NaN", "Infinity", "-Infinity"} {
		records := []map[string]interface{}{{"d": s}}
		ApplyTrueTypes(records, types)
		if records[0]["d"] != s {
			t.Errorf("double %q was cast to %#v, want left as string", s, records[0]["d"])
		}
		// And it must still be JSON-marshalable.
		if _, err := json.Marshal(records[0]); err != nil {
			t.Errorf("json.Marshal failed for %q: %v", s, err)
		}
	}
}

func TestApplyTrueTypes_LeavesUncastableAlone(t *testing.T) {
	types := []ColumnTypeMapping{
		{Name: "n", Type: "long"},
		{Name: "d", Type: "double"},
		{Name: "b", Type: "boolean"},
		{Name: "nested", Type: "record"},
	}
	records := []map[string]interface{}{
		{
			"n":      "not-a-number",                          // unparseable long → unchanged string
			"d":      "1.2.3",                                 // unparseable double → unchanged string
			"b":      "yes",                                   // not true/false → unchanged string
			"nested": map[string]interface{}{"a": float64(1)}, // complex → unchanged
			"absent": nil,                                     // explicit null → untouched
		},
	}
	ApplyTrueTypes(records, types)
	r := records[0]
	if r["n"] != "not-a-number" {
		t.Errorf("n = %#v, want unchanged", r["n"])
	}
	if r["d"] != "1.2.3" {
		t.Errorf("d = %#v, want unchanged", r["d"])
	}
	if r["b"] != "yes" {
		t.Errorf("b = %#v, want unchanged", r["b"])
	}
	if _, ok := r["nested"].(map[string]interface{}); !ok {
		t.Errorf("nested = %#v, want unchanged map", r["nested"])
	}
	if r["absent"] != nil {
		t.Errorf("absent = %#v, want nil", r["absent"])
	}
}

func TestApplyTrueTypes_LongRejectsFractionalFloat(t *testing.T) {
	// A long delivered as a numeric float must not be truncated: an integral
	// value casts, a fractional one is left untouched.
	types := []ColumnTypeMapping{{Name: "n", Type: "long"}}
	records := []map[string]interface{}{
		{"n": float64(42)},
		{"n": float64(7.5)},
	}
	ApplyTrueTypes(records, types)
	if got, ok := records[0]["n"].(int64); !ok || got != 42 {
		t.Errorf("row0 n = %#v, want int64(42)", records[0]["n"])
	}
	if got, ok := records[1]["n"].(float64); !ok || got != 7.5 {
		t.Errorf("row1 n = %#v, want float64(7.5) left untouched", records[1]["n"])
	}
}

func TestApplyTrueTypes_NoTypesOrNoRecordsIsNoOp(t *testing.T) {
	records := []map[string]interface{}{{"count": "42"}}
	ApplyTrueTypes(records, nil) // no types → no-op
	if records[0]["count"] != "42" {
		t.Errorf("count = %#v, want unchanged string when no types", records[0]["count"])
	}
	ApplyTrueTypes(nil, []ColumnTypeMapping{{Name: "x", Type: "long"}}) // no records → no panic
}

func TestApplyTrueTypes_Idempotent(t *testing.T) {
	types := []ColumnTypeMapping{{Name: "n", Type: "long"}}
	records := []map[string]interface{}{{"n": "42"}}
	ApplyTrueTypes(records, types)
	ApplyTrueTypes(records, types) // second pass: int64 already, must stay int64(42)
	if got, ok := records[0]["n"].(int64); !ok || got != 42 {
		t.Errorf("n = %#v, want stable int64(42) across repeated calls", records[0]["n"])
	}
}
