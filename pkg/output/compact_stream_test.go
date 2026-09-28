package output

import (
	"reflect"
	"testing"
)

// The accumulator is the streaming half of CompactRecords, so it has to agree
// with it on every shape that decides what the summary reports.
func TestCompactionAccumulator_MatchesCompactRecords(t *testing.T) {
	cases := []struct {
		name    string
		records []map[string]interface{}
	}{
		{name: "no rows"},
		{
			name:    "one row is never constant",
			records: []map[string]interface{}{{"a": "x", "b": 1.0}},
		},
		{
			name: "shared columns hoist",
			records: []map[string]interface{}{
				{"host": "web-01", "env": "prod", "n": 1.0},
				{"host": "web-02", "env": "prod", "n": 2.0},
				{"host": "web-03", "env": "prod", "n": 3.0},
			},
		},
		{
			name: "a column absent from one row is not constant",
			records: []map[string]interface{}{
				{"env": "prod", "n": 1.0},
				{"n": 2.0},
			},
		},
		{
			name: "a column null in one row is not constant",
			records: []map[string]interface{}{
				{"env": "prod", "n": 1.0},
				{"env": nil, "n": 2.0},
			},
		},
		{
			name: "all-null columns are named, not hoisted",
			records: []map[string]interface{}{
				{"env": "prod", "gone": nil},
				{"env": "prod", "gone": nil},
			},
		},
		{
			name: "a column only ever absent is not seen at all",
			records: []map[string]interface{}{
				{"a": 1.0},
				{"a": 2.0, "b": nil},
			},
		},
		{
			name: "nested values compare by value",
			records: []map[string]interface{}{
				{"tags": map[string]interface{}{"t": "x"}, "n": 1.0},
				{"tags": map[string]interface{}{"t": "x"}, "n": 2.0},
			},
		},
		{
			name: "nested values that differ are not constant",
			records: []map[string]interface{}{
				{"tags": map[string]interface{}{"t": "x"}},
				{"tags": map[string]interface{}{"t": "y"}},
			},
		},
		{
			name: "every column constant",
			records: []map[string]interface{}{
				{"a": "x", "b": "y"},
				{"a": "x", "b": "y"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := CompactRecords(tc.records)

			acc := NewCompactionAccumulator()
			for _, rec := range tc.records {
				acc.Observe(rec)
			}
			got := acc.Finalize()

			if !reflect.DeepEqual(got.Constant, want.Constant) {
				t.Errorf("Constant = %#v, want %#v", got.Constant, want.Constant)
			}
			if !reflect.DeepEqual(got.NullColumns, want.NullColumns) {
				t.Errorf("NullColumns = %v, want %v", got.NullColumns, want.NullColumns)
			}
			if got.DroppedNulls != want.DroppedNulls {
				t.Errorf("DroppedNulls = %d, want %d", got.DroppedNulls, want.DroppedNulls)
			}
			for _, enc := range []string{"json", "toon", "csv"} {
				if got.Changed(enc) != want.Changed(enc) {
					t.Errorf("Changed(%q) = %v, want %v", enc, got.Changed(enc), want.Changed(enc))
				}
			}
		})
	}
}

// Memory must not grow with the row count.
func TestCompactionAccumulator_HoldsNoRows(t *testing.T) {
	acc := NewCompactionAccumulator()
	for i := 0; i < 10000; i++ {
		acc.Observe(map[string]interface{}{"env": "prod", "n": float64(i)})
	}
	got := acc.Finalize()
	if len(got.Records) != 0 {
		t.Errorf("Records holds %d rows; the streaming accumulator must keep none", len(got.Records))
	}
	if want := (map[string]interface{}{"env": "prod"}); !reflect.DeepEqual(got.Constant, want) {
		t.Errorf("Constant = %#v, want %#v", got.Constant, want)
	}
}
