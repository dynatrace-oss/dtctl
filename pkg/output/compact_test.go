package output

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCompactRecords_HoistsConstantsAndDropsNulls(t *testing.T) {
	records := []map[string]interface{}{
		{"service.name": "payment", "k8s.namespace.name": "checkout", "span.name": "POST /pay", "error": nil},
		{"service.name": "payment", "k8s.namespace.name": "checkout", "span.name": "GET /cart", "error": nil},
	}
	c := CompactRecords(records)

	wantConst := map[string]interface{}{"service.name": "payment", "k8s.namespace.name": "checkout"}
	if !reflect.DeepEqual(c.Constant, wantConst) {
		t.Errorf("Constant = %v, want %v", c.Constant, wantConst)
	}
	wantRows := []map[string]interface{}{{"span.name": "POST /pay"}, {"span.name": "GET /cart"}}
	if !reflect.DeepEqual(c.Records, wantRows) {
		t.Errorf("Records = %v, want %v", c.Records, wantRows)
	}
	if !reflect.DeepEqual(c.NullColumns, []string{"error"}) {
		t.Errorf("NullColumns = %v, want [error]", c.NullColumns)
	}
}

// A value is only constant when it is present, non-null and equal in every row:
// a column that is null (or absent) in one row keeps its value on the rows that
// have it, so `row = constant ∪ record` stays exact.
func TestCompactRecords_PartialColumnsAreNotConstant(t *testing.T) {
	records := []map[string]interface{}{
		{"a": "x", "b": "y", "c": 1.0},
		{"a": nil, "b": "y", "c": 2.0},
		{"b": "y", "c": 1.0},
	}
	c := CompactRecords(records)

	if !reflect.DeepEqual(c.Constant, map[string]interface{}{"b": "y"}) {
		t.Errorf("Constant = %v, want only b", c.Constant)
	}
	want := []map[string]interface{}{{"a": "x", "c": 1.0}, {"c": 2.0}, {"c": 1.0}}
	if !reflect.DeepEqual(c.Records, want) {
		t.Errorf("Records = %v, want %v", c.Records, want)
	}
	if len(c.NullColumns) != 0 {
		t.Errorf("NullColumns = %v, want none (a has a value in one row)", c.NullColumns)
	}
}

func TestCompactRecords_NestedValuesCompareDeeply(t *testing.T) {
	records := []map[string]interface{}{
		{"tags": map[string]interface{}{"env": "prod"}, "list": []interface{}{"a"}, "v": 1.0},
		{"tags": map[string]interface{}{"env": "prod"}, "list": []interface{}{"b"}, "v": 2.0},
	}
	c := CompactRecords(records)
	if _, ok := c.Constant["tags"]; !ok {
		t.Errorf("equal nested maps should be constant, got %v", c.Constant)
	}
	if _, ok := c.Constant["list"]; ok {
		t.Errorf("differing arrays must not be constant, got %v", c.Constant)
	}
}

// With a single row every column is trivially "constant", which would just move
// the row into another key; only nulls are dropped.
func TestCompactRecords_SingleRowOnlyDropsNulls(t *testing.T) {
	c := CompactRecords([]map[string]interface{}{{"a": "x", "b": nil}})
	if c.Constant != nil {
		t.Errorf("Constant = %v, want nil for one row", c.Constant)
	}
	if !reflect.DeepEqual(c.Records, []map[string]interface{}{{"a": "x"}}) {
		t.Errorf("Records = %v", c.Records)
	}
	if !reflect.DeepEqual(c.NullColumns, []string{"b"}) {
		t.Errorf("NullColumns = %v, want [b]", c.NullColumns)
	}
}

func TestCompactRecords_EmptyAndNil(t *testing.T) {
	for _, in := range [][]map[string]interface{}{nil, {}} {
		c := CompactRecords(in)
		if c.Constant != nil || len(c.NullColumns) != 0 {
			t.Errorf("CompactRecords(%v) = %+v, want no constant/null columns", in, c)
		}
		if c.Records == nil || len(c.Records) != 0 {
			t.Errorf("Records = %#v, want empty non-nil slice so JSON stays []", c.Records)
		}
	}
}

func TestCompactRecords_DoesNotMutateInput(t *testing.T) {
	records := []map[string]interface{}{
		{"a": "x", "b": nil, "c": 1.0},
		{"a": "x", "b": nil, "c": 2.0},
	}
	before, _ := json.Marshal(records)
	_ = CompactRecords(records)
	after, _ := json.Marshal(records)
	if string(before) != string(after) {
		t.Errorf("input mutated:\nbefore %s\nafter  %s", before, after)
	}
}

// A column where every row holds the same value is only hoisted when there is
// something left per row; identical rows still keep their row count.
func TestCompactRecords_IdenticalRowsKeepRowCount(t *testing.T) {
	c := CompactRecords([]map[string]interface{}{{"a": "x"}, {"a": "x"}})
	if !reflect.DeepEqual(c.Constant, map[string]interface{}{"a": "x"}) {
		t.Errorf("Constant = %v", c.Constant)
	}
	if len(c.Records) != 2 {
		t.Errorf("len(Records) = %d, want 2", len(c.Records))
	}
}

func TestCompaction_FilterColumns(t *testing.T) {
	c := CompactRecords([]map[string]interface{}{
		{"k": "same", "v": 1.0, "n": nil},
		{"k": "same", "v": 2.0, "n": nil},
	})
	cols := []ColumnStats{{Name: "k"}, {Name: "n"}, {Name: "v"}}
	got := c.FilterColumns(cols)
	if len(got) != 1 || got[0].Name != "v" {
		t.Errorf("FilterColumns = %v, want only v", got)
	}
	if len(cols) != 3 {
		t.Errorf("input slice mutated: %v", cols)
	}
}

// The constant map must come before the rows in the encoded envelope so a
// streaming reader sees the shared values first.
func TestInlineRecords_ConstantPrecedesRecords(t *testing.T) {
	b, err := json.Marshal(&InlineRecords{
		Kind:     KindRecords,
		Constant: map[string]interface{}{"service.name": "payment"},
		Records:  []map[string]interface{}{{"span.name": "a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"kind":"records","constant":{"service.name":"payment"},"records":[{"span.name":"a"}]}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}

	// Without compaction the shape is byte-for-byte what it was.
	b, _ = json.Marshal(&InlineRecords{Kind: KindRecords, Records: []map[string]interface{}{{"a": nil}}})
	if string(b) != `{"kind":"records","records":[{"a":null}]}` {
		t.Errorf("uncompacted shape changed: %s", b)
	}
}

// TOON lays uniform rows out as one table; dropping a partial null would make
// the rows ragged and fall back to a per-row key/value list, which costs more
// than the nulls it saved. The tabular view keeps partial nulls and drops only
// the constant and all-null columns.
func TestCompaction_TabularKeepsRowsUniform(t *testing.T) {
	records := []map[string]interface{}{
		{"k": "same", "a": "x", "b": nil, "n": nil},
		{"k": "same", "a": "y", "b": "z", "n": nil},
	}
	c := CompactRecords(records)
	got := c.Tabular(records)
	want := []map[string]interface{}{{"a": "x", "b": nil}, {"a": "y", "b": "z"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tabular = %v, want %v", got, want)
	}

	toon, err := MarshalTOON(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(toon, "{a,b}") {
		t.Errorf("tabular rows did not encode as one TOON table:\n%s", toon)
	}
	if _, ok := records[0]["k"]; !ok {
		t.Error("input records were mutated")
	}
}
