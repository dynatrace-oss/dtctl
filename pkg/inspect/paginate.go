package inspect

import (
	"io"
	"sort"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// readHead returns the first n records, projected to fields. It stops after n,
// so memory and time are O(n) — it never reads the rest of the file.
func readHead(r Reader, n int, fields []string) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, n)
	for len(out) < n {
		rec, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, wrapReadError(err)
		}
		out = append(out, project(rec, fields))
	}
	return out, nil
}

// readPage returns the window [offset, offset+limit), projected to fields. It
// skips offset records and emits up to limit, so memory is O(limit). Row order
// is file order, so pagination is stable across calls on the same file (IN7).
func readPage(r Reader, offset, limit int, fields []string) ([]map[string]interface{}, error) {
	if offset < 0 {
		offset = 0
	}
	skipped := 0
	for skipped < offset {
		_, err := r.Next()
		if err == io.EOF {
			return []map[string]interface{}{}, nil // offset past the end → empty window
		}
		if err != nil {
			return nil, wrapReadError(err)
		}
		skipped++
	}
	return readHead(r, limit, fields)
}

// readTail returns the last n records, projected to fields. It keeps an n-slot
// ring buffer while streaming, so memory is O(n) regardless of file size. (Time
// is O(file) for NDJSON/JSON — a reverse-read optimisation is possible but tail
// is rare; Parquet could read trailing row groups — both are future
// optimisations; correctness and bounded memory hold today.)
func readTail(r Reader, n int, fields []string) ([]map[string]interface{}, error) {
	if n <= 0 {
		return []map[string]interface{}{}, nil
	}
	ring := make([]map[string]interface{}, n)
	count := 0
	for {
		rec, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, wrapReadError(err)
		}
		ring[count%n] = project(rec, fields)
		count++
	}

	size := n
	if count < n {
		size = count
	}
	out := make([]map[string]interface{}, 0, size)
	start := count - size
	for i := start; i < count; i++ {
		out = append(out, ring[i%n])
	}
	return out, nil
}

// validateFields rejects a --fields name that is not a column in the file, but
// only when the schema is knowable up front: the reader's own schema (Parquet
// footer / CSV header) or, failing that, the sidecar's column list. For NDJSON /
// JSON with no sidecar the schema is only knowable by scanning, so validation is
// deferred to a post-hoc warning (missingFieldWarnings) rather than a hard error.
func validateFields(r Reader, sidecar *output.SidecarManifest, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	schema := knownSchema(r, sidecar)
	if schema == nil {
		return nil // unknowable up front; handled as a warning later
	}
	known := make(map[string]bool, len(schema))
	for _, c := range schema {
		known[c] = true
	}
	for _, f := range fields {
		if !known[f] {
			return errUnknownField(f, schema)
		}
	}
	return nil
}

// missingFieldWarnings reports requested --fields that never appeared in the
// returned rows when the schema could not be validated up front. It is advisory:
// without a full scan we cannot prove a field is absent from the whole file, so
// this is a warning, not an error.
func missingFieldWarnings(r Reader, sidecar *output.SidecarManifest, fields []string, rows []map[string]interface{}) []string {
	if len(fields) == 0 || knownSchema(r, sidecar) != nil {
		return nil
	}
	seen := make(map[string]bool)
	for _, rec := range rows {
		for k := range rec {
			seen[k] = true
		}
	}
	var missing []string
	for _, f := range fields {
		if !seen[f] {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return []string{"requested field(s) not present in the returned rows (the file has no schema header to validate against): " + joinColumns(missing)}
}

// knownSchema returns the authoritative up-front column set for --fields
// validation: the reader's own schema first (Parquet/CSV), then the sidecar's
// columns. nil means the schema is only knowable by scanning rows.
func knownSchema(r Reader, sidecar *output.SidecarManifest) []string {
	if cols := r.Columns(); cols != nil {
		return cols
	}
	if sidecar != nil && len(sidecar.Columns) > 0 {
		names := make([]string, len(sidecar.Columns))
		for i, c := range sidecar.Columns {
			names[i] = c.Name
		}
		return names
	}
	return nil
}

// schemaView trims full column stats to the schema-only fields (name, type, null
// count) that --schema reports.
func schemaView(cols []output.ColumnStats) []output.ColumnStats {
	out := make([]output.ColumnStats, len(cols))
	for i, c := range cols {
		out[i] = output.ColumnStats{Name: c.Name, Type: c.Type, Nulls: c.Nulls, Basis: c.Basis}
	}
	return out
}

// selectColumns filters computed stats to the requested column names, preserving
// the request order. An unknown name is an inspect_unknown_field error listing
// the available columns.
func selectColumns(cols []output.ColumnStats, want []string) ([]output.ColumnStats, error) {
	byName := make(map[string]output.ColumnStats, len(cols))
	available := make([]string, len(cols))
	for i, c := range cols {
		byName[c.Name] = c
		available[i] = c.Name
	}
	out := make([]output.ColumnStats, 0, len(want))
	for _, name := range want {
		c, ok := byName[name]
		if !ok {
			return nil, errUnknownField(name, available)
		}
		out = append(out, c)
	}
	return out, nil
}

// wrapReadError normalises a low-level read failure into an unreadable-file
// error unless it is already a typed inspect error (readers may already wrap).
func wrapReadError(err error) error {
	if ie, ok := err.(*Error); ok {
		return ie
	}
	return errUnreadable("", "", err)
}
