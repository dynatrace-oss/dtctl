package inspect

import (
	"io"
	"os"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/format"
)

// parquetReader streams a Parquet file written by the `-o parquet` printer. It
// reads row groups in bounded batches (parquet-go has no map-based generic
// reader for an arbitrary schema, so it maps each leaf value back to its column
// name itself), so peak memory is one batch, not the whole file.
//
// Timestamp columns are written as INT64 TIMESTAMP logical-type values
// (nanoseconds); this reader converts them back to RFC3339 strings so they
// round-trip to the same shape the other formats carry and the stats accumulator
// recognises them as timestamps rather than as opaque longs.
type parquetReader struct {
	c      io.Closer
	file   *parquet.File
	names  []string      // leaf column index → name
	tsMult map[int]int64 // leaf column index → ns multiplier for TIMESTAMP columns
	path   string
	q      string

	rowGroups []parquet.RowGroup
	rgIndex   int
	rows      parquet.Rows
	buf       []parquet.Row
	pending   []parquet.Row // decoded rows not yet served
	eof       bool
}

func newParquetReader(f *os.File, path, query string) (*parquetReader, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, errUnreadable(path, query, err)
	}
	pf, err := parquet.OpenFile(f, info.Size())
	if err != nil {
		return nil, errUnreadable(path, query, err)
	}

	cols := pf.Schema().Columns()
	names := make([]string, len(cols))
	for i, p := range cols {
		names[i] = p[len(p)-1] // flat schema: one element per leaf path
	}

	// Detect TIMESTAMP logical-type columns and their unit so int64 values can be
	// converted back to wall-clock strings.
	tsMult := make(map[int]int64)
	for i, p := range cols {
		if leaf, ok := pf.Schema().Lookup(p...); ok {
			if lt := leaf.Node.Type().LogicalType(); lt != nil && lt.Timestamp != nil {
				tsMult[i] = nsMultiplier(lt.Timestamp.Unit)
			}
		}
	}

	return &parquetReader{
		c:         f,
		file:      pf,
		names:     names,
		tsMult:    tsMult,
		path:      path,
		q:         query,
		rowGroups: pf.RowGroups(),
		buf:       make([]parquet.Row, 64),
	}, nil
}

// Columns returns the schema leaf names — authoritative for --fields validation
// without needing a sidecar.
func (r *parquetReader) Columns() []string { return r.names }

func (r *parquetReader) Next() (map[string]interface{}, error) {
	for len(r.pending) == 0 {
		if err := r.fill(); err != nil {
			return nil, err // io.EOF or a read error
		}
	}
	row := r.pending[0]
	r.pending = r.pending[1:]
	return r.rowToMap(row), nil
}

// fill decodes the next batch of rows into r.pending, advancing across row
// groups. It returns io.EOF when the whole file is exhausted.
func (r *parquetReader) fill() error {
	if r.eof {
		return io.EOF
	}
	for {
		if r.rows == nil {
			if r.rgIndex >= len(r.rowGroups) {
				r.eof = true
				return io.EOF
			}
			r.rows = r.rowGroups[r.rgIndex].Rows()
			r.rgIndex++
		}
		n, err := r.rows.ReadRows(r.buf)
		if n > 0 {
			r.pending = make([]parquet.Row, n)
			for i := 0; i < n; i++ {
				r.pending[i] = r.buf[i].Clone()
			}
		}
		if err == io.EOF {
			_ = r.rows.Close()
			r.rows = nil
			if n > 0 {
				return nil
			}
			continue // exhausted this row group, try the next
		}
		if err != nil {
			return errUnreadable(r.path, r.q, err)
		}
		if n > 0 {
			return nil
		}
	}
}

func (r *parquetReader) rowToMap(row parquet.Row) map[string]interface{} {
	m := make(map[string]interface{}, len(r.names))
	for _, v := range row {
		col := v.Column()
		if col < 0 || col >= len(r.names) {
			continue
		}
		name := r.names[col]
		if v.IsNull() {
			m[name] = nil
			continue
		}
		switch v.Kind() {
		case parquet.Boolean:
			m[name] = v.Boolean()
		case parquet.Int64:
			if mult, ok := r.tsMult[col]; ok {
				m[name] = time.Unix(0, v.Int64()*mult).UTC().Format(time.RFC3339Nano)
			} else {
				m[name] = v.Int64()
			}
		case parquet.Double:
			m[name] = v.Double()
		default:
			m[name] = v.String()
		}
	}
	return m
}

func (r *parquetReader) Close() error {
	if r.rows != nil {
		_ = r.rows.Close()
	}
	return r.c.Close()
}

// nsMultiplier maps a parquet TIMESTAMP unit to the multiplier that converts a
// stored value to nanoseconds. The `-o parquet` writer always uses nanoseconds;
// the others are handled for files written by other tooling.
func nsMultiplier(unit format.TimeUnit) int64 {
	switch {
	case unit.Millis != nil:
		return int64(time.Millisecond)
	case unit.Micros != nil:
		return int64(time.Microsecond)
	default: // Nanos (and the writer's default)
		return 1
	}
}
