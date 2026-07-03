package output

import (
	"bytes"
	"strings"
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
	// Force the animated path directly — a buffer is not a TTY.
	r := &ProgressReporter{w: &buf, animate: true}

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
	r := &ProgressReporter{w: &buf, animate: true}
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
