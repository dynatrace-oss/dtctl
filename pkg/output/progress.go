package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// progressBarWidth is the number of cells in the rendered bar.
const progressBarWidth = 20

// defaultTickInterval is how often the animated line is redrawn so the spinner
// and elapsed timer advance smoothly between (much slower) query polls.
const defaultTickInterval = 100 * time.Millisecond

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
// runs. It is drawn only for an interactive terminal; when disabled (non-TTY
// stderr, --plain, agent mode, or the user opted out) every method is a no-op,
// so callers can wire it unconditionally without disturbing piped or structured
// output. Color is emitted only when ColorEnabled() is true, so NO_COLOR still
// yields a plain (monochrome) bar.
//
// A background goroutine redraws the line on a fixed interval (so the spinner
// and elapsed timer stay smooth between the far slower query polls), while
// Update — called from the poll loop — only refreshes the shared state. The
// mutex guards that state and serializes writes between the two.
type ProgressReporter struct {
	w io.Writer
	// animate is true when the reporter is enabled and stderr is an interactive
	// TTY: the line is redrawn in place with a carriage return and cleared on Stop.
	animate bool
	start   time.Time
	// tickInterval overrides the redraw cadence (tests only); 0 => default.
	tickInterval time.Duration
	// manualTick disables the background animator so tests can drive frames
	// synchronously via Update; production always leaves it false.
	manualTick bool

	mu        sync.Mutex
	latest    ProgressState
	spinIdx   int
	lastVis   int  // visible width of the last animated line, for padding/clearing
	shown     bool // at least one frame was drawn (gates the completion summary)
	animating bool // the background ticker goroutine is running
	stopped   bool
	ticker    *time.Ticker
	done      chan struct{}
	wg        sync.WaitGroup
}

// NewProgressReporter returns a reporter. When enabled is false (the user opted
// out), or in agent mode, or under --plain, or when stderr is not an
// interactive terminal, the reporter is a silent no-op. NO_COLOR is respected
// for color only — the bar is still drawn, just without color.
func NewProgressReporter(enabled, agentMode bool) *ProgressReporter {
	return newProgressReporter(enabled, agentMode, os.Stderr)
}

// newProgressReporter is the testable core of NewProgressReporter with an
// injectable writer.
func newProgressReporter(enabled, agentMode bool, w io.Writer) *ProgressReporter {
	r := &ProgressReporter{w: w, start: time.Now()}
	// Progress is a stderr affordance: gate on stderr being a TTY (not stdout)
	// and on --plain/agent mode — but not on NO_COLOR, which only drops color.
	r.animate = enabled && !agentMode && !plainModeEnabled && isTerminalWriter(w)
	return r
}

// Update refreshes the progress state. In animated mode it also starts the
// background redraw loop on first call and repaints immediately so new poll
// data (progress, scan totals) appears without waiting for the next tick. It is
// a no-op when the reporter is disabled.
func (p *ProgressReporter) Update(s ProgressState) {
	if p == nil {
		return
	}
	if s.Progress < 0 {
		s.Progress = 0
	}
	if s.Progress > 100 {
		s.Progress = 100
	}

	if !p.animate {
		return
	}

	p.mu.Lock()
	// A late Update after Stop/Complete must not repaint the cleared line.
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.latest = s
	if !p.animating && !p.manualTick {
		p.animating = true
		p.startTickerLocked()
	}
	p.renderLocked()
	p.mu.Unlock()
}

// startTickerLocked launches the background redraw goroutine. Caller holds mu.
func (p *ProgressReporter) startTickerLocked() {
	iv := p.tickInterval
	if iv <= 0 {
		iv = defaultTickInterval
	}
	p.ticker = time.NewTicker(iv)
	p.done = make(chan struct{})
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-p.done:
				return
			case <-p.ticker.C:
				p.mu.Lock()
				p.spinIdx++
				p.renderLocked()
				p.mu.Unlock()
			}
		}
	}()
}

// renderLocked paints one frame from the current state. Caller holds mu.
func (p *ProgressReporter) renderLocked() {
	s := p.latest
	verb := "querying"
	if s.ScannedBytes > 0 {
		verb = "scanning"
	}
	spin := colorize(Cyan, string(progressSpinner[p.spinIdx%len(progressSpinner)]))
	pct := colorize(Bold, fmt.Sprintf("%3d%%", s.Progress))
	line := fmt.Sprintf("%s %s %s %s%s", spin, verb, pct, renderBar(s.Progress), colorize(Dim, statsSuffix(s, p.start)))

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
	p.shown = true
}

// Stop halts the background animator and erases the progress line so the real
// result renders on a clean row. Use it on error or cancellation; use Complete
// on success to leave a summary. It is safe to call multiple times and when
// disabled.
func (p *ProgressReporter) Stop() {
	if p == nil || !p.animate {
		return
	}
	if _, first := p.stopAnimator(); !first {
		return
	}
	p.clearLine()
}

// Complete halts the animator and, when a bar was actually shown, replaces it
// with a one-line completion summary (a green check, the final scan totals, and
// elapsed time). For a query too fast to have animated it just clears, adding
// no noise. It is idempotent with Stop; whichever runs first wins.
func (p *ProgressReporter) Complete(final ProgressState) {
	if p == nil || !p.animate {
		return
	}
	shown, first := p.stopAnimator()
	if !first {
		return
	}
	p.clearLine()
	if !shown {
		return
	}
	check := colorize(Bold+Green, "✓")
	fmt.Fprintf(p.w, "%s %s\n", check, colorize(Dim, summaryText(final, p.start)))
}

// stopAnimator halts the background loop exactly once, waiting for the goroutine
// to exit. It reports whether an animation had started and whether this was the
// first stop (so callers don't double-clear or double-print). The wait happens
// outside the lock to avoid deadlocking the goroutine.
func (p *ProgressReporter) stopAnimator() (shown, firstStop bool) {
	p.mu.Lock()
	if p.stopped {
		s := p.shown
		p.mu.Unlock()
		return s, false
	}
	p.stopped = true
	shown = p.shown
	animating := p.animating
	done, ticker := p.done, p.ticker
	p.mu.Unlock()

	if animating {
		ticker.Stop()
		close(done)
		p.wg.Wait()
	}
	return shown, true
}

// clearLine erases the current animated line, if any.
func (p *ProgressReporter) clearLine() {
	p.mu.Lock()
	if p.lastVis > 0 {
		fmt.Fprintf(p.w, "\r%s\r", strings.Repeat(" ", p.lastVis))
		p.lastVis = 0
	}
	p.mu.Unlock()
}

// renderBar draws a fixed-width bar for a 0-100 percentage using block glyphs
// with sub-cell resolution. The filled portion is colored; the remainder dim.
func renderBar(progress int) string {
	exact := float64(progress) / 100 * float64(progressBarWidth)
	full := int(exact)
	if full > progressBarWidth {
		full = progressBarWidth
	}
	if full < 0 {
		full = 0
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

	var b strings.Builder
	b.WriteString("▕")
	// Render the moving frontier (the partial cell, or the last full cell when
	// the bar lands exactly on a boundary) in bright cyan, with the body a
	// calmer cyan — a subtle leading-edge glow rather than a flat block.
	switch {
	case partial != "":
		if full > 0 {
			b.WriteString(colorize(Cyan, strings.Repeat("█", full)))
		}
		b.WriteString(colorize(BrightCyan, partial))
	case full > 0:
		if full > 1 {
			b.WriteString(colorize(Cyan, strings.Repeat("█", full-1)))
		}
		b.WriteString(colorize(BrightCyan, "█"))
	}
	b.WriteString(colorize(Dim, strings.Repeat("░", empty)))
	b.WriteString("▏")
	return b.String()
}

// summaryText renders the completion line body, e.g.
// "scanned 17.3 TB · 6.0B records in 42.1s", or "done in 3.2s" for a query that
// reported no scan totals.
func summaryText(s ProgressState, start time.Time) string {
	var b strings.Builder
	if s.ScannedBytes > 0 {
		b.WriteString("scanned ")
		b.WriteString(formatBytes(s.ScannedBytes))
		if s.ScannedRecords > 0 {
			b.WriteString(" · ")
			b.WriteString(humanizeMetric(s.ScannedRecords))
			b.WriteString(" records")
		}
		b.WriteString(" in ")
	} else {
		b.WriteString("done in ")
	}
	b.WriteString(formatElapsed(time.Since(start)))
	return b.String()
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
