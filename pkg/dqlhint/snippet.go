package dqlhint

import (
	"strings"
	"unicode/utf8"
)

// snippetContext is how many characters of a long line the snippet keeps on
// either side of the error position.
const snippetContext = 60

// Snippet renders the query line holding the start of span with a caret line
// underneath marking the span. A span that continues on later lines is marked
// to the end of its first line. It returns "" when span does not point into
// query.
func Snippet(query string, span Span) string {
	lines := strings.Split(query, "\n")
	if span.StartLine < 1 || span.StartLine > len(lines) {
		return ""
	}
	line := []rune(strings.TrimSuffix(lines[span.StartLine-1], "\r"))
	start := span.StartColumn - 1
	if start < 0 || start >= len(line) {
		return ""
	}
	end := start
	switch {
	case span.EndLine > span.StartLine:
		end = len(line) - 1
	case span.EndLine == span.StartLine && span.EndColumn-1 > start:
		end = min(span.EndColumn-1, len(line)-1)
	}

	from, to := 0, len(line)
	prefix, suffix := "", ""
	if len(line) > 2*snippetContext {
		from = max(0, start-snippetContext)
		to = min(len(line), start+snippetContext)
		end = min(end, to-1)
		if from > 0 {
			prefix = "…"
		}
		if to < len(line) {
			suffix = "…"
		}
	}

	var caret strings.Builder
	caret.WriteString(strings.Repeat(" ", utf8.RuneCountInString(prefix)))
	for _, r := range line[from:start] {
		// Keep tabs so the caret lines up however the reader renders them.
		if r == '\t' {
			caret.WriteRune('\t')
		} else {
			caret.WriteByte(' ')
		}
	}
	caret.WriteString(strings.Repeat("^", end-start+1))
	return prefix + string(line[from:to]) + suffix + "\n" + caret.String()
}
