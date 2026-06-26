package inspect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// jsonReader streams a JSON file written by the `-o json` printer: a single
// top-level array of record objects. It reads elements one at a time with a
// json.Decoder (read '[', then decode each element on demand) so peak memory is
// one record rather than the whole array — important for a large spilled result.
type jsonReader struct {
	c    io.Closer
	dec  *json.Decoder
	path string
	q    string
	done bool
}

func newJSONReader(rc io.ReadCloser, path, query string) (*jsonReader, error) {
	dec := json.NewDecoder(bufio.NewReaderSize(rc, 64*1024))
	// Consume the opening '['. A JSON document that is not an array is not a
	// shape inspect can stream as records → unreadable.
	tok, err := dec.Token()
	if err != nil {
		return nil, errUnreadable(path, query, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, errUnreadable(path, query, fmt.Errorf("expected a JSON array of records, got %v", tok))
	}
	return &jsonReader{c: rc, dec: dec, path: path, q: query}, nil
}

// Columns returns nil: a JSON array exposes no schema header.
func (r *jsonReader) Columns() []string { return nil }

func (r *jsonReader) Next() (map[string]interface{}, error) {
	if r.done || !r.dec.More() {
		r.done = true
		return nil, io.EOF
	}
	var rec map[string]interface{}
	if err := r.dec.Decode(&rec); err != nil {
		return nil, errUnreadable(r.path, r.q, err)
	}
	return rec, nil
}

func (r *jsonReader) Close() error { return r.c.Close() }
