package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/output"
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
		Spill: SpillOptions{Mode: SpillAlways, Threshold: 0, Dir: t.TempDir(), Format: "parquet"},
	}
	_, _, err := e.buildSpillResponse("fetch logs", result, records, "json", opts)
	if err == nil {
		t.Fatal("expected error for not-yet-available parquet format")
	}
}
