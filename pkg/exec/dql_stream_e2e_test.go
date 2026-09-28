package exec

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The unit tests cover each link on its own; this drives the whole chain a real
// `dtctl query -A` takes — HTTP response -> streaming decode -> collector ->
// committed spill file -> envelope on stdout.
func TestE2E_AgentSpillStreamsTheResult(t *testing.T) {
	dir := t.TempDir()
	e := mockGrail(t, manyRecords(200))

	out := runAndCapture(t, func() error {
		return e.ExecuteWithOptions("fetch logs", DQLExecuteOptions{
			OutputFormat: "json",
			AgentMode:    true,
			ContextName:  "prod",
			Spill:        SpillOptions{Mode: SpillAuto, Threshold: 200, Dir: dir, Format: "jsonl"},
		})
	})

	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Kind string `json:"kind"`
			Path string `json:"path"`
			Rows int    `json:"rows"`
		} `json:"result"`
		Context struct {
			Decided          string `json:"decided"`
			Streamed         bool   `json:"streamed"`
			MeasuredBytes    *int64 `json:"measured_bytes"`
			MeasuredEncoding string `json:"measured_encoding"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not a single JSON envelope: %v\n%s", err, out)
	}
	if !env.OK || env.Result.Kind != "result-file" || env.Context.Decided != "spilled" {
		t.Fatalf("ok=%v kind=%q decided=%q\n%s", env.OK, env.Result.Kind, env.Context.Decided, out)
	}
	if env.Result.Rows != 200 {
		t.Errorf("rows = %d, want 200", env.Result.Rows)
	}
	if !env.Context.Streamed {
		t.Error("context.streamed is false; the envelope must say the rows were never held")
	}
	// Nothing was serialised for inline emission, so there is no measurement to
	// report — reporting one would be a number nobody produced.
	if env.Context.MeasuredBytes != nil || env.Context.MeasuredEncoding != "" {
		t.Errorf("measured_bytes=%v measured_encoding=%q, want both absent",
			env.Context.MeasuredBytes, env.Context.MeasuredEncoding)
	}
	if strings.Contains(out, rowMarker(199)) {
		t.Errorf("a non-sampled row leaked into stdout:\n%s", out)
	}

	lines := readSpilledLines(t, env.Result.Path)
	if len(lines) != 200 {
		t.Fatalf("spill file holds %d rows, want 200", len(lines))
	}
	for i, line := range lines {
		if !strings.Contains(line, rowMarker(i)) {
			t.Fatalf("spill line %d = %s, want the row-%d marker (order must be preserved)", i, line, i)
		}
	}
}

// Below the switch point the streaming path hands its buffer to the very same
// printer, so an eligible small result must come out byte for byte as it does
// when the plan is rejected outright.
func TestE2E_SmallAgentResultIsByteIdenticalStreamedOrNot(t *testing.T) {
	run := func(format string) string {
		e := mockGrail(t, manyRecords(3))
		return runAndCapture(t, func() error {
			return e.ExecuteWithOptions("fetch logs", DQLExecuteOptions{
				OutputFormat: "json",
				AgentMode:    true,
				ContextName:  "prod",
				Spill:        SpillOptions{Mode: SpillAuto, Threshold: 1 << 20, Dir: t.TempDir(), Format: format},
			})
		})
	}

	// jsonl is streaming-eligible, json is not; neither result reaches the
	// threshold, so the spill format never gets to matter.
	streamed, buffered := run("jsonl"), run("json")
	if streamed != buffered {
		t.Errorf("streaming-eligible run differs from the buffered one:\n streamed: %s\n buffered: %s", streamed, buffered)
	}
	if !strings.Contains(streamed, rowMarker(2)) {
		t.Errorf("small result should be inline, but the last row is missing:\n%s", streamed)
	}
}

// `-o jsonl` writes the rows straight to stdout as they decode.
func TestE2E_JSONLStreamsToStdout(t *testing.T) {
	e := mockGrail(t, manyRecords(50))

	out := runAndCapture(t, func() error {
		return e.ExecuteWithOptions("fetch logs", DQLExecuteOptions{OutputFormat: "jsonl"})
	})

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 50 {
		t.Fatalf("got %d lines, want 50\n%s", len(lines), out)
	}
	for i, line := range lines {
		var row map[string]interface{}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("line %d is not JSON: %v (%s)", i, err, line)
		}
		if !strings.Contains(line, rowMarker(i)) {
			t.Errorf("line %d = %s, want the row-%d marker", i, line, i)
		}
	}
}

func readSpilledLines(t *testing.T, path string) []string {
	t.Helper()
	if !filepath.IsAbs(path) {
		t.Fatalf("envelope path %q is not absolute", path)
	}
	f, err := os.Open(path) //nolint:gosec // path comes from the envelope this test just produced
	if err != nil {
		t.Fatalf("open spill file: %v", err)
	}
	defer func() { _ = f.Close() }()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan spill file: %v", err)
	}
	return lines
}
