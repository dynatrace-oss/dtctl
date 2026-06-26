package inspect

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
)

// ndjsonReader streams an NDJSON / JSON Lines file (the default spill format,
// D26): one JSON object per line. It reads line by line so peak memory is one
// record, and uses a buffered reader (not bufio.Scanner) so an arbitrarily long
// line — a log record with a large `content` field is common — is never
// truncated by the scanner's token cap.
type ndjsonReader struct {
	c  io.Closer
	br *bufio.Reader
}

func newNDJSONReader(rc io.ReadCloser) *ndjsonReader {
	return &ndjsonReader{c: rc, br: bufio.NewReaderSize(rc, 64*1024)}
}

// Columns returns nil: NDJSON carries no schema header, so the column set is
// only knowable by scanning rows. The engine falls back to the sidecar schema or
// best-effort discovery for --fields validation.
func (r *ndjsonReader) Columns() []string { return nil }

func (r *ndjsonReader) Next() (map[string]interface{}, error) {
	for {
		line, err := r.readLine()
		if err == io.EOF && len(line) == 0 {
			return nil, io.EOF
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			if err == io.EOF {
				return nil, io.EOF
			}
			continue // skip blank lines (e.g. a trailing newline)
		}
		var rec map[string]interface{}
		if jerr := json.Unmarshal(trimmed, &rec); jerr != nil {
			return nil, jerr
		}
		return rec, nil
	}
}

// readLine reads a full line (including the trailing newline when present).
// bufio.ReadBytes accumulates across buffer boundaries and grows as needed, so a
// very long line is never truncated; the terminal line without a newline is
// returned together with io.EOF.
func (r *ndjsonReader) readLine() ([]byte, error) {
	return r.br.ReadBytes('\n')
}

func (r *ndjsonReader) Close() error { return r.c.Close() }
