package output

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// ProgressMode controls whether a live progress indicator is drawn while a
// long-running query is polled. It mirrors the auto|always|never convention
// used elsewhere in the CLI.
const (
	ProgressAuto   = "auto"
	ProgressAlways = "always"
	ProgressNever  = "never"
)

// progressBarWidth is the number of cells in the rendered bar.
const progressBarWidth = 20

// progressSpinner holds the braille spinner frames advanced on each update.
var progressSpinner = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// ProgressReporter renders an in-place progress line to stderr while a query
// runs. When it is not enabled (non-TTY, --plain, agent mode, NO_COLOR, or
// mode=never) every method is a no-op, so callers can wire it unconditionally
// without disturbing piped or structured output.
//
// A reporter is used from a single goroutine (the poll loop's OnUpdate callback
// plus a deferred Stop), so it needs no synchronization.
type ProgressReporter struct {
	w io.Writer
	// animate is true when stderr is an interactive TTY: the line is redrawn in
	// place with a carriage return and cleared on Stop.
	animate bool
	// logLines is true for mode=always on a non-TTY: progress is written as
	// plain, ANSI-free lines (one per update) so it survives in CI logs.
	logLines bool
	spinIdx  int
	lastLen  int
}

// NewProgressReporter returns a reporter for the given mode. It draws an
// animated line only when stderr is a TTY and color is enabled (which already
// folds in --plain and NO_COLOR); agent mode is always silent. mode=always
// forces output even on a non-TTY, degrading to plain per-line logging.
func NewProgressReporter(mode string, agentMode bool) *ProgressReporter {
	return newProgressReporter(mode, agentMode, os.Stderr)
}

// newProgressReporter is the testable core of NewProgressReporter with an
// injectable writer. isTerminalWriter reports whether w is an interactive TTY.
func newProgressReporter(mode string, agentMode bool, w io.Writer) *ProgressReporter {
	r := &ProgressReporter{w: w}
	if agentMode || mode == ProgressNever {
		return r
	}

	tty := isTerminalWriter(w) && ColorEnabled()
	switch mode {
	case ProgressAlways:
		if tty {
			r.animate = true
		} else {
			r.logLines = true
		}
	case ProgressAuto:
		r.animate = tty
	}
	return r
}

// Update redraws the progress line with the given completion percentage and, if
// previews are enabled, the number of rows seen so far. It is a no-op when the
// reporter is disabled.
func (p *ProgressReporter) Update(progress, previewRows int) {
	if p == nil {
		return
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	if p.logLines {
		if previewRows > 0 {
			fmt.Fprintf(p.w, "querying... %d%% (preview: %s rows)\n", progress, humanizeCount(previewRows))
		} else {
			fmt.Fprintf(p.w, "querying... %d%%\n", progress)
		}
		return
	}
	if !p.animate {
		return
	}

	line := fmt.Sprintf("%c querying… %3d%% %s", progressSpinner[p.spinIdx%len(progressSpinner)], progress, renderBar(progress))
	if previewRows > 0 {
		line += fmt.Sprintf("  (preview: %s rows)", humanizeCount(previewRows))
	}

	// Return the cursor to column 0, write the new line, and pad with spaces to
	// erase any tail left by a previously longer line. The next Update (or Stop)
	// begins with its own carriage return, so leaving the cursor past the pad is
	// fine. lastLen tracks the visible line width for Stop's clear.
	pad := ""
	if n := p.lastLen - len(line); n > 0 {
		pad = strings.Repeat(" ", n)
	}
	fmt.Fprintf(p.w, "\r%s%s", line, pad)
	p.lastLen = len(line)
	p.spinIdx++
}

// Stop erases the progress line (animated mode) so the real result renders on a
// clean row. It is safe to call multiple times and when disabled.
func (p *ProgressReporter) Stop() {
	if p == nil || !p.animate || p.lastLen == 0 {
		return
	}
	fmt.Fprintf(p.w, "\r%s\r", strings.Repeat(" ", p.lastLen))
	p.lastLen = 0
}

// renderBar draws a fixed-width bar for a 0-100 percentage using block glyphs.
func renderBar(progress int) string {
	filled := progress * progressBarWidth / 100
	if filled > progressBarWidth {
		filled = progressBarWidth
	}
	return "▕" + strings.Repeat("█", filled) + strings.Repeat("░", progressBarWidth-filled) + "▏"
}

// humanizeCount formats a non-negative integer with thousands separators, e.g.
// 1234567 -> "1,234,567".
func humanizeCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}
