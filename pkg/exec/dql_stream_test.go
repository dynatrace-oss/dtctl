package exec

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/output"
)

func TestPlanStream_Eligibility(t *testing.T) {
	spillAuto := SpillOptions{Mode: SpillAuto, Threshold: 50 << 10, Dir: t.TempDir()}

	cases := []struct {
		name string
		opts DQLExecuteOptions
		want bool
		kind streamSinkKind
	}{
		{
			name: "agent mode with auto spill streams to a spill file",
			opts: DQLExecuteOptions{AgentMode: true, Spill: spillAuto},
			want: true, kind: sinkSpill,
		},
		{
			name: "-o jsonl without spilling streams to stdout",
			opts: DQLExecuteOptions{OutputFormat: "jsonl"},
			want: true, kind: sinkJSONL,
		},
		{
			name: "--jq needs the whole document",
			opts: DQLExecuteOptions{AgentMode: true, Spill: spillAuto, JQFilter: ".records"},
			want: false,
		},
		{
			name: "--typed needs the response type block",
			opts: DQLExecuteOptions{AgentMode: true, Spill: spillAuto, Typed: true},
			want: false,
		},
		{
			name: "snapshot decoding reshapes rows per format",
			opts: DQLExecuteOptions{AgentMode: true, Spill: spillAuto, Decode: DecodeSimplified},
			want: false,
		},
		{
			name: "agent mode under --spill=never always emits rows inline",
			opts: DQLExecuteOptions{AgentMode: true, Spill: SpillOptions{Mode: SpillNever}},
			want: false,
		},
		{
			name: "a json spill file needs array framing",
			opts: DQLExecuteOptions{AgentMode: true, Spill: SpillOptions{Mode: SpillAuto, Threshold: 1, Dir: t.TempDir(), Format: "json"}},
			want: false,
		},
		{
			name: "a csv spill file needs a header built from every row",
			opts: DQLExecuteOptions{AgentMode: true, Spill: SpillOptions{Mode: SpillAuto, Threshold: 1, Dir: t.TempDir(), Format: "csv"}},
			want: false,
		},
		{
			name: "-o table is not row-at-a-time",
			opts: DQLExecuteOptions{OutputFormat: "table"},
			want: false,
		},
		{
			name: "-o csv is not row-at-a-time",
			opts: DQLExecuteOptions{OutputFormat: "csv"},
			want: false,
		},
	}

	e := &DQLExecutor{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, ok := e.planStream(tc.opts)
			if ok != tc.want {
				t.Fatalf("planStream() ok = %v, want %v", ok, tc.want)
			}
			if ok && plan.kind != tc.kind {
				t.Errorf("plan.kind = %v, want %v", plan.kind, tc.kind)
			}
		})
	}
}

// The switch point is the largest row count that can still be emitted inline:
// holding one more row could never have paid off, and holding one fewer would
// have streamed a result that still fitted.
func TestStreamSwitchRows_IsTheLargestInlineRowCount(t *testing.T) {
	for _, format := range []string{"json", "toon", "csv"} {
		t.Run(format, func(t *testing.T) {
			opts := DQLExecuteOptions{Spill: SpillOptions{Mode: SpillAuto, Threshold: 50 << 10}}
			switchAt := streamSwitchRows(opts, format)

			emptyRows := func(n int) []map[string]interface{} {
				rows := make([]map[string]interface{}, n)
				for i := range rows {
					rows[i] = map[string]interface{}{}
				}
				return rows
			}

			fits, _ := output.MeasureSerializedBytes(emptyRows(switchAt), format)
			if fits > opts.Spill.Threshold {
				t.Errorf("%d empty rows measure %d bytes, over the %d-byte threshold — the switch is too late",
					switchAt, fits, opts.Spill.Threshold)
			}
			over, _ := output.MeasureSerializedBytes(emptyRows(switchAt+1), format)
			if over <= opts.Spill.Threshold {
				t.Errorf("%d empty rows still measure %d bytes under the %d-byte threshold — the switch is too early",
					switchAt+1, over, opts.Spill.Threshold)
			}
		})
	}
}

func TestStreamSwitchRows_SpillAlwaysStreamsFromTheFirstRow(t *testing.T) {
	opts := DQLExecuteOptions{Spill: SpillOptions{Mode: SpillAlways, Threshold: 50 << 10}}
	if got := streamSwitchRows(opts, "json"); got != 0 {
		t.Errorf("streamSwitchRows() = %d, want 0 for --spill=always", got)
	}
}

// bigRecords builds n distinct rows sharing two constant columns, so the
// compaction, stats and sample accumulators all have something to find.
func bigRecords(n int) []map[string]interface{} {
	out := make([]map[string]interface{}, n)
	for i := range out {
		out[i] = map[string]interface{}{
			"timestamp":       fmt.Sprintf("2026-06-21T00:%02d:%02dZ", i/60%60, i%60),
			"content":         fmt.Sprintf("request %d completed", i),
			"loglevel":        []string{"INFO", "WARN", "ERROR"}[i%3],
			"k8s.namespace":   "production",
			"dt.entity.host":  "HOST-1",
			"response.status": float64(200 + i%3),
			"optional":        nil,
		}
	}
	return out
}

// feed runs rows through a collector exactly as the SDK would.
func feed(t *testing.T, e *DQLExecutor, plan *streamPlan, opts DQLExecuteOptions, rows []map[string]interface{}) *streamCollector {
	t.Helper()
	col := newStreamCollector(e, plan, opts)
	for _, r := range rows {
		if err := col.observe(r); err != nil {
			t.Fatalf("observe: %v", err)
		}
	}
	return col
}

// The streamed envelope must describe the result the same way the buffered one
// does. Only the spill-measurement fields may differ: a streamed result is
// never serialised for inline emission, so there is nothing to measure.
func TestStreamedSpillResponse_MatchesBufferedEnvelope(t *testing.T) {
	const rows = 400
	records := bigRecords(rows)
	result, _ := sampleResult(false)
	result.Records = nil

	bufferedOpts := DQLExecuteOptions{
		AgentMode: true, Compact: true, ContextName: "prod", TenantID: "abc12345",
		Spill: SpillOptions{Mode: SpillAlways, Threshold: 1 << 10, Dir: t.TempDir(), Format: "jsonl"},
	}
	streamedOpts := bufferedOpts
	streamedOpts.Spill.Dir = t.TempDir()

	e := &DQLExecutor{}
	buffered, handled, err := e.buildSpillResponse("fetch logs", result, records, "json", bufferedOpts)
	if err != nil || !handled {
		t.Fatalf("buildSpillResponse: handled=%v err=%v", handled, err)
	}

	plan, ok := e.planStream(streamedOpts)
	if !ok {
		t.Fatal("planStream() declined an --spill=always jsonl agent invocation")
	}
	col := feed(t, e, plan, streamedOpts, records)
	defer col.abort()
	if !col.streaming {
		t.Fatal("collector never left the buffering phase under --spill=always")
	}
	streamed, err := e.buildStreamedSpillResponse("fetch logs", result, col, streamedOpts)
	if err != nil {
		t.Fatalf("buildStreamedSpillResponse: %v", err)
	}

	bm := buffered.Result.(*output.ResultFileManifest)
	sm := streamed.Result.(*output.ResultFileManifest)

	if bm.Rows != sm.Rows {
		t.Errorf("rows: streamed %d, buffered %d", sm.Rows, bm.Rows)
	}
	if !reflect.DeepEqual(bm.Constant, sm.Constant) {
		t.Errorf("constant:\n streamed %#v\n buffered %#v", sm.Constant, bm.Constant)
	}
	if !reflect.DeepEqual(bm.NullColumns, sm.NullColumns) {
		t.Errorf("null_columns: streamed %v, buffered %v", sm.NullColumns, bm.NullColumns)
	}
	if !reflect.DeepEqual(bm.SampleRows, sm.SampleRows) {
		t.Errorf("sample_rows:\n streamed %#v\n buffered %#v", sm.SampleRows, bm.SampleRows)
	}
	if !reflect.DeepEqual(bm.ColumnsOmitted, sm.ColumnsOmitted) {
		t.Errorf("columns_omitted: streamed %v, buffered %v", sm.ColumnsOmitted, bm.ColumnsOmitted)
	}
	if bs, ss := mustJSON(t, bm.Columns), mustJSON(t, sm.Columns); bs != ss {
		t.Errorf("column stats differ:\n streamed %s\n buffered %s", ss, bs)
	}

	if streamed.Context.Decided != buffered.Context.Decided {
		t.Errorf("decided: streamed %q, buffered %q", streamed.Context.Decided, buffered.Context.Decided)
	}
	// The two runs spill into different temp dirs, so compare with the dir
	// itself elided — the file name, which keys off the query, must still match.
	elide := func(lines []string, dir string) []string {
		out := make([]string, len(lines))
		for i, l := range lines {
			out[i] = strings.ReplaceAll(l, dir, "<dir>")
		}
		return out
	}
	gotSug := elide(streamed.Context.Suggestions, streamedOpts.Spill.Dir)
	wantSug := elide(buffered.Context.Suggestions, bufferedOpts.Spill.Dir)
	if !reflect.DeepEqual(gotSug, wantSug) {
		t.Errorf("suggestions:\n streamed %v\n buffered %v", gotSug, wantSug)
	}
	if !reflect.DeepEqual(streamed.Context.Warnings, buffered.Context.Warnings) {
		t.Errorf("warnings:\n streamed %v\n buffered %v", streamed.Context.Warnings, buffered.Context.Warnings)
	}
	if !streamed.Context.Streamed {
		t.Error("context.streamed should mark a result that was never held")
	}
	if streamed.Context.MeasuredBytes != 0 {
		t.Errorf("context.measured_bytes = %d, want 0 — nothing was serialised inline to measure", streamed.Context.MeasuredBytes)
	}

	// Both files must hold the same rows.
	if bufLines, strLines := readJSONL(t, bm.Path), readJSONL(t, sm.Path); !reflect.DeepEqual(bufLines, strLines) {
		t.Errorf("spilled rows differ: streamed %d rows, buffered %d rows", len(strLines), len(bufLines))
	}
}

// A result small enough to still fit inline must never reach the stream: it
// takes the buffered path, which is what keeps today's output byte-identical.
func TestStreamCollector_SmallResultStaysBuffered(t *testing.T) {
	records := bigRecords(5)
	opts := DQLExecuteOptions{
		AgentMode: true, Compact: true,
		Spill: SpillOptions{Mode: SpillAuto, Threshold: 50 << 10, Dir: t.TempDir(), Format: "jsonl"},
	}
	e := &DQLExecutor{}
	plan, ok := e.planStream(opts)
	if !ok {
		t.Fatal("planStream() declined an auto-spill agent invocation")
	}
	col := feed(t, e, plan, opts, records)
	defer col.abort()

	if col.streaming {
		t.Error("a 5-row result should not have switched to the stream")
	}
	if len(col.buf) != len(records) {
		t.Errorf("buffered %d rows, want %d", len(col.buf), len(records))
	}
	if col.writer != nil {
		t.Error("no spill file should have been opened for a buffered result")
	}
}

// Past the switch point the collector must stop holding rows.
func TestStreamCollector_ReleasesRowsOnceStreaming(t *testing.T) {
	opts := DQLExecuteOptions{
		AgentMode: true, Compact: true,
		Spill: SpillOptions{Mode: SpillAuto, Threshold: 512, Dir: t.TempDir(), Format: "jsonl"},
	}
	e := &DQLExecutor{}
	plan, ok := e.planStream(opts)
	if !ok {
		t.Fatal("planStream() declined an auto-spill agent invocation")
	}
	records := bigRecords(plan.switchRows + 50)
	col := feed(t, e, plan, opts, records)
	defer col.abort()

	if !col.streaming {
		t.Fatalf("%d rows past a switch point of %d should have started the stream", len(records), plan.switchRows)
	}
	if col.buf != nil {
		t.Errorf("collector still holds %d rows after switching to the stream", len(col.buf))
	}
	if col.rows != len(records) {
		t.Errorf("counted %d rows, want %d", col.rows, len(records))
	}
	if col.writer.Bytes() == 0 {
		t.Error("nothing was written to the spill file")
	}
}

// --spill=always writes even a one-row result through the stream, and the file
// holds exactly what arrived.
func TestStreamedSpill_WritesEveryRow(t *testing.T) {
	records := bigRecords(250)
	result, _ := sampleResult(false)
	result.Records = nil
	opts := DQLExecuteOptions{
		AgentMode: true, Compact: true,
		Spill: SpillOptions{Mode: SpillAlways, Threshold: 50 << 10, Dir: t.TempDir(), Format: "jsonl"},
	}

	e := &DQLExecutor{}
	plan, _ := e.planStream(opts)
	col := feed(t, e, plan, opts, records)
	defer col.abort()

	resp, err := e.buildStreamedSpillResponse("fetch logs", result, col, opts)
	if err != nil {
		t.Fatalf("buildStreamedSpillResponse: %v", err)
	}
	m := resp.Result.(*output.ResultFileManifest)
	if m.Kind != output.KindResultFile {
		t.Fatalf("kind = %q, want %q", m.Kind, output.KindResultFile)
	}
	if m.Rows != len(records) {
		t.Errorf("manifest rows = %d, want %d", m.Rows, len(records))
	}

	got := readJSONL(t, m.Path)
	if len(got) != len(records) {
		t.Fatalf("spill file holds %d rows, want %d", len(got), len(records))
	}
	for i := range records {
		want := mustJSON(t, records[i])
		if got[i] != want {
			t.Fatalf("row %d:\n got %s\nwant %s", i, got[i], want)
		}
	}

	// The sidecar is written last, so its presence implies a complete file.
	if _, err := os.Stat(output.SidecarPathFor(m.Path)); err != nil {
		t.Errorf("sidecar manifest missing: %v", err)
	}
}

// An abandoned stream must leave no committed file behind, only a .tmp for the
// TTL prune.
func TestStreamCollector_AbortLeavesNoSpillFile(t *testing.T) {
	dir := t.TempDir()
	opts := DQLExecuteOptions{
		AgentMode: true,
		Spill:     SpillOptions{Mode: SpillAlways, Threshold: 50 << 10, Dir: dir, Format: "jsonl"},
	}
	e := &DQLExecutor{}
	plan, _ := e.planStream(opts)
	col := feed(t, e, plan, opts, bigRecords(10))
	col.abort()

	var leftovers []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			leftovers = append(leftovers, path)
		}
		return nil
	})
	for _, p := range leftovers {
		if !strings.HasSuffix(p, ".tmp") {
			t.Errorf("abort left a file behind: %s", p)
		}
	}
}

// Series/precision reshaping is row-independent, so folding it row by row must
// produce exactly what the buffered path produces in one call.
func TestStreamCollector_PerRowTransformMatchesBuffered(t *testing.T) {
	records := []map[string]interface{}{
		{"host": "web-01", "latency": 1.23456789},
		{"host": "web-02", "latency": 9.87654321},
		{"host": "web-03", "latency": 0.000123456789},
	}
	opts := DQLExecuteOptions{
		AgentMode: true, Precision: 4, Series: output.SeriesMode{Kind: output.SeriesFull},
		Spill: SpillOptions{Mode: SpillAuto, Threshold: 50 << 10, Dir: t.TempDir(), Format: "jsonl"},
	}

	want, wantEffect := output.ApplySeriesModeWithEffect(records, opts.Series, opts.Precision, false)

	e := &DQLExecutor{}
	plan, _ := e.planStream(opts)
	col := feed(t, e, plan, opts, records)
	defer col.abort()

	if !reflect.DeepEqual(col.buf, want) {
		t.Errorf("per-row transform:\n got %#v\nwant %#v", col.buf, want)
	}
	if col.effect != wantEffect {
		t.Errorf("series effect = %+v, want %+v", col.effect, wantEffect)
	}
}

func readJSONL(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // test-local temp path
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return out
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
