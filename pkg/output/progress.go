package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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

// partialBlocks maps an eighth (0-7) to the left-aligned partial block glyph
// used to render sub-cell bar progress. Index 0 is empty (handled separately).
var partialBlocks = []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉'}

// ProgressState is a snapshot of a running query's progress passed to
// ProgressReporter.Update. All fields are optional; zero values are omitted
// from the rendered line.
type ProgressState struct {
	// Progress is the completion percentage (0-100).
	Progress int
	// ScannedBytes / ScannedRecords are the running scan totals reported by the
	// backend. When non-zero they are shown in preference to the preview count,
	// since they reflect the actual work (and cost) done so far.
	ScannedBytes   int64
	ScannedRecords int64
	// PreviewRows is the number of rows in the latest preview snapshot, shown
	// only when no scan totals are available (e.g. non-Grail-scan queries).
	PreviewRows int
}

// ProgressReporter renders an in-place progress line to stderr while a query
// runs. When it is not enabled (non-TTY stderr, --plain, agent mode, or
// mode=never) every method is a no-op, so callers can wire it unconditionally
// without disturbing piped or structured output. Color is emitted only when
// ColorEnabled() is true, so NO_COLOR still yields a plain (monochrome) bar.
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
	start    time.Time
	spinIdx  int
	lastVis  int // visible width of the last animated line, for padding/clearing
}

// NewProgressReporter returns a reporter for the given mode. It draws an
// animated line when stderr is a TTY and output is not --plain; agent mode is
// always silent. mode=always forces output even on a non-TTY, degrading to
// plain per-line logging.
func NewProgressReporter(mode string, agentMode bool) *ProgressReporter {
	return newProgressReporter(mode, agentMode, os.Stderr)
}

// newProgressReporter is the testable core of NewProgressReporter with an
// injectable writer.
func newProgressReporter(mode string, agentMode bool, w io.Writer) *ProgressReporter {
	r := &ProgressReporter{w: w, start: time.Now()}
	if agentMode || mode == ProgressNever {
		return r
	}

	// Progress is a stderr affordance, so gate on stderr being a TTY (not
	// stdout) and on --plain — but not on NO_COLOR, which only suppresses color,
	// not the bar itself.
	tty := isTerminalWriter(w) && !plainModeEnabled
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

// Update redraws the progress line for the given state. It is a no-op when the
// reporter is disabled.
func (p *ProgressReporter) Update(s ProgressState) {
	if p == nil {
		return
	}
	progress := s.Progress
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	if p.logLines {
		fmt.Fprintf(p.w, "querying... %d%%%s\n", progress, plainStats(s))
		return
	}
	if !p.animate {
		return
	}

	verb := "querying"
	if s.ScannedBytes > 0 {
		verb = "scanning"
	}
	spin := colorize(Cyan, string(progressSpinner[p.spinIdx%len(progressSpinner)]))
	pct := colorize(Bold, fmt.Sprintf("%3d%%", progress))
	line := fmt.Sprintf("%s %s %s %s%s", spin, verb, pct, renderBar(progress), colorize(Dim, statsSuffix(s, p.start)))

	// Return the cursor to column 0, write the new line, then pad with spaces to
	// erase any tail left by a previously longer line. Padding is measured in
	// visible columns (ANSI escapes excluded) so a colored line never overflows
	// the terminal width and wraps.
	vis := visibleWidth(line)
	pad := ""
	if n := p.lastVis - vis; n > 0 {
		pad = strings.Repeat(" ", n)
	}
	fmt.Fprintf(p.w, "\r%s%s", line, pad)
	p.lastVis = vis
	p.spinIdx++
}

// Stop erases the progress line (animated mode) so the real result renders on a
// clean row. It is safe to call multiple times and when disabled.
func (p *ProgressReporter) Stop() {
	if p == nil || !p.animate || p.lastVis == 0 {
		return
	}
	fmt.Fprintf(p.w, "\r%s\r", strings.Repeat(" ", p.lastVis))
	p.lastVis = 0
}

// renderBar draws a fixed-width bar for a 0-100 percentage using block glyphs
// with sub-cell resolution. The filled portion is colored; the remainder dim.
func renderBar(progress int) string {
	exact := float64(progress) / 100 * float64(progressBarWidth)
	full := int(exact)
	if full > progressBarWidth {
		full = progressBarWidth
	}
	partial := ""
	if full < progressBarWidth {
		if e := int((exact - float64(full)) * 8); e > 0 {
			partial = string(partialBlocks[e])
		}
	}
	empty := progressBarWidth - full
	if partial != "" {
		empty--
	}
	filled := colorize(BrightCyan, strings.Repeat("█", full)+partial)
	rest := colorize(Dim, strings.Repeat("░", empty))
	return "▕" + filled + rest + "▏"
}

// statsSuffix renders the trailing stats segment for the animated line: scan
// totals (preferred) or preview rows, plus elapsed time.
func statsSuffix(s ProgressState, start time.Time) string {
	var parts []string
	if s.ScannedBytes > 0 {
		parts = append(parts, formatBytes(s.ScannedBytes))
		if s.ScannedRecords > 0 {
			parts = append(parts, humanizeMetric(s.ScannedRecords)+" recs")
		}
	} else if s.PreviewRows > 0 {
		parts = append(parts, "preview: "+humanizeCount(s.PreviewRows)+" rows")
	}
	parts = append(parts, formatElapsed(time.Since(start)))
	return "  " + strings.Join(parts, " · ")
}

// plainStats renders the ANSI-free stats segment for the CI log-lines path.
func plainStats(s ProgressState) string {
	switch {
	case s.ScannedBytes > 0:
		if s.ScannedRecords > 0 {
			return fmt.Sprintf(" (%s scanned, %s records)", formatBytes(s.ScannedBytes), humanizeMetric(s.ScannedRecords))
		}
		return fmt.Sprintf(" (%s scanned)", formatBytes(s.ScannedBytes))
	case s.PreviewRows > 0:
		return fmt.Sprintf(" (preview: %s rows)", humanizeCount(s.PreviewRows))
	}
	return ""
}

// colorize wraps text in a color code only when color is enabled.
func colorize(code, text string) string {
	if !ColorEnabled() {
		return text
	}
	return code + text + Reset
}

// formatElapsed renders a duration compactly: "8.2s" under a minute, "1m03s"
// above.
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d / time.Minute)
	sec := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm%02ds", m, sec)
}

// humanizeMetric formats a large count with a decimal SI-style suffix, e.g.
// 5983657731 -> "6.0B", 2085739 -> "2.1M", 4200 -> "4.2K".
func humanizeMetric(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
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

// visibleWidth returns the number of visible columns in s, skipping ANSI SGR
// escape sequences (\x1b[...m). Used so colored lines are padded and cleared by
// their on-screen width rather than their byte length.
func visibleWidth(s string) int {
	w, inEsc := 0, false
	for _, r := range s {
		switch {
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		case r == '\x1b':
			inEsc = true
		default:
			w++
		}
	}
	return w
}
