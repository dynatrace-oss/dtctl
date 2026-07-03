package output

import (
	"bytes"
	"strings"
	"testing"
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
	r.Update(50, 100)
	r.Stop()
	if buf.Len() != 0 {
		t.Errorf("disabled reporter wrote %q, want nothing", buf.String())
	}
}

func TestProgressReporter_LogLines(t *testing.T) {
	var buf bytes.Buffer
	r := newProgressReporter(ProgressAlways, false, &buf) // non-TTY always => logLines
	r.Update(20, 0)
	r.Update(80, 1234)
	r.Stop() // no-op for logLines

	got := buf.String()
	wantLines := []string{
		"querying... 20%\n",
		"querying... 80% (preview: 1,234 rows)\n",
	}
	want := strings.Join(wantLines, "")
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

	r.Update(45, 1204)
	r.Update(90, 0) // shorter line (no preview suffix) must be fully padded over
	r.Stop()

	got := buf.String()
	// Each Update begins by returning the cursor to column 0.
	if !strings.HasPrefix(got, "\r") {
		t.Errorf("animated output should start with a carriage return: %q", got)
	}
	if !strings.Contains(got, "45%") || !strings.Contains(got, "90%") {
		t.Errorf("output missing progress values: %q", got)
	}
	if !strings.Contains(got, "(preview: 1,204 rows)") {
		t.Errorf("output missing preview counter: %q", got)
	}
	// Stop clears the line and leaves the cursor at column 0.
	if !strings.HasSuffix(got, "\r") {
		t.Errorf("output should end cleared (trailing CR): %q", got)
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
