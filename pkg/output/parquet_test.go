package output

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
)

// readParquet reads all rows of a parquet byte buffer back into maps for
// assertion. It uses the file's own schema (the columns are dynamic, so a typed
// generic reader cannot be used), mapping each leaf value back to its column
// name. Null/missing cells come back as nil.
func readParquet(t *testing.T, data []byte) []map[string]interface{} {
	t.Helper()
	f, err := parquet.OpenFile(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open parquet: %v", err)
	}

	// Column index → leaf name (flat schema: one element per path).
	cols := f.Schema().Columns()
	names := make([]string, len(cols))
	for i, path := range cols {
		names[i] = path[len(path)-1]
	}

	var out []map[string]interface{}
	for _, rg := range f.RowGroups() {
		rows := rg.Rows()
		buf := make([]parquet.Row, 8)
		for {
			n, err := rows.ReadRows(buf)
			for i := 0; i < n; i++ {
				m := map[string]interface{}{}
				for _, v := range buf[i] {
					name := names[v.Column()]
					if v.IsNull() {
						m[name] = nil
						continue
					}
					switch v.Kind() {
					case parquet.Boolean:
						m[name] = v.Boolean()
					case parquet.Int64:
						m[name] = v.Int64()
					case parquet.Double:
						m[name] = v.Double()
					default:
						m[name] = v.String()
					}
				}
				out = append(out, m)
			}
			if err != nil {
				break
			}
		}
		rows.Close()
	}
	return out
}

func TestParquetPrinter_DQLTypes(t *testing.T) {
	var buf bytes.Buffer
	p := &ParquetPrinter{
		writer: &buf,
		types: []ColumnTypeMapping{
			{Name: "count", Type: "long"},
			{Name: "ratio", Type: "double"},
			{Name: "host", Type: "string"},
			{Name: "ok", Type: "boolean"},
		},
	}
	records := []map[string]interface{}{
		// "long" arriving as a JSON string (DQL does this) must coerce to int64.
		{"count": "194414758", "ratio": 0.5, "host": "web-01", "ok": true},
		{"count": float64(42), "ratio": 1.5, "host": "web-02", "ok": false},
	}
	if err := p.PrintList(records); err != nil {
		t.Fatalf("PrintList: %v", err)
	}

	rows := readParquet(t, buf.Bytes())
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0]["count"] != int64(194414758) {
		t.Errorf("count = %#v, want int64(194414758)", rows[0]["count"])
	}
	if rows[0]["host"] != "web-01" {
		t.Errorf("host = %#v, want web-01", rows[0]["host"])
	}
	if rows[0]["ok"] != true {
		t.Errorf("ok = %#v, want true", rows[0]["ok"])
	}
	if rows[1]["ratio"] != 1.5 {
		t.Errorf("ratio = %#v, want 1.5", rows[1]["ratio"])
	}
}

func TestParquetPrinter_ComplexFallback(t *testing.T) {
	var buf bytes.Buffer
	p := &ParquetPrinter{writer: &buf} // no DQL types → inference
	records := []map[string]interface{}{
		{"nested": map[string]interface{}{"a": 1}, "list": []interface{}{"x", "y"}},
	}
	if err := p.PrintList(records); err != nil {
		t.Fatalf("PrintList: %v", err)
	}
	rows := readParquet(t, buf.Bytes())
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	// Nested values become JSON-encoded strings.
	if s, ok := rows[0]["nested"].(string); !ok || s != `{"a":1}` {
		t.Errorf("nested = %#v, want JSON string {\"a\":1}", rows[0]["nested"])
	}
	if s, ok := rows[0]["list"].(string); !ok || s != `["x","y"]` {
		t.Errorf("list = %#v, want JSON string [\"x\",\"y\"]", rows[0]["list"])
	}
}

func TestParquetPrinter_SparseRowsAndNulls(t *testing.T) {
	var buf bytes.Buffer
	p := &ParquetPrinter{writer: &buf}
	records := []map[string]interface{}{
		{"a": "first"},           // missing "b"
		{"b": "second"},          // missing "a"
		{"a": nil, "b": "third"}, // explicit null "a"
	}
	if err := p.PrintList(records); err != nil {
		t.Fatalf("PrintList: %v", err)
	}
	rows := readParquet(t, buf.Bytes())
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0]["a"] != "first" {
		t.Errorf("row0 a = %#v, want first", rows[0]["a"])
	}
	// Missing/null cells read back as nil.
	if rows[0]["b"] != nil {
		t.Errorf("row0 b = %#v, want nil", rows[0]["b"])
	}
	if rows[2]["a"] != nil {
		t.Errorf("row2 a = %#v, want nil", rows[2]["a"])
	}
}

func TestParquetPrinter_Empty(t *testing.T) {
	// An empty result must still yield a valid, openable Parquet file (a
	// zero-byte file is not valid Parquet), even with no DQL types to lean on.
	var buf bytes.Buffer
	p := &ParquetPrinter{writer: &buf}
	if err := p.PrintList([]map[string]interface{}{}); err != nil {
		t.Fatalf("PrintList: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected a valid (non-zero-byte) Parquet file for an empty result")
	}
	f, err := parquet.OpenFile(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("empty result did not produce a valid Parquet file: %v", err)
	}
	if got := f.NumRows(); got != 0 {
		t.Errorf("NumRows = %d, want 0", got)
	}
}

func TestParquetPrinter_EmptyWithTypesKeepsSchema(t *testing.T) {
	// With DQL types but no rows, the empty file should still carry the schema.
	var buf bytes.Buffer
	p := &ParquetPrinter{
		writer: &buf,
		types: []ColumnTypeMapping{
			{Name: "host", Type: "string"},
			{Name: "count", Type: "long"},
		},
	}
	if err := p.PrintList([]map[string]interface{}{}); err != nil {
		t.Fatalf("PrintList: %v", err)
	}
	f, err := parquet.OpenFile(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("open parquet: %v", err)
	}
	if got := f.NumRows(); got != 0 {
		t.Errorf("NumRows = %d, want 0", got)
	}
	got := map[string]bool{}
	for _, path := range f.Schema().Columns() {
		got[path[len(path)-1]] = true
	}
	for _, want := range []string{"host", "count"} {
		if !got[want] {
			t.Errorf("schema missing declared column %q (cols: %v)", want, got)
		}
	}
}

func TestParquetPrinter_LongCoercionFromNativeInts(t *testing.T) {
	// A DQL "long" column whose cells arrive as assorted native integer widths
	// must all coerce to int64 rather than being written as null.
	var buf bytes.Buffer
	p := &ParquetPrinter{
		writer: &buf,
		types:  []ColumnTypeMapping{{Name: "n", Type: "long"}},
	}
	records := []map[string]interface{}{
		{"n": int32(7)},
		{"n": uint16(8)},
		{"n": int64(9)},
		{"n": float64(10)},
	}
	if err := p.PrintList(records); err != nil {
		t.Fatalf("PrintList: %v", err)
	}
	rows := readParquet(t, buf.Bytes())
	want := []int64{7, 8, 9, 10}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d", len(rows), len(want))
	}
	for i, w := range want {
		if rows[i]["n"] != w {
			t.Errorf("row %d n = %#v, want int64(%d)", i, rows[i]["n"], w)
		}
	}
}

func TestNewPrinterWithOpts_ParquetUsesTypes(t *testing.T) {
	// Guards the dispatch wiring: NewPrinterWithOpts must route "parquet" to the
	// ParquetPrinter AND thread opts.Types through, so the schema is type-faithful
	// (a "long" coerces to int64) rather than value-inferred.
	var buf bytes.Buffer
	printer := NewPrinterWithOpts(PrinterOptions{
		Format: "parquet",
		Writer: &buf,
		Types:  []ColumnTypeMapping{{Name: "count", Type: "long"}},
	})
	if _, ok := printer.(*ParquetPrinter); !ok {
		t.Fatalf("got %T, want *ParquetPrinter", printer)
	}
	// "long" arriving as a JSON string must land as int64 because the DQL type
	// (not value-inference) drove the schema.
	if err := printer.PrintList([]map[string]interface{}{{"count": "194414758"}}); err != nil {
		t.Fatalf("PrintList: %v", err)
	}
	rows := readParquet(t, buf.Bytes())
	if len(rows) != 1 || rows[0]["count"] != int64(194414758) {
		t.Errorf("count = %#v, want int64(194414758) — Types not threaded through dispatch", rows[0]["count"])
	}
}

func TestParquetPrinter_AllNullColumnIsString(t *testing.T) {
	if got := inferKind("x", []map[string]interface{}{{"x": nil}, {}}); got != colString {
		t.Errorf("all-null column inferred as %v, want colString", got)
	}
}

func TestParquetPrinter_Timestamp(t *testing.T) {
	// A DQL "timestamp" column is written as a native INT64 TIMESTAMP(NANOS)
	// column (not an opaque string), so downstream tooling sees a real temporal
	// type. RFC3339 strings, with and without sub-second precision, must parse.
	var buf bytes.Buffer
	p := &ParquetPrinter{
		writer: &buf,
		types:  []ColumnTypeMapping{{Name: "ts", Type: "timestamp"}},
	}
	records := []map[string]interface{}{
		{"ts": "2025-03-15T10:30:00Z"},
		{"ts": "2025-03-15T10:30:00.123456789Z"},
		{"ts": "not-a-timestamp"}, // unparseable → null, not a failed export
	}
	if err := p.PrintList(records); err != nil {
		t.Fatalf("PrintList: %v", err)
	}

	// The schema must declare a TIMESTAMP logical type on the column.
	f, err := parquet.OpenFile(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("open parquet: %v", err)
	}
	col, ok := f.Schema().Lookup("ts")
	if !ok || col.Node.Type().LogicalType() == nil || col.Node.Type().LogicalType().Timestamp == nil {
		t.Fatalf("ts column is not a TIMESTAMP logical type: %+v", col)
	}

	rows := readParquet(t, buf.Bytes())
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	want0 := time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC).UnixNano()
	if rows[0]["ts"] != want0 {
		t.Errorf("row0 ts = %#v, want int64(%d)", rows[0]["ts"], want0)
	}
	want1 := time.Date(2025, 3, 15, 10, 30, 0, 123456789, time.UTC).UnixNano()
	if rows[1]["ts"] != want1 {
		t.Errorf("row1 ts = %#v, want int64(%d)", rows[1]["ts"], want1)
	}
	if rows[2]["ts"] != nil {
		t.Errorf("row2 ts = %#v, want nil (unparseable timestamp)", rows[2]["ts"])
	}
}

func TestParquetPrinter_LongRejectsFractionalFloat(t *testing.T) {
	// A "long" column must coerce integral floats to int64 but must NOT silently
	// truncate a fractional value — that cell is written as null instead.
	var buf bytes.Buffer
	p := &ParquetPrinter{
		writer: &buf,
		types:  []ColumnTypeMapping{{Name: "n", Type: "long"}},
	}
	records := []map[string]interface{}{
		{"n": float64(42)},  // integral → int64(42)
		{"n": float64(7.5)}, // fractional → null (never truncated to 7)
	}
	if err := p.PrintList(records); err != nil {
		t.Fatalf("PrintList: %v", err)
	}
	rows := readParquet(t, buf.Bytes())
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0]["n"] != int64(42) {
		t.Errorf("row0 n = %#v, want int64(42)", rows[0]["n"])
	}
	if rows[1]["n"] != nil {
		t.Errorf("row1 n = %#v, want nil (fractional float must not truncate)", rows[1]["n"])
	}
}

func TestFloat64ToInt64(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want interface{}
		ok   bool
	}{
		{"integral", 42, int64(42), true},
		{"zero", 0, int64(0), true},
		{"negative integral", -9, int64(-9), true},
		{"fractional", 7.5, nil, false},
		{"NaN", math.NaN(), nil, false},
		{"+Inf", math.Inf(1), nil, false},
		{"-Inf", math.Inf(-1), nil, false},
		{"above int64 range", 9.3e18, nil, false},
		{"min int64", math.MinInt64, int64(math.MinInt64), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := float64ToInt64(tt.in)
			if ok != tt.ok || got != tt.want {
				t.Errorf("float64ToInt64(%v) = (%#v, %v), want (%#v, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}
