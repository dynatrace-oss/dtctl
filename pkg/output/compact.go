package output

import (
	"reflect"
	"sort"
)

// Compaction is a token-lean view of a row set (#578). Telemetry rows are
// dominated by resource attributes that are identical across a result
// (k8s.*, host.*, service.name, ...) and by explicit nulls from mixed sources;
// repeating them on every row is most of what an agent pays for.
//
// The original rows are recovered exactly as `row = Constant ∪ record`, with a
// key absent from both meaning null.
type Compaction struct {
	// Constant holds every column that is present, non-null and equal in every
	// row. Nil when there are fewer than two rows (a single row is trivially
	// constant, and hoisting it would only move it) or nothing is shared.
	Constant map[string]interface{}
	// Records are copies of the input rows without the Constant columns and
	// without null values. Never nil, so an empty result still encodes as [].
	Records []map[string]interface{}
	// NullColumns names, sorted, the columns that are null or absent in every
	// row. They vanish from Records entirely; a summary that describes the
	// schema can still list them.
	NullColumns []string
	// DroppedNulls counts the null values omitted from Records.
	DroppedNulls int
}

// Changed reports whether the compaction altered what the given encoding
// emits: something was hoisted, or a null was dropped. Under the tabular
// encodings (TOON, CSV) only all-null columns drop (see Tabular), so a partial
// null does not count there.
func (c Compaction) Changed(encoding string) bool {
	if len(c.Constant) > 0 {
		return true
	}
	if encoding == "toon" || encoding == "csv" {
		return len(c.NullColumns) > 0
	}
	return c.DroppedNulls > 0
}

// CompactRecords computes the Compaction of records. The input is not mutated.
func CompactRecords(records []map[string]interface{}) Compaction {
	var c Compaction

	// Columns with at least one non-null value, and those that are null in
	// every row they appear in.
	seen := map[string]bool{}
	hasValue := map[string]bool{}
	for _, rec := range records {
		for k, v := range rec {
			seen[k] = true
			if v != nil {
				hasValue[k] = true
			}
		}
	}
	for k := range seen {
		if !hasValue[k] {
			c.NullColumns = append(c.NullColumns, k)
		}
	}
	sort.Strings(c.NullColumns)

	if len(records) >= 2 {
		for k, v := range records[0] {
			if v == nil {
				continue
			}
			constant := true
			for _, rec := range records[1:] {
				other, ok := rec[k]
				if !ok || other == nil || !reflect.DeepEqual(v, other) {
					constant = false
					break
				}
			}
			if constant {
				if c.Constant == nil {
					c.Constant = map[string]interface{}{}
				}
				c.Constant[k] = v
			}
		}
	}

	c.Records, c.DroppedNulls = c.strip(records)
	return c
}

// Sparse returns copies of records without the constant columns and without
// any null value — the shape CompactRecords gives Records. A streamed result
// uses it for the few rows it keeps (the sample), since its Compaction carries
// no Records.
func (c Compaction) Sparse(records []map[string]interface{}) []map[string]interface{} {
	rows, _ := c.strip(records)
	return rows
}

// strip is Sparse, also counting the null values it dropped.
func (c Compaction) strip(records []map[string]interface{}) ([]map[string]interface{}, int) {
	out := make([]map[string]interface{}, 0, len(records))
	dropped := 0
	for _, rec := range records {
		row := make(map[string]interface{}, len(rec))
		for k, v := range rec {
			if v == nil {
				dropped++
				continue
			}
			if _, ok := c.Constant[k]; ok {
				continue
			}
			row[k] = v
		}
		out = append(out, row)
	}
	return out, dropped
}

// Tabular returns copies of records (the rows this compaction was computed
// from) without the constant and all-null columns, but keeping the nulls of
// every other column. A tabular encoding (TOON, CSV) needs uniform rows to
// print one table; dropping a partial null there makes the rows ragged, and
// TOON's per-row key/value fallback costs more than the nulls it saved.
func (c Compaction) Tabular(records []map[string]interface{}) []map[string]interface{} {
	nulls := make(map[string]bool, len(c.NullColumns))
	for _, n := range c.NullColumns {
		nulls[n] = true
	}
	out := make([]map[string]interface{}, 0, len(records))
	for _, rec := range records {
		row := make(map[string]interface{}, len(rec))
		for k, v := range rec {
			if _, ok := c.Constant[k]; ok || nulls[k] {
				continue
			}
			row[k] = v
		}
		out = append(out, row)
	}
	return out
}

// FilterColumns returns cols without the columns this compaction collapsed
// (constant and all-null ones), whose per-column profile carries nothing the
// Constant map or NullColumns list does not already say. cols is not mutated.
func (c Compaction) FilterColumns(cols []ColumnStats) []ColumnStats {
	nulls := make(map[string]bool, len(c.NullColumns))
	for _, n := range c.NullColumns {
		nulls[n] = true
	}
	out := make([]ColumnStats, 0, len(cols))
	for _, col := range cols {
		if _, ok := c.Constant[col.Name]; ok || nulls[col.Name] {
			continue
		}
		out = append(out, col)
	}
	return out
}

// EnvelopeConstant returns Constant prepared for a summary envelope the way
// sample rows are (see prepareSampleValue): long strings clipped, non-finite
// floats dropped. The full values are in the rows on disk. Nil when there is
// nothing constant.
func (c Compaction) EnvelopeConstant() map[string]interface{} {
	if len(c.Constant) == 0 {
		return nil
	}
	m, _ := prepareSampleValue(c.Constant).(map[string]interface{})
	return m
}

// CompactionAccumulator folds rows one at a time into the column-level half of
// a Compaction — the constant and all-null columns — so a streamed result can
// be compacted without being held. The per-row Records a buffered Compaction
// also carries have no place here; a streamed row is written to its destination
// and released.
//
// Memory is O(columns), independent of the row count.
type CompactionAccumulator struct {
	rows      int
	seen      map[string]bool
	hasValue  map[string]bool
	candidate map[string]interface{}
	nulls     int
}

// NewCompactionAccumulator creates an accumulator. Reuse one per result; it is
// not safe for concurrent use.
func NewCompactionAccumulator() *CompactionAccumulator {
	return &CompactionAccumulator{seen: map[string]bool{}, hasValue: map[string]bool{}}
}

// Observe folds a single row in.
func (a *CompactionAccumulator) Observe(rec map[string]interface{}) {
	for k, v := range rec {
		a.seen[k] = true
		if v != nil {
			a.hasValue[k] = true
		} else {
			a.nulls++
		}
	}

	if a.rows == 0 {
		a.candidate = make(map[string]interface{}, len(rec))
		for k, v := range rec {
			if v != nil {
				a.candidate[k] = v
			}
		}
		a.rows++
		return
	}

	for k, v := range a.candidate {
		other, ok := rec[k]
		if !ok || other == nil || !reflect.DeepEqual(v, other) {
			delete(a.candidate, k)
		}
	}
	a.rows++
}

// Finalize produces the compaction the observed rows imply. It matches what
// CompactRecords would have computed over the same rows, minus Records: a
// single row yields no Constant (hoisting it would only move it), and columns
// that were null or absent everywhere are named rather than profiled.
func (a *CompactionAccumulator) Finalize() Compaction {
	c := Compaction{Records: []map[string]interface{}{}, DroppedNulls: a.nulls}
	for k := range a.seen {
		if !a.hasValue[k] {
			c.NullColumns = append(c.NullColumns, k)
		}
	}
	sort.Strings(c.NullColumns)
	if a.rows >= 2 && len(a.candidate) > 0 {
		c.Constant = a.candidate
	}
	return c
}
