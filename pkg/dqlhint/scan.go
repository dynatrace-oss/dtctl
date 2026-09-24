package dqlhint

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// masked replaces the contents of string literals and backtick identifiers in
// the structural view of a query, so no rule mistakes `"a=b"` for code.
const masked = '#'

// queryContext is a query error prepared for the rules: the query, a masked
// structural view of it with bracket depths, its pipeline segments, and the
// error span as byte offsets.
type queryContext struct {
	q     string
	code  string // q with literal contents masked; byte offsets match q
	depth []int  // bracket nesting depth of each byte; a bracket belongs to its outer level
	segs  []segment
	start int // byte offset of the span start in q
	end   int // byte offset of the span end in q, inclusive
	args  []string
}

// segment is one command of the pipeline: q[start:end], without the pipe.
type segment struct {
	start, end int
	cmd        string
}

var commandRe = regexp.MustCompile(`^\s*([A-Za-z_]\w*)`)

// newContext prepares e for the rules. It returns nil — no rule runs — when
// there is no query or position, the position does not point into the query,
// or the query holds something the scanner does not model (comments,
// unbalanced brackets or quotes).
func newContext(e Error) *queryContext {
	if e.Query == "" || e.Span == nil {
		return nil
	}
	start := byteOffset(e.Query, e.Span.StartLine, e.Span.StartColumn)
	if start < 0 {
		return nil
	}
	end := start
	if e.Span.EndLine > 0 {
		if o := byteOffset(e.Query, e.Span.EndLine, e.Span.EndColumn); o > start {
			end = o
		}
	}
	code, depth, ok := scan(e.Query)
	if !ok {
		return nil
	}
	c := &queryContext{q: e.Query, code: code, depth: depth, start: start, end: end, args: e.Arguments}
	from := 0
	for i := 0; i <= len(code); i++ {
		if i == len(code) || (code[i] == '|' && depth[i] == 0) {
			seg := segment{start: from, end: i}
			if m := commandRe.FindStringSubmatch(code[from:i]); m != nil {
				seg.cmd = m[1]
			}
			c.segs = append(c.segs, seg)
			from = i + 1
		}
	}
	return c
}

// byteOffset converts a 1-based line and character column into a byte offset,
// or -1 when it points past the line.
func byteOffset(q string, line, col int) int {
	if line < 1 || col < 1 {
		return -1
	}
	off := 0
	for l := 1; l < line; l++ {
		nl := strings.IndexByte(q[off:], '\n')
		if nl < 0 {
			return -1
		}
		off += nl + 1
	}
	for c := 1; c < col; c++ {
		if off >= len(q) || q[off] == '\n' {
			return -1
		}
		_, size := utf8.DecodeRuneInString(q[off:])
		off += size
	}
	if off >= len(q) || q[off] == '\n' {
		return -1
	}
	return off
}

// scan builds the masked view and bracket depths of q. ok is false for input
// the rules should not touch.
func scan(q string) (code string, depth []int, ok bool) {
	b := []byte(q)
	depth = make([]int, len(q))
	d := 0
	for i := 0; i < len(b); i++ {
		depth[i] = d
		switch b[i] {
		case '"', '`':
			quote := b[i]
			j := i + 1
			for ; j < len(b) && b[j] != quote; j++ {
				if b[j] == '\\' && quote == '"' {
					b[j] = masked
					depth[j] = d
					j++
					if j >= len(b) {
						break
					}
				}
				b[j] = masked
				depth[j] = d
			}
			if j >= len(b) {
				return "", nil, false
			}
			depth[j] = d
			i = j
		case '(', '[', '{':
			d++
		case ')', ']', '}':
			d--
			if d < 0 {
				return "", nil, false
			}
			depth[i] = d
		case '/':
			if i+1 < len(b) && (b[i+1] == '/' || b[i+1] == '*') {
				return "", nil, false
			}
		}
	}
	if d != 0 {
		return "", nil, false
	}
	return string(b), depth, true
}

// segmentAt returns the pipeline segment holding byte offset off.
func (c *queryContext) segmentAt(off int) *segment {
	for i := range c.segs {
		if off >= c.segs[i].start && off < c.segs[i].end {
			return &c.segs[i]
		}
	}
	return nil
}

func (c *queryContext) arg(i int) string {
	if i < len(c.args) {
		return c.args[i]
	}
	return ""
}

// edit replaces q[start:end] with text.
type edit struct {
	start, end int
	text       string
}

// apply returns q with non-overlapping edits applied.
func apply(q string, edits []edit) string {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out strings.Builder
	last := 0
	for _, e := range edits {
		out.WriteString(q[last:e.start])
		out.WriteString(e.text)
		last = e.end
	}
	out.WriteString(q[last:])
	return out.String()
}

func isIdentByte(b byte) bool {
	return b == '_' || b == '.' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// wordAt reports whether word occurs in s at off as a whole identifier.
func wordAt(s string, off int, word string) bool {
	if off < 0 || !strings.HasPrefix(s[off:], word) {
		return false
	}
	if off > 0 && isIdentByte(s[off-1]) {
		return false
	}
	after := off + len(word)
	return after >= len(s) || !isIdentByte(s[after])
}

// closingParen returns the offset of the bracket closing the one at open.
func (c *queryContext) closingParen(open int) int {
	for i := open + 1; i < len(c.code); i++ {
		if c.code[i] == ')' && c.depth[i] == c.depth[open] {
			return i
		}
	}
	return -1
}

// splitArgs splits the call arguments between the brackets at open and close
// on their top-level commas, returning each argument trimmed.
func (c *queryContext) splitArgs(open, close int) []string {
	var out []string
	from := open + 1
	for i := open + 1; i <= close; i++ {
		if i == close || (c.code[i] == ',' && c.depth[i] == c.depth[open]+1) {
			out = append(out, strings.TrimSpace(c.q[from:i]))
			from = i + 1
		}
	}
	return out
}
