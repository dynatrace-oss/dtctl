package output

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/parquet-go/parquet-go"
)

// ParquetPrinter writes query results as an Apache Parquet file.
//
// Parquet is columnar, pure-Go (no CGO), and read natively by common local
// analytics tooling, which makes it a good target for large exported results.
// In this iteration the writer is buffered — it builds the schema from the full
// set of records held in memory before writing — matching how the rest of the
// query path already buffers results. (A streaming, row-group-flushed writer is
// a later optimisation.)
//
// # DQL → Parquet type mapping
//
// The column schema is derived from the DQL column types returned by the query
// API (populated when --include-types is set), falling back to value-inference
// for any column missing from the type mappings. Each column is OPTIONAL
// (nullable) so sparse rows and explicit nulls are tolerated.
//
//	DQL type                  Parquet column      Notes
//	------------------------  ------------------  ---------------------------------
//	boolean                   BOOLEAN
//	long                      INT64
//	double                    DOUBLE
//	string                    STRING
//	timestamp                 INT64 (TIMESTAMP)   nanosecond precision, UTC-adjusted;
//	                                              unparseable values written as null
//	duration                  INT64               nanoseconds; math-ready, no CAST
//	                                              needed; unparseable values → null
//	timeframe / ip            STRING              rendered as their string form
//	(anything else, nested    STRING (complex)    JSON-encoded — never crash, never
//	 record/array, mixed)                         silently drop a column
//
// Cells that cannot be coerced to their column's physical type are written as
// null rather than failing the whole export.
type ParquetPrinter struct {
	writer io.Writer
	// types carries the DQL column type mappings (from Response.GetTypes()).
	// It may be nil, in which case the schema is inferred from record values.
	types []ColumnTypeMapping
}

// ColumnTypeMapping is the output-layer view of a DQL column's type. It mirrors
// the SDK's per-column type info without importing the SDK into the dispatch
// path, keeping the dependency direction one-way.
type ColumnTypeMapping struct {
	Name string
	Type string // DQL type name, e.g. "long", "string", "timestamp"
}

// parquetColumnKind is the physical column type chosen for a DQL column.
type parquetColumnKind int

const (
	colString parquetColumnKind = iota
	colInt64
	colDouble
	colBoolean
	colTimestamp // INT64 column with TIMESTAMP(NANOS) logical type
	colComplex   // JSON-encoded string column for nested/variant/unmappable types
)

// parquetRowsPerRowGroup caps how many rows accumulate in the writer's in-memory
// column buffers before a complete row group is flushed to the output. Without
// a cap, parquet-go buffers every row until Close, so peak memory scales with
// the whole result set; flushing in bounded groups keeps it roughly constant in
// the row count (and produces multiple row groups, which readers can skip/scan
// independently). This is a stopgap until the input itself is streamed — the
// records slice is still held in memory by the caller.
const parquetRowsPerRowGroup = 100_000

// parquetEmptyResultColumn is the name of the single placeholder column emitted
// when a result has no columns to declare — an empty result for which the DQL
// API returned neither records nor a `types` block (it returns `"types":[]`
// alongside `"records":[]`, so there is no schema to recover). A column-less
// Parquet file, while a structurally valid container, is rejected by mainstream
// readers (DuckDB, pyarrow, pandas: "Need at least one non-root column in the
// file"), so we emit one nullable placeholder column to keep the file portable.
const parquetEmptyResultColumn = "_dtctl_empty"

// Print writes a single object as a one-row Parquet file.
func (p *ParquetPrinter) Print(obj interface{}) error {
	return p.PrintList([]interface{}{obj})
}

// PrintList writes a slice of records as a Parquet file.
//
// A Parquet file is always emitted, even for an empty result: a zero-byte file
// is not valid Parquet and would be rejected by downstream tooling, so an empty
// result yields a valid file with zero rows. When the DQL types are known the
// file carries that schema; when they are not (the API returns no types for an
// empty result) it carries a single placeholder column so it stays readable by
// mainstream tooling. (This differs from the CSV/JSONL printers, where an empty
// file is itself valid output.)
func (p *ParquetPrinter) PrintList(obj interface{}) error {
	records, err := toRecordMaps(obj)
	if err != nil {
		return err
	}

	// Stable column order: union of keys across all records, sorted. When there
	// are no records, fall back to the DQL-declared columns so the empty file
	// still carries a faithful schema.
	columns := p.columnsFor(records)

	// A column-less schema yields a file mainstream readers reject (see
	// parquetEmptyResultColumn). This happens for an empty result the API did
	// not type, and for the degenerate case where every record is an empty map.
	// Emit a single nullable placeholder column so the file stays portable.
	if len(columns) == 0 {
		columns = []string{parquetEmptyResultColumn}
	}

	kinds := p.resolveColumnKinds(columns, records)

	// Build the parquet schema: one optional leaf per column.
	group := parquet.Group{}
	for _, name := range columns {
		group[name] = parquet.Optional(leafNodeFor(kinds[name]))
	}
	schema := parquet.NewSchema("dtctl", group)

	w := parquet.NewWriter(p.writer, schema, parquet.MaxRowsPerRowGroup(parquetRowsPerRowGroup))
	for _, rec := range records {
		row := make(map[string]interface{}, len(columns))
		for _, name := range columns {
			raw, present := rec[name]
			if !present || raw == nil {
				continue // absent → null
			}
			coerced, ok := coerceValue(raw, kinds[name])
			if !ok {
				continue // uncoercible cell → null, rather than failing the export
			}
			row[name] = coerced
		}
		if err := w.Write(row); err != nil {
			_ = w.Close() // release writer buffers; original error wins
			return fmt.Errorf("writing parquet row: %w", err)
		}
	}
	return w.Close()
}

// columnsFor returns the ordered column set for the schema: the sorted union of
// record keys when there are records, otherwise the sorted set of DQL-declared
// columns. The latter lets an empty result still produce a valid, schema-bearing
// Parquet file. Returns nil when there are neither records nor DQL types; the
// caller substitutes a placeholder column so the file is never column-less.
func (p *ParquetPrinter) columnsFor(records []map[string]interface{}) []string {
	if len(records) > 0 {
		return unionColumns(records)
	}
	if len(p.types) == 0 {
		return nil
	}
	cols := make([]string, 0, len(p.types))
	for _, t := range p.types {
		cols = append(cols, t.Name)
	}
	sort.Strings(cols)
	return cols
}

// resolveColumnKinds picks a physical column kind for every column, preferring
// the DQL type when available and falling back to value-inference otherwise.
func (p *ParquetPrinter) resolveColumnKinds(columns []string, records []map[string]interface{}) map[string]parquetColumnKind {
	dqlTypes := make(map[string]string, len(p.types))
	for _, t := range p.types {
		dqlTypes[t.Name] = t.Type
	}

	kinds := make(map[string]parquetColumnKind, len(columns))
	for _, name := range columns {
		if dt, ok := dqlTypes[name]; ok {
			kinds[name] = kindForDQLType(dt)
			continue
		}
		kinds[name] = inferKind(name, records)
	}
	return kinds
}

// kindForDQLType maps a DQL type name to a physical column kind.
func kindForDQLType(dqlType string) parquetColumnKind {
	switch dqlType {
	case "boolean":
		return colBoolean
	case "long":
		return colInt64
	case "double":
		return colDouble
	case "timestamp":
		return colTimestamp
	case "duration":
		// DQL durations arrive as an integer count of nanoseconds (JSON-encoded
		// as a string, e.g. "598600"). Writing them as INT64 keeps the value
		// math-ready in local tooling — no CAST needed for sum/avg/quantile —
		// rather than as an opaque string. A value that does not parse as an
		// integer is written as null (coerceValue), never crashing the export.
		return colInt64
	case "string", "timeframe", "ip":
		return colString
	default:
		// arrays, records, variant and any future/unknown type → JSON column.
		return colComplex
	}
}

// inferKind chooses a column kind from the first non-null value seen for a
// column. JSON decoding yields float64 for all numbers, so numeric columns
// infer as DOUBLE; nested values (maps/slices) become complex JSON columns.
func inferKind(name string, records []map[string]interface{}) parquetColumnKind {
	for _, rec := range records {
		v, ok := rec[name]
		if !ok || v == nil {
			continue
		}
		switch v.(type) {
		case bool:
			return colBoolean
		case float64, float32:
			return colDouble
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return colInt64
		case string:
			return colString
		default:
			return colComplex // maps, slices, structs, etc.
		}
	}
	// All-null column: a STRING column is the safe, lossless choice.
	return colString
}

// leafNodeFor returns the parquet leaf node for a column kind.
func leafNodeFor(kind parquetColumnKind) parquet.Node {
	switch kind {
	case colInt64:
		return parquet.Leaf(parquet.Int64Type)
	case colDouble:
		return parquet.Leaf(parquet.DoubleType)
	case colBoolean:
		return parquet.Leaf(parquet.BooleanType)
	case colTimestamp:
		// Physically INT64; nanosecond precision keeps DQL's full resolution.
		return parquet.Timestamp(parquet.Nanosecond)
	default: // colString and colComplex are both physically STRING
		return parquet.String()
	}
}

// coerceValue converts a raw record value to the Go type the column kind needs.
// Returns ok=false when the value cannot be represented (→ written as null).
func coerceValue(raw interface{}, kind parquetColumnKind) (interface{}, bool) {
	switch kind {
	case colComplex:
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, false
		}
		return string(b), true

	case colString:
		switch s := raw.(type) {
		case string:
			return s, true
		default:
			return fmt.Sprintf("%v", raw), true
		}

	case colBoolean:
		if b, ok := raw.(bool); ok {
			return b, true
		}
		return nil, false

	case colTimestamp:
		// DQL delivers timestamps as RFC3339 strings; parquet-go writes the
		// time.Time into the INT64 TIMESTAMP column. A value we cannot parse is
		// written as null rather than failing the whole export.
		switch t := raw.(type) {
		case time.Time:
			return t, true
		case string:
			if ts, ok := parseDQLTimestamp(t); ok {
				return ts, true
			}
		}
		return nil, false

	case colInt64:
		switch n := raw.(type) {
		case int:
			return int64(n), true
		case int8:
			return int64(n), true
		case int16:
			return int64(n), true
		case int32:
			return int64(n), true
		case int64:
			return n, true
		case uint:
			return int64(n), true
		case uint8:
			return int64(n), true
		case uint16:
			return int64(n), true
		case uint32:
			return int64(n), true
		case uint64:
			return int64(n), true
		case float32:
			return float64ToInt64(float64(n))
		case float64:
			// JSON decodes integer longs to float64; coerce only when the value
			// is genuinely integral and in range, never silently truncating.
			return float64ToInt64(n)
		case string:
			if i, err := strconv.ParseInt(n, 10, 64); err == nil {
				return i, true
			}
		}
		return nil, false

	case colDouble:
		switch n := raw.(type) {
		case int:
			return float64(n), true
		case int8:
			return float64(n), true
		case int16:
			return float64(n), true
		case int32:
			return float64(n), true
		case int64:
			return float64(n), true
		case uint:
			return float64(n), true
		case uint8:
			return float64(n), true
		case uint16:
			return float64(n), true
		case uint32:
			return float64(n), true
		case uint64:
			return float64(n), true
		case float32:
			return float64(n), true
		case float64:
			return n, true
		case string:
			if f, err := strconv.ParseFloat(n, 64); err == nil {
				return f, true
			}
		}
		return nil, false
	}
	return nil, false
}

// float64ToInt64 converts a JSON-decoded float into an int64 for a DQL "long"
// column. It returns ok=false (→ null) for NaN/Inf, fractional values, or values
// outside the int64 range, so a non-integral value is never silently truncated.
func float64ToInt64(f float64) (interface{}, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
		return nil, false
	}
	// float64(math.MaxInt64) rounds up to 2^63, so use a strict upper bound;
	// math.MinInt64 (-2^63) is exactly representable, so the lower bound is
	// inclusive.
	const maxInt64Plus1 = 9223372036854775808.0 // 2^63
	if f < math.MinInt64 || f >= maxInt64Plus1 {
		return nil, false
	}
	return int64(f), true
}

// parseDQLTimestamp parses a DQL timestamp string (RFC3339, with or without
// sub-second precision) into a time.Time. RFC3339Nano accepts both forms.
func parseDQLTimestamp(s string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// unionColumns returns the sorted union of keys across all records.
func unionColumns(records []map[string]interface{}) []string {
	set := make(map[string]struct{})
	for _, rec := range records {
		for k := range rec {
			set[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(set))
	for k := range set {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

// toRecordMaps normalises the printer input (a slice of records) into a slice of
// string-keyed maps, reflecting over interface-wrapped maps like the CSV printer.
func toRecordMaps(obj interface{}) ([]map[string]interface{}, error) {
	// Fast path: the query layer already hands us []map[string]interface{}.
	// Reflecting over and rebuilding every record (as the general path below
	// does) would hold a second full copy of the result set in memory — costly
	// for large exports — so reuse the slice directly. PrintList only reads it.
	if m, ok := obj.([]map[string]interface{}); ok {
		return m, nil
	}

	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Slice {
		return nil, fmt.Errorf("expected slice, got %s", v.Kind())
	}

	out := make([]map[string]interface{}, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elem.Kind() == reflect.Interface {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}

		switch elem.Kind() {
		case reflect.Map:
			m := make(map[string]interface{}, elem.Len())
			iter := elem.MapRange()
			for iter.Next() {
				m[fmt.Sprintf("%v", iter.Key().Interface())] = iter.Value().Interface()
			}
			out = append(out, m)
		default:
			// Non-map elements (e.g. structs) round-trip through JSON so json
			// tags are respected, matching the toon/json printers' behaviour.
			b, err := json.Marshal(elem.Interface())
			if err != nil {
				return nil, err
			}
			var m map[string]interface{}
			if err := json.Unmarshal(b, &m); err != nil {
				return nil, fmt.Errorf("parquet output requires record-shaped rows: %w", err)
			}
			out = append(out, m)
		}
	}
	return out, nil
}
