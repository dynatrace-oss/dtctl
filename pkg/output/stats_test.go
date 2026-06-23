package output

import (
	"encoding/json"
	"math"
	"strings"
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

func TestComputeColumnStats_TruncatesLongTopValues(t *testing.T) {
	long := "https://example.invalid/" + string(make([]rune, 0)) // build a >64-rune value
	for len([]rune(long)) <= maxTopValueRunes {
		long += "abcdefghij"
	}
	records := []map[string]interface{}{
		{"url": long}, {"url": long}, {"url": "short"},
	}
	cols := ComputeColumnStats(records, false, DefaultStatsTopK, DefaultStatsMaxDistinct)
	url, _ := findCol(cols, "url")
	if len(url.Top) == 0 {
		t.Fatal("expected top values")
	}
	// The long value is most frequent (n=2) so it ranks first; it must be clipped.
	top := url.Top[0].V.(string)
	if r := []rune(top); len(r) > maxTopValueRunes+32 { // prefix + short marker
		t.Errorf("top value not truncated: %d runes", len(r))
	}
	if !strings.Contains(top, "…(+") {
		t.Errorf("truncated value missing length marker: %q", top)
	}
	// A value within the cap is left untouched.
	short := url.Top[1].V.(string)
	if short != "short" {
		t.Errorf("short value altered: %q", short)
	}
}

func TestCapColumnsForEnvelope(t *testing.T) {
	// 5 columns, descending population (a has fewest nulls, e the most).
	cols := []ColumnStats{
		{Name: "a", Nulls: 0},
		{Name: "b", Nulls: 1},
		{Name: "c", Nulls: 2},
		{Name: "d", Nulls: 3},
		{Name: "e", Nulls: 4},
	}

	// Within the cap: returned unchanged, nothing omitted.
	kept, omitted := CapColumnsForEnvelope(cols, 10)
	if len(kept) != 5 || omitted != nil {
		t.Errorf("within cap: kept=%d omitted=%v, want 5/nil", len(kept), omitted)
	}

	// Over the cap: keep the 2 most-populated (a, b), omit the rest by name.
	kept, omitted = CapColumnsForEnvelope(cols, 2)
	if len(kept) != 2 || kept[0].Name != "a" || kept[1].Name != "b" {
		t.Errorf("kept = %v, want [a b] (most populated)", names(kept))
	}
	wantOmitted := []string{"c", "d", "e"}
	if len(omitted) != 3 || omitted[0] != "c" || omitted[1] != "d" || omitted[2] != "e" {
		t.Errorf("omitted = %v, want %v (sorted)", omitted, wantOmitted)
	}

	// The input slice (used for the full sidecar) must not be mutated/reordered.
	if cols[0].Name != "a" || cols[4].Name != "e" {
		t.Error("input slice was reordered")
	}

	// max <= 0 disables capping.
	if k, o := CapColumnsForEnvelope(cols, 0); len(k) != 5 || o != nil {
		t.Errorf("max=0 should not cap: kept=%d omitted=%v", len(k), o)
	}
}

func names(cols []ColumnStats) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Name
	}
	return out
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

func TestSampleRows_ClipsLongStrings(t *testing.T) {
	long := strings.Repeat("x", maxSampleValueRunes*3) // well over the cap
	records := []map[string]interface{}{
		{
			"content": long,                                  // top-level long string
			"short":   "ok",                                  // within cap, untouched
			"nested":  map[string]interface{}{"stack": long}, // nested in a record
			"arr":     []interface{}{long, "fine"},           // nested in an array
		},
	}
	got := SampleRows(records, 1)[0]

	content := got["content"].(string)
	if r := []rune(content); len(r) > maxSampleValueRunes+32 {
		t.Errorf("top-level string not clipped: %d runes", len(r))
	}
	if !strings.Contains(content, "…(+") {
		t.Errorf("clipped value missing marker: %q", content[:40])
	}
	if got["short"] != "ok" {
		t.Errorf("short string altered: %v", got["short"])
	}
	if s := got["nested"].(map[string]interface{})["stack"].(string); !strings.Contains(s, "…(+") {
		t.Error("nested string not clipped")
	}
	if a := got["arr"].([]interface{}); !strings.Contains(a[0].(string), "…(+") || a[1] != "fine" {
		t.Errorf("array strings handled wrong: %v", a)
	}
	// Original record must be untouched (the file writer needs the full value).
	if records[0]["content"].(string) != long {
		t.Error("original record was mutated")
	}
}

func TestSampleRows_SanitizesNonFiniteFloats(t *testing.T) {
	// A non-finite float anywhere in a sampled row (top-level, nested record, or
	// array) must not survive into the envelope — encoding/json rejects NaN/Inf
	// and would fail the whole emit. The originals must be left untouched.
	records := []map[string]interface{}{
		{
			"bad":    math.NaN(),
			"posInf": math.Inf(1),
			"ok":     float64(3.5),
			"nested": map[string]interface{}{"x": math.Inf(-1)},
			"arr":    []interface{}{float64(1), math.NaN()},
		},
	}
	got := SampleRows(records, 1)
	row := got[0]
	if row["bad"] != nil || row["posInf"] != nil {
		t.Errorf("non-finite top-level floats not sanitised: %v", row)
	}
	if row["ok"] != float64(3.5) {
		t.Errorf("finite value altered: %v", row["ok"])
	}
	if n := row["nested"].(map[string]interface{}); n["x"] != nil {
		t.Errorf("nested non-finite not sanitised: %v", n)
	}
	if a := row["arr"].([]interface{}); a[0] != float64(1) || a[1] != nil {
		t.Errorf("array non-finite not sanitised: %v", a)
	}
	// The whole sanitised slice must now JSON-encode without error.
	if _, err := json.Marshal(got); err != nil {
		t.Fatalf("sanitised sample rows must marshal: %v", err)
	}
	// Original record is untouched (the file writer needs the raw rows).
	if v, ok := records[0]["bad"].(float64); !ok || !math.IsNaN(v) {
		t.Errorf("original record was mutated: %v", records[0]["bad"])
	}
}
