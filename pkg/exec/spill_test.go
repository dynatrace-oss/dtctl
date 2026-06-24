package exec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	sdkquery "github.com/dynatrace-oss/dtctl/sdk/api/query"
)

func TestParseByteSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		err  bool
	}{
		{"50KB", 50 * 1024, false},
		{"50kb", 50 * 1024, false},
		{"50k", 50 * 1024, false},
		{"1MB", 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"1.5KiB", 1536, false},
		{"1024", 1024, false},
		{"512B", 512, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-5KB", 0, true},
	}
	for _, c := range cases {
		got, err := ParseByteSize(c.in)
		if c.err {
			if err == nil {
				t.Errorf("ParseByteSize(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseByteSize(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseByteSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func sampleResult(sampled bool) (*DQLQueryResponse, []map[string]interface{}) {
	records := []map[string]interface{}{
		{"host": "web-01", "status": float64(200)},
		{"host": "web-02", "status": float64(500)},
		{"host": "web-01", "status": float64(404)},
	}
	resp := &DQLQueryResponse{
		Records: records,
		Metadata: &DQLMetadata{
			Grail: &GrailMetadata{
				Sampled:        sampled,
				CanonicalQuery: "fetch logs",
				AnalysisTimeframe: &AnalysisTimeframe{
					Start: "2026-06-21T00:00:00Z",
					End:   "2026-06-22T00:00:00Z",
				},
			},
		},
	}
	return resp, records
}

func TestBuildSpillResponse_InlineUnderThreshold(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(false)
	opts := DQLExecuteOptions{
		Spill: SpillOptions{Mode: SpillAuto, Threshold: 1 << 20, Dir: t.TempDir(), Format: "json"},
	}
	_, spilled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spilled {
		t.Errorf("expected inline (under threshold), got spilled")
	}
}

func TestBuildSpillResponse_SpillAlways(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(false)
	dir := t.TempDir()
	opts := DQLExecuteOptions{
		ContextName: "prod",
		TenantID:    "abc12345",
		Spill:       SpillOptions{Mode: SpillAlways, Threshold: 1 << 20, Dir: dir, Format: "json"},
	}
	resp, spilled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spilled {
		t.Fatal("expected spilled")
	}
	if resp.EnvelopeVersion != output.EnvelopeVersion {
		t.Errorf("envelope_version = %d, want %d", resp.EnvelopeVersion, output.EnvelopeVersion)
	}
	if !resp.OK {
		t.Error("resp.OK should be true")
	}

	m, ok := resp.Result.(*output.ResultFileManifest)
	if !ok {
		t.Fatalf("result is %T, want *ResultFileManifest", resp.Result)
	}
	if m.Kind != output.KindResultFile {
		t.Errorf("kind = %q, want result-file", m.Kind)
	}
	if m.Rows != 3 {
		t.Errorf("rows = %d, want 3", m.Rows)
	}
	if m.TenantID != "abc12345" || m.ContextName != "prod" {
		t.Errorf("provenance = %q/%q", m.TenantID, m.ContextName)
	}
	if m.Path == "" {
		t.Fatal("path is empty")
	}
	if m.Columns == nil || m.SampleStats != nil {
		t.Errorf("non-sampled result should use columns, not sample_stats")
	}
	if len(m.SampleRows) == 0 {
		t.Error("expected sample_rows")
	}

	// File exists on disk (under the user-chosen dir; not context-partitioned)
	// and parses back.
	if !filepath.IsAbs(m.Path) {
		t.Errorf("path should be absolute: %q", m.Path)
	}
	if !strings.HasPrefix(m.Path, dir) {
		t.Errorf("spill path %q should be under user dir %q", m.Path, dir)
	}
	data, rerr := os.ReadFile(m.Path)
	if rerr != nil {
		t.Fatalf("read spilled file: %v", rerr)
	}
	var back []map[string]interface{}
	if jerr := json.Unmarshal(data, &back); jerr != nil {
		t.Fatalf("spilled json invalid: %v", jerr)
	}
	if len(back) != 3 {
		t.Errorf("spilled rows = %d, want 3", len(back))
	}
	if m.Bytes != int64(len(data)) {
		t.Errorf("manifest bytes = %d, file size = %d", m.Bytes, len(data))
	}

	// Sidecar written next to it.
	if _, serr := os.Stat(output.SidecarPathFor(m.Path)); serr != nil {
		t.Errorf("sidecar not written: %v", serr)
	}

	// Context provenance.
	if resp.Context.Decided != "spilled" {
		t.Errorf("decided = %q, want spilled", resp.Context.Decided)
	}
	if resp.Context.MeasuredEncoding != "json" {
		t.Errorf("measured_encoding = %q, want json", resp.Context.MeasuredEncoding)
	}
	if resp.Context.MeasuredBytes <= 0 {
		t.Errorf("measured_bytes = %d, want > 0", resp.Context.MeasuredBytes)
	}
	if len(resp.Context.Suggestions) == 0 {
		t.Error("expected suggestions")
	}
}

func TestBuildSpillResponse_WideResultCapsEnvelopeKeepsSidecarFull(t *testing.T) {
	e := &DQLExecutor{}
	// A wide result: more columns than the envelope cap. Every column is present
	// in every row (equal null counts) so the kept set is the alphabetically
	// first DefaultMaxSummaryColumns and the rest are omitted by name.
	const ncols = output.DefaultMaxSummaryColumns + 12
	rec := make(map[string]interface{}, ncols)
	for i := 0; i < ncols; i++ {
		rec[fmt.Sprintf("col%03d", i)] = fmt.Sprintf("v%d", i)
	}
	records := []map[string]interface{}{rec, rec, rec}
	result := &DQLQueryResponse{Records: records}

	dir := t.TempDir()
	opts := DQLExecuteOptions{
		ContextName: "prod",
		Spill:       SpillOptions{Mode: SpillAlways, Threshold: 1 << 20, Dir: dir, Format: "json"},
	}
	resp, handled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil || !handled {
		t.Fatalf("buildSpillResponse: handled=%v err=%v", handled, err)
	}
	m := resp.Result.(*output.ResultFileManifest)

	// Envelope is capped, and the kept + omitted partition covers every column.
	if len(m.Columns) != output.DefaultMaxSummaryColumns {
		t.Errorf("envelope columns = %d, want %d (capped)", len(m.Columns), output.DefaultMaxSummaryColumns)
	}
	if len(m.ColumnsOmitted) != ncols-output.DefaultMaxSummaryColumns {
		t.Errorf("columns_omitted = %d, want %d", len(m.ColumnsOmitted), ncols-output.DefaultMaxSummaryColumns)
	}
	if len(m.Columns)+len(m.ColumnsOmitted) != ncols {
		t.Errorf("kept(%d) + omitted(%d) != total %d", len(m.Columns), len(m.ColumnsOmitted), ncols)
	}

	// The on-disk sidecar keeps the FULL per-column set (nothing dropped on disk).
	scData, rerr := os.ReadFile(output.SidecarPathFor(m.Path))
	if rerr != nil {
		t.Fatalf("read sidecar: %v", rerr)
	}
	var sc output.SidecarManifest
	if jerr := json.Unmarshal(scData, &sc); jerr != nil {
		t.Fatalf("sidecar json invalid: %v", jerr)
	}
	if len(sc.Columns) != ncols {
		t.Errorf("sidecar columns = %d, want full %d", len(sc.Columns), ncols)
	}

	// The omission is surfaced to the consumer as a suggestion.
	var noted bool
	for _, s := range resp.Context.Suggestions {
		if strings.Contains(s, "columns_omitted") {
			noted = true
		}
	}
	if !noted {
		t.Error("expected a suggestion pointing at columns_omitted")
	}
}

func TestBuildSpillResponse_InlineRecordsEnvelopeInAgentMode(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(false)
	opts := DQLExecuteOptions{
		AgentMode: true,
		Spill:     SpillOptions{Mode: SpillAuto, Threshold: 1 << 20, Dir: t.TempDir(), Format: "json"},
	}
	resp, handled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("agent-mode inline result should be handled (kind:records envelope), not a fall-through")
	}
	if resp.EnvelopeVersion != output.EnvelopeVersion || !resp.OK {
		t.Errorf("envelope_version=%d ok=%v", resp.EnvelopeVersion, resp.OK)
	}
	ir, ok := resp.Result.(*output.InlineRecords)
	if !ok {
		t.Fatalf("result is %T, want *InlineRecords", resp.Result)
	}
	if ir.Kind != output.KindRecords {
		t.Errorf("kind = %q, want records", ir.Kind)
	}
	if len(ir.Records) != 3 {
		t.Errorf("records len = %d, want 3", len(ir.Records))
	}
	if resp.Context.Decided != "inline" {
		t.Errorf("decided = %q, want inline", resp.Context.Decided)
	}
	// The whole point of D2/D31: a consumer can find result.kind in the inline case too.
	js, _ := json.Marshal(resp)
	if !strings.Contains(string(js), `"kind":"records"`) {
		t.Errorf("inline envelope missing kind discriminator:\n%s", js)
	}
}

func TestBuildSpillResponse_NeverModeInlinesInAgentMode(t *testing.T) {
	// --spill=never forces rows inline regardless of size; in agent mode that
	// still means a self-describing kind:"records" envelope, never a table. Use
	// a Threshold of 0 to prove size is irrelevant under never (auto would spill).
	e := &DQLExecutor{}
	result, records := sampleResult(false)
	opts := DQLExecuteOptions{
		AgentMode: true,
		Spill:     SpillOptions{Mode: SpillNever, Threshold: 0, Dir: t.TempDir(), Format: "json"},
	}
	resp, handled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("never + agent mode should emit a kind:records envelope, not a fall-through")
	}
	ir, ok := resp.Result.(*output.InlineRecords)
	if !ok || ir.Kind != output.KindRecords {
		t.Fatalf("result = %T (kind unexpected), want *InlineRecords kind:records", resp.Result)
	}
	if resp.Context.Decided != "inline" {
		t.Errorf("decided = %q, want inline", resp.Context.Decided)
	}
}

func TestBuildSpillResponse_InlineFallThroughCases(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(false)
	base := SpillOptions{Mode: SpillAuto, Threshold: 1 << 20, Dir: t.TempDir(), Format: "json"}

	cases := []struct {
		name     string
		opts     DQLExecuteOptions
		encoding string
	}{
		// Not agent mode: a human inline result must stay a fall-through (table/CSV).
		{"non-agent", DQLExecuteOptions{Spill: base}, "json"},
		// Non-JSON display encoding: wrapping would discard the requested format.
		{"toon-encoding", DQLExecuteOptions{AgentMode: true, Spill: base}, "toon"},
		// --jq owns the output shape in agent mode.
		{"jq-set", DQLExecuteOptions{AgentMode: true, JQFilter: ".[]", Spill: base}, "json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, handled, err := e.buildSpillResponse("fetch logs", result, records, c.encoding, c.opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if handled {
				t.Errorf("%s: expected fall-through (handled=false), got an envelope", c.name)
			}
		})
	}
}

func TestBuildSpillResponse_ManagedPartition(t *testing.T) {
	// Redirect the OS user cache dir into the test sandbox so the managed-cache
	// path (D7) is exercised hermetically and partitioned by context (D9).
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp) // Linux
	t.Setenv("HOME", tmp)           // macOS uses $HOME/Library/Caches
	t.Setenv("LocalAppData", tmp)   // Windows: os.UserCacheDir reads %LocalAppData%

	e := &DQLExecutor{}
	result, records := sampleResult(false)
	opts := DQLExecuteOptions{
		ContextName: "prod",
		Spill:       SpillOptions{Mode: SpillAlways, Threshold: 0, Format: "json", TTL: output.DefaultSpillTTL}, // no Dir -> managed
	}
	resp, spilled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil || !spilled {
		t.Fatalf("spilled=%v err=%v", spilled, err)
	}
	m := resp.Result.(*output.ResultFileManifest)
	if filepath.Base(filepath.Dir(m.Path)) != "prod" {
		t.Errorf("managed cache should be partitioned by context, got %q", m.Path)
	}
	if !strings.HasPrefix(m.Path, tmp) {
		t.Errorf("managed spill escaped the test cache dir: %q (tmp=%q)", m.Path, tmp)
	}
	// Managed location must NOT carry the user-path privacy warning.
	for _, w := range resp.Context.Warnings {
		if w == userPathPrivacyWarning {
			t.Errorf("managed spill should not warn about user-path privacy opt-out")
		}
	}
}

func TestBuildSpillResponse_Sampled(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(true)
	opts := DQLExecuteOptions{
		Spill: SpillOptions{Mode: SpillAlways, Threshold: 0, Dir: t.TempDir(), Format: "json", TTL: output.DefaultSpillTTL},
	}
	resp, spilled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil || !spilled {
		t.Fatalf("spilled=%v err=%v", spilled, err)
	}
	m := resp.Result.(*output.ResultFileManifest)
	if !m.Sampled {
		t.Error("manifest.Sampled should be true")
	}
	if m.SampleStats == nil || m.Columns != nil {
		t.Fatal("sampled result must use sample_stats, not columns (D23)")
	}
	if m.SampleStats.Basis != "sample" {
		t.Errorf("sample_stats basis = %q, want sample", m.SampleStats.Basis)
	}
	for _, c := range m.SampleStats.Columns {
		if c.Basis != "sample" {
			t.Errorf("column %q basis = %q, want sample", c.Name, c.Basis)
		}
	}
}

func TestBuildSpillResponse_SummaryOnly(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(false)

	// Make the preferred dir unwritable: point it under a regular file so the
	// results subdir can never be created.
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := DQLExecuteOptions{
		Spill: SpillOptions{Mode: SpillAlways, Threshold: 0, Dir: filepath.Join(f, "nope"), Format: "json"},
	}
	resp, spilled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil || !spilled {
		t.Fatalf("spilled=%v err=%v", spilled, err)
	}
	m := resp.Result.(*output.ResultFileManifest)
	if m.Kind != output.KindSummaryOnly {
		t.Errorf("kind = %q, want summary-only", m.Kind)
	}
	if m.Path != "" {
		t.Errorf("summary-only must omit path, got %q", m.Path)
	}
	if m.Bytes != 0 {
		t.Errorf("summary-only must omit bytes, got %d", m.Bytes)
	}
	if m.Columns == nil {
		t.Error("summary-only should still carry computed stats")
	}
	if resp.Context.Decided != "summary-only" {
		t.Errorf("decided = %q, want summary-only", resp.Context.Decided)
	}
	if len(resp.Context.Warnings) == 0 {
		t.Error("expected a no-writable-location warning")
	}

	// A read-only filesystem makes --spill-to futile, so the advice must steer to
	// a self-bounding re-query (--spill=never plus a record/column cap), not to
	// writing a file we just proved we cannot write.
	joined := strings.Join(resp.Context.Suggestions, "\n")
	if !strings.Contains(joined, "--spill=never") || !strings.Contains(joined, "--max-result-records") {
		t.Errorf("expected --spill=never + --max-result-records advice for no-writable-location, got:\n%s", joined)
	}
	if strings.Contains(joined, "re-run with --spill-to <path> pointing at a writable location") {
		t.Errorf("must not recommend --spill-to as the remedy on a read-only filesystem, got:\n%s", joined)
	}
}

func TestSpillSuggestions(t *testing.T) {
	q := "fetch logs" // non-aggregating: the DQL-aggregate nudge applies

	t.Run("result-file points at the on-disk file", func(t *testing.T) {
		s := strings.Join(spillSuggestions(q, output.KindResultFile, ""), "\n")
		if !strings.Contains(s, "on disk at the path above") {
			t.Errorf("missing file-read hint:\n%s", s)
		}
		if strings.Contains(s, "--spill=never") || strings.Contains(s, "--spill-to") {
			t.Errorf("result-file must not suggest re-querying:\n%s", s)
		}
	})

	t.Run("no writable location steers to a bounded re-query", func(t *testing.T) {
		s := strings.Join(spillSuggestions(q, output.KindSummaryOnly, summaryReasonNoLocation), "\n")
		for _, want := range []string{"read-only filesystem", "--spill=never", "--max-result-records"} {
			if !strings.Contains(s, want) {
				t.Errorf("missing %q in no-location advice:\n%s", want, s)
			}
		}
		if strings.Contains(s, "pointing at a writable location") {
			t.Errorf("must not recommend --spill-to on a read-only filesystem:\n%s", s)
		}
	})

	t.Run("write failure can retry a different explicit path", func(t *testing.T) {
		s := strings.Join(spillSuggestions(q, output.KindSummaryOnly, summaryReasonWriteFailed), "\n")
		if !strings.Contains(s, "--spill-to") {
			t.Errorf("write-failure should offer --spill-to as a remedy:\n%s", s)
		}
		if strings.Contains(s, "--spill=never") {
			t.Errorf("write-failure should not steer to --spill=never:\n%s", s)
		}
	})
}

func TestBuildSpillResponse_SpillToExplicitPath(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(false)
	dest := filepath.Join(t.TempDir(), "out.csv")
	opts := DQLExecuteOptions{
		Spill: SpillOptions{Mode: SpillAlways, ToPath: dest, Threshold: 0},
	}
	resp, spilled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil || !spilled {
		t.Fatalf("spilled=%v err=%v", spilled, err)
	}
	m := resp.Result.(*output.ResultFileManifest)
	if m.Path != dest {
		t.Errorf("path = %q, want %q", m.Path, dest)
	}
	if m.Format != "csv" {
		t.Errorf("format = %q, want csv (from extension)", m.Format)
	}
	if _, serr := os.Stat(dest); serr != nil {
		t.Errorf("explicit spill file not written: %v", serr)
	}
	// user-chosen path must surface the privacy opt-out warning (D25)
	found := false
	for _, w := range resp.Context.Warnings {
		if w == userPathPrivacyWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("expected user-path privacy warning, got %v", resp.Context.Warnings)
	}
}

func TestSpillEnvelope_JSONShape(t *testing.T) {
	e := &DQLExecutor{}

	// result-file: envelope_version present, kind=result-file, has path.
	result, records := sampleResult(false)
	opts := DQLExecuteOptions{Spill: SpillOptions{Mode: SpillAlways, Threshold: 0, Dir: t.TempDir(), Format: "json"}}
	resp, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	js, _ := json.Marshal(resp)
	s := string(js)
	for _, want := range []string{`"envelope_version":1`, `"kind":"result-file"`, `"path":`, `"decided":"spilled"`, `"columns":`} {
		if !strings.Contains(s, want) {
			t.Errorf("result-file envelope missing %s\n%s", want, s)
		}
	}
	if strings.Contains(s, `"sample_stats"`) {
		t.Errorf("non-sampled envelope must not contain sample_stats")
	}

	// summary-only: no path key, kind=summary-only.
	f := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(f, []byte("x"), 0o600)
	opts = DQLExecuteOptions{Spill: SpillOptions{Mode: SpillAlways, Threshold: 0, Dir: filepath.Join(f, "x"), Format: "json"}}
	resp, _, err = e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil {
		t.Fatal(err)
	}
	js, _ = json.Marshal(resp)
	s = string(js)
	if !strings.Contains(s, `"kind":"summary-only"`) {
		t.Errorf("summary-only kind missing\n%s", s)
	}
	if strings.Contains(s, `"path":`) {
		t.Errorf("summary-only must omit path\n%s", s)
	}
}

func TestBuildSpillResponse_EmptyResultStillWritesOnAlways(t *testing.T) {
	e := &DQLExecutor{}
	result := &DQLQueryResponse{Records: []map[string]interface{}{}}
	dest := filepath.Join(t.TempDir(), "out.json")
	opts := DQLExecuteOptions{Spill: SpillOptions{Mode: SpillAlways, ToPath: dest, Threshold: 0}}

	resp, spilled, err := e.buildSpillResponse("fetch logs | limit 0", result, result.GetRecords(), "json", opts)
	if err != nil || !spilled {
		t.Fatalf("--spill-to must write even an empty result: spilled=%v err=%v", spilled, err)
	}
	m := resp.Result.(*output.ResultFileManifest)
	if m.Rows != 0 {
		t.Errorf("rows = %d, want 0", m.Rows)
	}
	if _, serr := os.Stat(dest); serr != nil {
		t.Errorf("explicit destination should exist for an empty result: %v", serr)
	}
}

func TestBuildSpillResponse_JQWarning(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(false)
	opts := DQLExecuteOptions{
		JQFilter: ".[] | {host}",
		Spill:    SpillOptions{Mode: SpillAlways, Threshold: 0, Dir: t.TempDir(), Format: "json"},
	}
	resp, spilled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil || !spilled {
		t.Fatalf("spilled=%v err=%v", spilled, err)
	}
	found := false
	for _, w := range resp.Context.Warnings {
		if strings.Contains(w, "--jq was not applied") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a --jq-not-applied warning, got %v", resp.Context.Warnings)
	}
}

func TestBuildSpillResponse_UnsupportedFormatErrors(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(false)
	opts := DQLExecuteOptions{
		Spill: SpillOptions{Mode: SpillAlways, Threshold: 0, Dir: t.TempDir(), Format: "avro"},
	}
	_, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err == nil {
		t.Fatal("expected error for an unsupported spill format")
	}
}

func TestBuildSpillResponse_DefaultsToJSONL(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(false)
	opts := DQLExecuteOptions{
		// No Format set -> the default spill format (jsonl) is used.
		Spill: SpillOptions{Mode: SpillAlways, Threshold: 0, Dir: t.TempDir()},
	}
	resp, spilled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil || !spilled {
		t.Fatalf("spilled=%v err=%v", spilled, err)
	}
	m := resp.Result.(*output.ResultFileManifest)
	if m.Format != "jsonl" {
		t.Errorf("default format = %q, want jsonl", m.Format)
	}
	if filepath.Ext(m.Path) != ".jsonl" {
		t.Errorf("default spill path %q should end in .jsonl", m.Path)
	}
	// The file is valid JSON Lines: one JSON object per non-empty line.
	data, rerr := os.ReadFile(m.Path)
	if rerr != nil {
		t.Fatalf("read spill: %v", rerr)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("jsonl line count = %d, want 3\n%s", len(lines), data)
	}
	for _, ln := range lines {
		var row map[string]interface{}
		if jerr := json.Unmarshal([]byte(ln), &row); jerr != nil {
			t.Errorf("line is not valid JSON: %q: %v", ln, jerr)
		}
	}
}

func TestBuildSpillResponse_ParquetUsesDQLTypes(t *testing.T) {
	e := &DQLExecutor{}
	// "count" arrives as a JSON string but is typed `long` in DQL. With the DQL
	// types threaded into the Parquet writer, the schema must make it an INT64
	// column; without types it would be value-inferred as a string (BYTE_ARRAY).
	records := []map[string]interface{}{
		{"count": "194414758", "host": "web-01"},
		{"count": "42", "host": "web-02"},
	}
	result := &DQLQueryResponse{Result: &DQLResult{
		Records: records,
		Types: []sdkquery.ColumnTypes{{
			Mappings: map[string]sdkquery.ColumnType{
				"count": {Type: "long"},
				"host":  {Type: "string"},
			},
		}},
	}}
	dest := filepath.Join(t.TempDir(), "out.parquet")
	opts := DQLExecuteOptions{Spill: SpillOptions{Mode: SpillAlways, ToPath: dest, Threshold: 0}}

	_, spilled, err := e.buildSpillResponse("fetch logs | summarize count()", result, records, "json", opts)
	if err != nil || !spilled {
		t.Fatalf("spilled=%v err=%v", spilled, err)
	}

	data, rerr := os.ReadFile(dest)
	if rerr != nil {
		t.Fatalf("read parquet: %v", rerr)
	}
	f, ferr := parquet.OpenFile(bytes.NewReader(data), int64(len(data)))
	if ferr != nil {
		t.Fatalf("open parquet: %v", ferr)
	}
	col, ok := f.Schema().Lookup("count")
	if !ok {
		t.Fatal("count column missing from parquet schema")
	}
	if got := col.Node.Type().Kind(); got != parquet.Int64 {
		t.Errorf("count physical type = %v, want Int64 — DQL `long` type was not applied to the spill", got)
	}
}

func TestBuildSpillResponse_ParquetExplicitPath(t *testing.T) {
	e := &DQLExecutor{}
	result, records := sampleResult(false)
	dest := filepath.Join(t.TempDir(), "out.parquet")
	opts := DQLExecuteOptions{
		Spill: SpillOptions{Mode: SpillAlways, ToPath: dest, Threshold: 0},
	}
	resp, spilled, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err != nil || !spilled {
		t.Fatalf("parquet spill should succeed now that the writer exists: spilled=%v err=%v", spilled, err)
	}
	m := resp.Result.(*output.ResultFileManifest)
	if m.Format != "parquet" {
		t.Errorf("format = %q, want parquet (from extension)", m.Format)
	}
	if fi, serr := os.Stat(dest); serr != nil || fi.Size() == 0 {
		t.Errorf("parquet spill file missing or empty: err=%v", serr)
	}
}
