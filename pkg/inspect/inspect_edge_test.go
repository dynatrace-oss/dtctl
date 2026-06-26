package inspect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

// writeRaw writes exact bytes to dir/name (so a test can craft empty, blank-line,
// non-array, or corrupt files the printers would never produce), optionally with a
// sidecar declaring the format authoritatively.
func writeRaw(t *testing.T, dir, name, content string, sc *output.SidecarManifest) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if sc != nil {
		if err := output.WriteSidecar(path, sc); err != nil {
			t.Fatalf("sidecar: %v", err)
		}
	}
	return path
}

// errCode runs req and asserts it fails with a *Error carrying the wanted code.
func errCode(t *testing.T, req Request, want string) {
	t.Helper()
	_, err := Run(req)
	ie, ok := err.(*Error)
	if !ok {
		t.Fatalf("err = %v (%T), want *inspect.Error with code %q", err, err, want)
	}
	if ie.Code != want {
		t.Fatalf("err code = %q, want %q (msg: %s)", ie.Code, want, ie.Message)
	}
}

// TestRun_TailAndPageBoundaries covers the count<n tail branch and an offset past
// the end (both untested by the happy-path suite).
func TestRun_TailAndPageBoundaries(t *testing.T) {
	dir := t.TempDir()
	path := writeSpill(t, dir, "jsonl", sampleRecords(), &output.SidecarManifest{Format: "jsonl", Rows: 4})

	// tail N larger than the file → all rows, in file order.
	tail, err := Run(Request{Path: path, Primitive: PrimTail, N: 100})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(tail.Records) != 4 || tail.Records[0]["host"] != "web-01" || tail.Records[3]["host"] != "web-03" {
		t.Errorf("tail(100) = %v, want all 4 rows in order", tail.Records)
	}

	// page offset past the end → empty window, not an error.
	page, err := Run(Request{Path: path, Primitive: PrimPage, Offset: 100, Limit: 10})
	if err != nil {
		t.Fatalf("page past end: %v", err)
	}
	if len(page.Records) != 0 {
		t.Errorf("page past end = %v, want empty", page.Records)
	}

	// tail N<=0 → empty (defensive; the CLI defaults this, but the engine must hold).
	zero, err := Run(Request{Path: path, Primitive: PrimTail, N: -1})
	if err != nil {
		t.Fatalf("tail(-1): %v", err)
	}
	// rowCount() promotes a non-positive N to DefaultRowCount, so this returns all rows.
	if len(zero.Records) != 4 {
		t.Errorf("tail(-1) = %d rows, want 4 (defaulted)", len(zero.Records))
	}
}

// TestRun_EmptyFiles ensures an empty NDJSON and an empty (headerless) CSV both
// read as a valid zero-row result rather than erroring.
func TestRun_EmptyFiles(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, content, format string }{
		{"q-empty.jsonl", "", "jsonl"},
		{"q-blanklines.jsonl", "\n\n", "jsonl"},
		{"q-empty.csv", "", "csv"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeRaw(t, dir, tc.name, tc.content, &output.SidecarManifest{Format: tc.format})
			res, err := Run(Request{Path: path, Primitive: PrimHead, N: 10})
			if err != nil {
				t.Fatalf("head: %v", err)
			}
			if len(res.Records) != 0 {
				t.Errorf("rows = %d, want 0 for empty file", len(res.Records))
			}
		})
	}
}

// TestRun_NDJSONBlankLinesBetweenRecords confirms blank lines interleaved with
// records (and a missing trailing newline) are tolerated.
func TestRun_NDJSONBlankLinesBetweenRecords(t *testing.T) {
	dir := t.TempDir()
	// Two records separated by a blank line, last line has no trailing newline.
	path := writeRaw(t, dir, "q-gaps.jsonl",
		"{\"a\":1}\n\n{\"a\":2}", &output.SidecarManifest{Format: "jsonl"})
	res, err := Run(Request{Path: path, Primitive: PrimHead, N: 10})
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if len(res.Records) != 2 {
		t.Fatalf("rows = %d, want 2 (blank line skipped)", len(res.Records))
	}
	if res.Records[1]["a"] != float64(2) {
		t.Errorf("last record = %v, want a=2", res.Records[1])
	}
}

// TestRun_UnreadableAndUndeterminable covers the error contract for files the
// reader cannot stream as records, and for a path whose format cannot be resolved.
func TestRun_UnreadableAndUndeterminable(t *testing.T) {
	dir := t.TempDir()

	// A JSON document that is an object, not an array of records → unreadable.
	obj := writeRaw(t, dir, "q-obj.json", `{"not":"an array"}`, &output.SidecarManifest{Format: "json"})
	errCode(t, Request{Path: obj, Primitive: PrimHead}, output.ErrCodeSpillFileUnreadable)

	// Garbage bytes declared as parquet → the reader fails to initialise → unreadable.
	pq := writeRaw(t, dir, "q-corrupt.parquet", "definitely not parquet", &output.SidecarManifest{Format: "parquet"})
	errCode(t, Request{Path: pq, Primitive: PrimHead}, output.ErrCodeSpillFileUnreadable)

	// No sidecar and an unrecognised extension → the format is undeterminable → bad flags.
	txt := writeRaw(t, dir, "q-mystery.txt", "{}", nil)
	errCode(t, Request{Path: txt, Primitive: PrimHead}, codeBadFlags)
}

// TestRun_FormatFromExtensionWithoutSidecar exercises extension-based format
// inference (the no-sidecar fallback) for csv, including header-derived schema.
func TestRun_FormatFromExtensionWithoutSidecar(t *testing.T) {
	dir := t.TempDir()
	// A real CSV with no sidecar: format is inferred from .csv, the header gives
	// the schema, and --fields validates against it (the authoritative-Columns path).
	path := writeSpill(t, dir, "csv", sampleRecords(), nil)
	res, err := Run(Request{Path: path, Primitive: PrimHead, N: 2, Fields: []string{"host"}})
	if err != nil {
		t.Fatalf("csv head: %v", err)
	}
	if res.Format != "csv" {
		t.Errorf("format = %q, want csv (inferred from extension)", res.Format)
	}
	if len(res.Records) != 2 || len(res.Records[0]) != 1 || res.Records[0]["host"] == nil {
		t.Errorf("projected csv rows = %v, want host-only", res.Records)
	}

	// An unknown --fields name is a hard error against the CSV header even without a sidecar.
	errCode(t, Request{Path: path, Primitive: PrimHead, Fields: []string{"ghost"}}, codeUnknownField)
}

// TestRun_CrossTenantUserPath covers the sidecar-tenant branch of checkContext:
// a non-managed (user-chosen) path is refused when its sidecar tenant disagrees
// with the active tenant.
func TestRun_CrossTenantUserPath(t *testing.T) {
	dir := t.TempDir() // user-chosen location, not the managed cache
	path := writeSpill(t, dir, "jsonl", sampleRecords(), &output.SidecarManifest{
		Format: "jsonl", TenantID: "tenant-a", ContextName: "ctx-a",
	})

	// Active tenant differs → refuse.
	errCode(t, Request{Path: path, Primitive: PrimHead, ActiveTenant: "tenant-b"},
		output.ErrCodeSpillFileWrongContext)

	// Same tenant → allowed.
	res, err := Run(Request{Path: path, Primitive: PrimHead, N: 1, ActiveTenant: "tenant-a"})
	if err != nil {
		t.Fatalf("same-tenant head: %v", err)
	}
	if len(res.Records) != 1 {
		t.Errorf("rows = %d, want 1", len(res.Records))
	}
}

// TestRun_StatsColumnSelection covers selectColumns: restricting --stats to named
// columns, and the unknown-column error.
func TestRun_StatsColumnSelection(t *testing.T) {
	dir := t.TempDir()
	path := writeSpill(t, dir, "jsonl", sampleRecords(), &output.SidecarManifest{Format: "jsonl", Rows: 4})

	res, err := Run(Request{Path: path, Primitive: PrimStats, StatsColumns: []string{"host"}})
	if err != nil {
		t.Fatalf("stats select: %v", err)
	}
	if res.Summary == nil || len(res.Summary.Columns) != 1 || res.Summary.Columns[0].Name != "host" {
		t.Fatalf("selected stats = %+v, want only host", res.Summary)
	}

	errCode(t, Request{Path: path, Primitive: PrimStats, StatsColumns: []string{"nope"}}, codeUnknownField)
}

// TestRun_DeferredFieldsWarning covers the NDJSON-without-schema path: an unknown
// --fields name cannot be validated up front, so it is not an error but produces
// a missing-field warning; a present field produces none.
func TestRun_DeferredFieldsWarning(t *testing.T) {
	dir := t.TempDir()
	path := writeSpill(t, dir, "jsonl", sampleRecords(), nil) // no sidecar → schema unknowable up front

	// Unknown field: no error, but a warning.
	missing, err := Run(Request{Path: path, Primitive: PrimHead, N: 2, Fields: []string{"nonexistent"}})
	if err != nil {
		t.Fatalf("unexpected error for deferred field: %v", err)
	}
	if len(missing.Warnings) == 0 {
		t.Errorf("expected a missing-field warning for an absent field on a schemaless file")
	}

	// Present field: no warning.
	present, err := Run(Request{Path: path, Primitive: PrimHead, N: 2, Fields: []string{"host"}})
	if err != nil {
		t.Fatalf("present field: %v", err)
	}
	if len(present.Warnings) != 0 {
		t.Errorf("unexpected warnings for a present field: %v", present.Warnings)
	}
}

// TestRun_ParquetProjectionTailPage exercises Parquet beyond head: --fields
// projection (which Parquet applies after decode), tail, and a page window.
func TestRun_ParquetProjectionTailPage(t *testing.T) {
	dir := t.TempDir()
	recs := []map[string]interface{}{
		{"host": "web-01", "count": float64(10)},
		{"host": "web-02", "count": float64(20)},
		{"host": "web-03", "count": float64(30)},
	}
	path := filepath.Join(dir, "q-p.parquet")
	f, _ := os.Create(path)
	p := output.NewPrinterWithOpts(output.PrinterOptions{Format: "parquet", Writer: f, Types: []output.ColumnTypeMapping{
		{Name: "host", Type: "string"}, {Name: "count", Type: "long"},
	}})
	if err := p.PrintList(recs); err != nil {
		t.Fatalf("write parquet: %v", err)
	}
	_ = f.Close()
	_ = output.WriteSidecar(path, &output.SidecarManifest{Format: "parquet", Rows: 3})

	// Projection: keep only host.
	proj, err := Run(Request{Path: path, Primitive: PrimHead, N: 3, Fields: []string{"host"}})
	if err != nil {
		t.Fatalf("parquet head+fields: %v", err)
	}
	if len(proj.Records) != 3 || len(proj.Records[0]) != 1 || proj.Records[0]["host"] == nil {
		t.Errorf("projected parquet rows = %v, want host-only", proj.Records)
	}

	// Tail.
	tail, err := Run(Request{Path: path, Primitive: PrimTail, N: 1})
	if err != nil {
		t.Fatalf("parquet tail: %v", err)
	}
	if len(tail.Records) != 1 || tail.Records[0]["host"] != "web-03" {
		t.Errorf("parquet tail = %v, want web-03", tail.Records)
	}

	// Page window [1,3).
	page, err := Run(Request{Path: path, Primitive: PrimPage, Offset: 1, Limit: 2})
	if err != nil {
		t.Fatalf("parquet page: %v", err)
	}
	if len(page.Records) != 2 || page.Records[0]["host"] != "web-02" {
		t.Errorf("parquet page = %v, want [web-02, web-03]", page.Records)
	}
	// Parquet integers round-trip as int64.
	if page.Records[0]["count"] != int64(20) {
		t.Errorf("count = %#v, want int64(20)", page.Records[0]["count"])
	}
}

// TestRun_Sample covers the sample primitive end to end (leading N rows as a
// file-summary, with embedded sample_rows).
func TestRun_Sample(t *testing.T) {
	dir := t.TempDir()
	path := writeSpill(t, dir, "jsonl", sampleRecords(), &output.SidecarManifest{Format: "jsonl", Rows: 4})
	res, err := Run(Request{Path: path, Primitive: PrimSample, N: 2})
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if res.Kind != output.KindFileSummary || res.Summary == nil {
		t.Fatalf("sample kind = %q", res.Kind)
	}
	if len(res.Summary.SampleRows) != 2 {
		t.Errorf("sample_rows = %d, want 2", len(res.Summary.SampleRows))
	}
}

// TestRun_NDJSONCorruptMidStream confirms a malformed record encountered while
// streaming surfaces as an unreadable-file error (the wrapReadError path), rather
// than silently truncating the result.
func TestRun_NDJSONCorruptMidStream(t *testing.T) {
	dir := t.TempDir()
	// First line valid, second line malformed JSON.
	path := writeRaw(t, dir, "q-corrupt.jsonl",
		"{\"a\":1}\n{not valid json}\n", &output.SidecarManifest{Format: "jsonl"})
	// N large enough to reach the bad line.
	errCode(t, Request{Path: path, Primitive: PrimHead, N: 10}, output.ErrCodeSpillFileUnreadable)
}

// TestRun_ParquetScalarKinds covers the boolean and double branches of the Parquet
// value→map conversion (the int64/timestamp branches are covered elsewhere).
func TestRun_ParquetScalarKinds(t *testing.T) {
	dir := t.TempDir()
	recs := []map[string]interface{}{
		{"ok": true, "ratio": float64(1.5)},
		{"ok": false, "ratio": float64(2.25)},
	}
	path := filepath.Join(dir, "q-kinds.parquet")
	f, _ := os.Create(path)
	p := output.NewPrinterWithOpts(output.PrinterOptions{Format: "parquet", Writer: f, Types: []output.ColumnTypeMapping{
		{Name: "ok", Type: "boolean"}, {Name: "ratio", Type: "double"},
	}})
	if err := p.PrintList(recs); err != nil {
		t.Fatalf("write parquet: %v", err)
	}
	_ = f.Close()
	_ = output.WriteSidecar(path, &output.SidecarManifest{Format: "parquet", Rows: 2})

	res, err := Run(Request{Path: path, Primitive: PrimHead, N: 2})
	if err != nil {
		t.Fatalf("parquet head: %v", err)
	}
	if res.Records[0]["ok"] != true || res.Records[1]["ok"] != false {
		t.Errorf("boolean round-trip = %v / %v, want true / false", res.Records[0]["ok"], res.Records[1]["ok"])
	}
	if res.Records[0]["ratio"] != float64(1.5) {
		t.Errorf("double round-trip = %#v, want 1.5", res.Records[0]["ratio"])
	}
}

// TestRun_StatsMixedAndComplexColumns confirms the streaming stats path classifies
// type-mixed and nested columns as "complex" (never crashing on a variant column)
// and reports timestamps as a timestamp column.
func TestRun_StatsMixedAndComplexColumns(t *testing.T) {
	dir := t.TempDir()
	recs := []map[string]interface{}{
		{"mixed": float64(1), "nested": map[string]interface{}{"k": "v"}, "ts": "2026-06-01T10:00:00Z"},
		{"mixed": "two", "nested": map[string]interface{}{"k": "w"}, "ts": "2026-06-01T11:00:00Z"},
	}
	path := writeSpill(t, dir, "jsonl", recs, &output.SidecarManifest{Format: "jsonl", Rows: 2})
	res, err := Run(Request{Path: path, Primitive: PrimStats})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	byName := map[string]output.ColumnStats{}
	for _, c := range res.Summary.Columns {
		byName[c.Name] = c
	}
	if byName["mixed"].Type != "complex" {
		t.Errorf("mixed type = %q, want complex", byName["mixed"].Type)
	}
	if byName["nested"].Type != "complex" {
		t.Errorf("nested type = %q, want complex", byName["nested"].Type)
	}
	if byName["ts"].Type != "timestamp" {
		t.Errorf("ts type = %q, want timestamp", byName["ts"].Type)
	}
}
