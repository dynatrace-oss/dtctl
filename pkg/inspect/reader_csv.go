package inspect

import (
	"encoding/csv"
	"io"
)

// csvReader streams a CSV file written by the `-o csv` printer: a header row of
// sorted column names followed by one row per record. CSV is all-strings, so
// every cell is returned as a string and an empty cell as "" (CSV cannot
// distinguish null from empty — a documented best-effort limitation, IN2). The
// header gives the schema cheaply, so Columns() is authoritative for --fields
// validation even without a sidecar.
type csvReader struct {
	c      io.Closer
	r      *csv.Reader
	header []string
	path   string
	q      string
}

func newCSVReader(rc io.ReadCloser, path, query string) (*csvReader, error) {
	cr := csv.NewReader(rc)
	cr.FieldsPerRecord = -1 // tolerate ragged rows rather than erroring the read
	cr.ReuseRecord = true   // we copy what we keep; lets the reader reuse its buffer
	header, err := cr.Read()
	if err != nil {
		if err == io.EOF {
			// An empty CSV file (no header) is a valid empty result: no columns,
			// no rows. Represent it as a reader that yields nothing.
			return &csvReader{c: rc, r: cr, header: nil, path: path, q: query}, nil
		}
		return nil, errUnreadable(path, query, err)
	}
	// Copy the header out of the reader's reused buffer.
	cols := make([]string, len(header))
	copy(cols, header)
	return &csvReader{c: rc, r: cr, header: cols, path: path, q: query}, nil
}

func (r *csvReader) Columns() []string { return r.header }

func (r *csvReader) Next() (map[string]interface{}, error) {
	if r.header == nil {
		return nil, io.EOF
	}
	row, err := r.r.Read()
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, errUnreadable(r.path, r.q, err)
	}
	rec := make(map[string]interface{}, len(r.header))
	for i, col := range r.header {
		if i < len(row) {
			rec[col] = row[i]
		} else {
			rec[col] = "" // ragged short row → empty cell
		}
	}
	return rec, nil
}

func (r *csvReader) Close() error { return r.c.Close() }
