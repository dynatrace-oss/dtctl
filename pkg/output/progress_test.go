package output

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewProgressReporter_Gating(t *testing.T) {
	// A bytes.Buffer is never a TTY, so animate must stay false. mode=always on
	// a non-TTY degrades to plain per-line logging; every other combination is
	// fully silent.
	tests := []struct {
		name         string
		mode         string
		agent        bool
		wantAnimate  bool
		wantLogLines bool
	}{
		{"auto non-tty", ProgressAuto, false, false, false},
		{"always non-tty", ProgressAlways, false, false, true},
		{"never non-tty", ProgressNever, false, false, false},
		{"always agent mode", ProgressAlways, true, false, false},
		{"auto agent mode", ProgressAuto, true, false, false},
		{"never agent mode", ProgressNever, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newProgressReporter(tt.mode, tt.agent, &bytes.Buffer{})
			if r.animate != tt.wantAnimate {
				t.Errorf("animate = %v, want %v", r.animate, tt.wantAnimate)
			}
			if r.logLines != tt.wantLogLines {
				t.Errorf("logLines = %v, want %v", r.logLines, tt.wantLogLines)
			}
		})
	}
}

func TestProgressReporter_DisabledIsSilent(t *testing.T) {
	var buf bytes.Buffer
	r := newProgressReporter(ProgressAuto, false, &buf) // non-TTY auto => disabled
	r.Update(ProgressState{Progress: 50, PreviewRows: 100})
	r.Stop()
	if buf.Len() != 0 {
		t.Errorf("disabled reporter wrote %q, want nothing", buf.String())
	}
}

func TestProgressReporter_LogLines(t *testing.T) {
	var buf bytes.Buffer
	r := newProgressReporter(ProgressAlways, false, &buf) // non-TTY always => logLines
	r.Update(ProgressState{Progress: 20})
	r.Update(ProgressState{Progress: 80, PreviewRows: 1234})
	r.Update(ProgressState{Progress: 95, ScannedBytes: 13_359_294_822_752, ScannedRecords: 4_647_571_690})
	r.Stop() // no-op for logLines

	got := buf.String()
	want := "querying... 20%\n" +
		"querying... 80% (preview: 1,234 rows)\n" +
		"querying... 95% (12.2 TB scanned, 4.6B records)\n"
	if got != want {
		t.Errorf("logLines output = %q, want %q", got, want)
	}
	// Plain log path must never emit ANSI escape sequences or carriage returns.
	if strings.ContainsAny(got, "\r\x1b") {
		t.Errorf("logLines output contains control chars: %q", got)
	}
}

func TestProgressReporter_AnimateOverwritesAndClears(t *testing.T) {
	var buf bytes.Buffer
	// Force the animated path directly — a buffer is not a TTY. manualTick keeps
	// the background animator off so frames are driven synchronously by Update.
	r := &ProgressReporter{w: &buf, animate: true, manualTick: true}

	r.Update(ProgressState{Progress: 45, ScannedBytes: 13_359_294_822_752, ScannedRecords: 4_647_571_690})
	r.Update(ProgressState{Progress: 90}) // shorter line must be fully padded over
	r.Stop()

	got := buf.String()
	// Each Update begins by returning the cursor to column 0.
	if !strings.HasPrefix(got, "\r") {
		t.Errorf("animated output should start with a carriage return: %q", got)
	}
	if !strings.Contains(got, "45%") || !strings.Contains(got, "90%") {
		t.Errorf("output missing progress values: %q", got)
	}
	if !strings.Contains(got, "12.2 TB") || !strings.Contains(got, "4.6B recs") {
		t.Errorf("output missing scan stats: %q", got)
	}
	if !strings.Contains(got, "scanning") {
		t.Errorf("output should use the 'scanning' verb when bytes are present: %q", got)
	}
	// Stop clears the line and leaves the cursor at column 0.
	if !strings.HasSuffix(got, "\r") {
		t.Errorf("output should end cleared (trailing CR): %q", got)
	}
}

func TestProgressReporter_AnimatePadsByVisibleWidth(t *testing.T) {
	// With color enabled the line carries ANSI escapes; the clear on Stop must
	// be sized by visible width, not byte length, or it would over-pad and wrap.
	ResetColorCache()
	t.Setenv("FORCE_COLOR", "1")
	t.Cleanup(ResetColorCache)

	var buf bytes.Buffer
	r := &ProgressReporter{w: &buf, animate: true, manualTick: true}
	r.Update(ProgressState{Progress: 50, ScannedBytes: 1 << 40, ScannedRecords: 1_000_000})
	if !ColorEnabled() {
		t.Skip("color not enabled in this environment; visible-width path not exercised")
	}
	// The colored line contains ANSI escapes, so its byte length exceeds its
	// on-screen width; lastVis must track the (smaller) visible width so Stop
	// clears exactly the visible columns without wrapping.
	if r.lastVis == 0 || r.lastVis >= len(buf.String()) {
		t.Errorf("visible width %d should be >0 and less than byte length %d when colored", r.lastVis, len(buf.String()))
	}
}

// syncBuf is a mutex-guarded buffer so the test can read output while the
// background animator writes concurrently, without racing.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestProgressReporter_SpinnerAdvancesBetweenUpdates(t *testing.T) {
	// The animator must keep the spinner moving on its own timer, without any
	// further Update calls, and stop cleanly.
	ResetColorCache()
	t.Setenv("NO_COLOR", "1") // keep output ANSI-free so we count raw spinner runes
	t.Cleanup(ResetColorCache)

	buf := &syncBuf{}
	r := &ProgressReporter{w: buf, animate: true, start: time.Now(), tickInterval: 10 * time.Millisecond}

	r.Update(ProgressState{Progress: 30}) // starts the animator; no further updates
	time.Sleep(80 * time.Millisecond)     // ~8 ticks
	r.Stop()

	// Count how many distinct spinner frames appeared — proves it advanced
	// without additional Update calls.
	out := buf.String()
	distinct := 0
	for _, g := range progressSpinner {
		if strings.ContainsRune(out, g) {
			distinct++
		}
	}
	if distinct < 3 {
		t.Errorf("spinner advanced through only %d distinct frames, want >=3 (output=%q)", distinct, out)
	}
	// After Stop the line is cleared and the animator has exited; a subsequent
	// Stop must be a safe no-op.
	if !strings.HasSuffix(out, "\r") {
		t.Errorf("output should end cleared with a trailing CR: %q", out)
	}
	r.Stop() // must not panic (idempotent)
}

func TestProgressReporter_CompletePrintsSummary(t *testing.T) {
	var buf bytes.Buffer
	// manualTick keeps the animator off; started must be forced so Complete
	// treats this as a bar that was shown.
	r := &ProgressReporter{w: &buf, animate: true, manualTick: true, start: time.Now().Add(-42100 * time.Millisecond)}
	r.Update(ProgressState{Progress: 90, ScannedBytes: 1 << 40, ScannedRecords: 5_983_657_731})

	r.Complete(ProgressState{Progress: 100, ScannedBytes: 19_051_610_460_057, ScannedRecords: 5_983_657_731})

	got := buf.String()
	if !strings.Contains(got, "✓") {
		t.Errorf("summary should include a check mark: %q", got)
	}
	if !strings.Contains(got, "scanned 17.3 TB · 6.0B records in 42.1s") {
		t.Errorf("summary body wrong: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("summary should end with a newline: %q", got)
	}
}

func TestProgressReporter_CompleteSilentWhenNoBar(t *testing.T) {
	// A query too fast to have animated (never Update'd) must not print a
	// summary — Complete just clears (a no-op here) and stays quiet.
	var buf bytes.Buffer
	r := &ProgressReporter{w: &buf, animate: true, manualTick: true, start: time.Now()}
	r.Complete(ProgressState{Progress: 100, ScannedBytes: 1 << 40})
	if buf.Len() != 0 {
		t.Errorf("Complete without a prior bar should be silent, wrote %q", buf.String())
	}
}

func TestProgressReporter_StopAfterCompleteIsNoop(t *testing.T) {
	var buf bytes.Buffer
	r := &ProgressReporter{w: &buf, animate: true, manualTick: true, start: time.Now()}
	r.Update(ProgressState{Progress: 50})
	r.Complete(ProgressState{Progress: 100})
	n := buf.Len()
	r.Stop() // must not clear or double-print after Complete already ran
	if buf.Len() != n {
		t.Errorf("Stop after Complete wrote extra bytes: %q", buf.String()[n:])
	}
}

func TestRenderBar(t *testing.T) {
	tests := []struct {
		progress   int
		wantFilled int
	}{
		{0, 0},
		{50, 10},
		{100, 20},
		{150, 20}, // clamped
	}
	for _, tt := range tests {
		bar := renderBar(tt.progress)
		if got := strings.Count(bar, "█"); got != tt.wantFilled {
			t.Errorf("renderBar(%d) filled = %d, want %d (bar=%q)", tt.progress, got, tt.wantFilled, bar)
		}
		if total := strings.Count(bar, "█") + strings.Count(bar, "░"); total != progressBarWidth {
			t.Errorf("renderBar(%d) total cells = %d, want %d", tt.progress, total, progressBarWidth)
		}
	}
}

func TestHumanizeMetric(t *testing.T) {
	tests := map[int64]string{
		0:          "0",
		999:        "999",
		1000:       "1.0K",
		4200:       "4.2K",
		2085739:    "2.1M",
		5983657731: "6.0B",
	}
	for in, want := range tests {
		if got := humanizeMetric(in); got != want {
			t.Errorf("humanizeMetric(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatElapsed(t *testing.T) {
	tests := map[time.Duration]string{
		500 * time.Millisecond:  "0.5s",
		8200 * time.Millisecond: "8.2s",
		59 * time.Second:        "59.0s",
		63 * time.Second:        "1m03s",
		125 * time.Second:       "2m05s",
	}
	for in, want := range tests {
		if got := formatElapsed(in); got != want {
			t.Errorf("formatElapsed(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanizeCount(t *testing.T) {
	tests := map[int]string{
		0:       "0",
		7:       "7",
		42:      "42",
		999:     "999",
		1000:    "1,000",
		1234:    "1,234",
		12345:   "12,345",
		1234567: "1,234,567",
	}
	for in, want := range tests {
		if got := humanizeCount(in); got != want {
			t.Errorf("humanizeCount(%d) = %q, want %q", in, got, want)
		}
	}
}
