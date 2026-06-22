package output

import (
	"encoding/json"
	"math"
	"testing"
)

func findCol(cols []ColumnStats, name string) (ColumnStats, bool) {
	for _, c := range cols {
		if c.Name == name {
			return c, true
		}
	}
	return ColumnStats{}, false
}

func TestComputeColumnStats_Types(t *testing.T) {
	records := []map[string]interface{}{
		{"host": "web-01", "status": float64(200), "ratio": 1.5, "ok": true, "ts": "2026-06-21T08:00:00Z"},
		{"host": "web-01", "status": float64(500), "ratio": 2.5, "ok": false, "ts": "2026-06-22T09:00:00Z"},
		{"host": "web-02", "status": float64(404), "ratio": 3.0, "ok": true, "ts": "2026-06-20T07:00:00Z"},
	}

	cols := ComputeColumnStats(records, false, DefaultStatsTopK, DefaultStatsMaxDistinct)

	// deterministic alphabetical order
	wantOrder := []string{"host", "ok", "ratio", "status", "ts"}
	if len(cols) != len(wantOrder) {
		t.Fatalf("got %d columns, want %d", len(cols), len(wantOrder))
	}
	for i, n := range wantOrder {
		if cols[i].Name != n {
			t.Fatalf("column[%d] = %q, want %q (order not deterministic)", i, cols[i].Name, n)
		}
	}

	host, _ := findCol(cols, "host")
	if host.Type != colTypeString {
		t.Errorf("host type = %q, want string", host.Type)
	}
	if host.Distinct == nil || *host.Distinct != 2 {
		t.Errorf("host distinct = %v, want 2", host.Distinct)
	}
	if len(host.Top) == 0 || host.Top[0].V != "web-01" || host.Top[0].N != 2 {
		t.Errorf("host top = %v, want web-01:2 first", host.Top)
	}

	status, _ := findCol(cols, "status")
	if status.Type != colTypeLong {
		t.Errorf("status type = %q, want long", status.Type)
	}
	if status.Min != int64(200) || status.Max != int64(500) {
		t.Errorf("status min/max = %v/%v, want 200/500", status.Min, status.Max)
	}
	if status.Mean == nil {
		t.Fatal("status mean is nil")
	}
	if got := *status.Mean; got < 367.9 || got > 368.1 {
		t.Errorf("status mean = %v, want ~368", got)
	}

	ratio, _ := findCol(cols, "ratio")
	if ratio.Type != colTypeDouble {
		t.Errorf("ratio type = %q, want double", ratio.Type)
	}

	ok, _ := findCol(cols, "ok")
	if ok.Type != colTypeBoolean {
		t.Errorf("ok type = %q, want boolean", ok.Type)
	}
	if ok.Min != nil || ok.Max != nil || ok.Mean != nil {
		t.Errorf("boolean column should not carry min/max/mean")
	}

	ts, _ := findCol(cols, "ts")
	if ts.Type != colTypeTimestamp {
		t.Errorf("ts type = %q, want timestamp", ts.Type)
	}
	if ts.Min != "2026-06-20T07:00:00Z" || ts.Max != "2026-06-22T09:00:00Z" {
		t.Errorf("ts min/max = %v/%v", ts.Min, ts.Max)
	}
}

func TestComputeColumnStats_NullsAndMissing(t *testing.T) {
	records := []map[string]interface{}{
		{"a": "x", "b": nil},
		{"a": "y"}, // b missing entirely -> counts as null
		{"b": "z"}, // a missing
	}
	cols := ComputeColumnStats(records, false, DefaultStatsTopK, DefaultStatsMaxDistinct)

	a, _ := findCol(cols, "a")
	if a.Nulls != 1 {
		t.Errorf("a nulls = %d, want 1 (one record missing a)", a.Nulls)
	}
	b, _ := findCol(cols, "b")
	if b.Nulls != 2 {
		t.Errorf("b nulls = %d, want 2 (one explicit null + one missing)", b.Nulls)
	}
}

func TestComputeColumnStats_ComplexAndMixed(t *testing.T) {
	records := []map[string]interface{}{
		{"nested": map[string]interface{}{"k": "v"}, "mixed": "s"},
		{"nested": []interface{}{1, 2}, "mixed": float64(3)},
	}
	cols := ComputeColumnStats(records, false, DefaultStatsTopK, DefaultStatsMaxDistinct)

	nested, _ := findCol(cols, "nested")
	if nested.Type != colTypeComplex {
		t.Errorf("nested type = %q, want complex", nested.Type)
	}
	if nested.Min != nil || nested.Max != nil {
		t.Errorf("complex column must skip min/max")
	}

	mixed, _ := findCol(cols, "mixed")
	if mixed.Type != colTypeComplex {
		t.Errorf("mixed-type column = %q, want complex", mixed.Type)
	}
}

func TestComputeColumnStats_Sampled(t *testing.T) {
	records := []map[string]interface{}{{"a": "x"}, {"a": "y"}}
	cols := ComputeColumnStats(records, true, DefaultStatsTopK, DefaultStatsMaxDistinct)
	for _, c := range cols {
		if c.Basis != "sample" {
			t.Errorf("column %q basis = %q, want sample", c.Name, c.Basis)
		}
	}
}

func TestComputeColumnStats_HighCardinality(t *testing.T) {
	var records []map[string]interface{}
	for i := 0; i < 50; i++ {
		records = append(records, map[string]interface{}{"id": string(rune('a'+i%26)) + string(rune('0'+i/26))})
	}
	cols := ComputeColumnStats(records, false, DefaultStatsTopK, 5) // tiny cap
	id, _ := findCol(cols, "id")
	if !id.HighCardinality {
		t.Errorf("expected high_cardinality flag when distinct exceeds cap")
	}
	if id.Distinct != nil {
		t.Errorf("high-cardinality column should drop exact distinct")
	}
}

func TestComputeColumnStats_NaNInfExcluded(t *testing.T) {
	records := []map[string]interface{}{
		{"v": float64(10)},
		{"v": math.NaN()},
		{"v": math.Inf(1)},
		{"v": float64(30)},
	}
	cols := ComputeColumnStats(records, false, DefaultStatsTopK, DefaultStatsMaxDistinct)
	v, _ := findCol(cols, "v")
	// NaN/Inf force double, but must not poison aggregates.
	if v.Min != float64(10) || v.Max != float64(30) {
		t.Errorf("min/max = %v/%v, want 10/30 (NaN/Inf excluded)", v.Min, v.Max)
	}
	if v.Mean == nil || *v.Mean != 20 {
		t.Errorf("mean = %v, want 20 (NaN/Inf excluded from sum and denominator)", v.Mean)
	}
	// The whole point: the stats must marshal (encoding/json rejects NaN/Inf).
	if _, err := json.Marshal(cols); err != nil {
		t.Errorf("stats with NaN/Inf input must still marshal: %v", err)
	}
}

func TestComputeColumnStats_AllNaN(t *testing.T) {
	records := []map[string]interface{}{{"v": math.NaN()}, {"v": math.Inf(-1)}}
	cols := ComputeColumnStats(records, false, DefaultStatsTopK, DefaultStatsMaxDistinct)
	v, _ := findCol(cols, "v")
	if v.Min != nil || v.Max != nil || v.Mean != nil {
		t.Errorf("all-NaN column must report no min/max/mean, got %v/%v/%v", v.Min, v.Max, v.Mean)
	}
}

func TestComputeColumnStats_LargeIntNoOverflow(t *testing.T) {
	big := 1e19 // integral, but beyond int64 range and 2^53
	records := []map[string]interface{}{{"n": big}, {"n": float64(1)}}
	cols := ComputeColumnStats(records, false, DefaultStatsTopK, DefaultStatsMaxDistinct)
	n, _ := findCol(cols, "n")
	// Must not saturate to a fabricated int64; keep the float form.
	if got, ok := n.Max.(float64); !ok || got != big {
		t.Errorf("max = %v (%T), want float64 %v (no int64 overflow)", n.Max, n.Max, big)
	}
}

func TestSampleRows(t *testing.T) {
	records := []map[string]interface{}{{"a": 1}, {"a": 2}, {"a": 3}, {"a": 4}}
	if got := SampleRows(records, 2); len(got) != 2 {
		t.Errorf("SampleRows(2) len = %d, want 2", len(got))
	}
	if got := SampleRows(records, 10); len(got) != 4 {
		t.Errorf("SampleRows(10) len = %d, want 4 (clamped)", len(got))
	}
}
