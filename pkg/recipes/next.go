package recipes

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"
)

// Carry is what a follow-up invocation inherits from the current one, as the
// caller typed it: the window and the scope narrow the follow-up the same way.
type Carry struct {
	From, To string
	Scope    map[string][]string
	Segments []string
}

// NextCommands renders a recipe's `next` edges as ready-to-run command lines.
// with-bindings come from this invocation's params. bind-bindings come from
// the first of the leading result rows whose bound fields all hold a value
// the target accepts, so a top row with a null or malformed field (a service
// named ":8080") does not become the suggestion; with no such row the edge is
// skipped. empty selects `when: empty` edges over `when: nonempty` ones.
func (b *Book) NextCommands(r *Recipe, params map[string]any, carry Carry, empty bool, rows []map[string]any) []string {
	var out []string
	for _, s := range b.NextSteps(r, params, carry, empty, rows) {
		out = append(out, s.Suggestion())
	}
	return out
}

// NextStep is one applicable follow-up: the target, its bound args and
// carried window/scope, and the command line that runs it.
type NextStep struct {
	Recipe *Recipe
	Args   map[string]string
	Carry  Carry
	Follow bool
	Line   string
}

// NextSteps is NextCommands with the parts of each command, for a caller
// that runs the follow-up itself (`dtctl run --follow`).
func (b *Book) NextSteps(r *Recipe, params map[string]any, carry Carry, empty bool, rows []map[string]any) []NextStep {
	var out []NextStep
	for _, n := range r.Spec.Next {
		switch n.When {
		case "empty":
			if !empty {
				continue
			}
		case "nonempty":
			if empty {
				continue
			}
		}
		target := b.Recipes[n.Recipe]
		if target == nil {
			continue
		}
		args := map[string]string{}
		ok := true
		for k, expr := range n.With {
			v, err := renderWith(expr, params)
			if err == nil && v == "" {
				// Unset here: an optional target param is simply left out
				// ("list services" rather than no suggestion at all).
				if p := target.Spec.Params.Get(k); p == nil || !p.Required {
					continue
				}
			}
			if err != nil || v == "" {
				ok = false
				break
			}
			args[k] = v
		}
		var row map[string]any
		rowWin := n.Window
		if rowWin != nil && rowWin.Literal() {
			rowWin = nil
		}
		if ok && (len(n.Bind) > 0 || rowWin != nil) {
			var bound map[string]string
			bound, row = bindRow(target, n.Bind, rowWin, rows)
			if row == nil {
				ok = false
			}
			for k, v := range bound {
				args[k] = v
			}
		}
		if !ok {
			continue
		}
		c := carryWindow(r, target, carry)
		switch {
		case rowWin != nil:
			c.From, c.To = rowWindow(rowWin, row, target.Spec.Timeframe.Max, time.Now())
		case n.Window != nil:
			c.From, c.To = literalWindow(n.Window, target.Spec.Timeframe.Max)
			if target == r && !widens(r, carry, c.From, time.Now()) {
				continue // already looked that far back
			}
		}
		kept := map[string]string{}
		for k, v := range args {
			kept[k] = v
		}
		out = append(out, NextStep{Recipe: target, Args: kept, Carry: c, Follow: n.Follow, Line: CommandLine(target, args, c)})
	}
	return out
}

// StepInput turns a follow-up's args and carry into the target's input.
func (b *Book) StepInput(s NextStep, now time.Time) (Input, error) {
	in := Input{Params: map[string]any{}, Scope: map[string][]string{}}
	t := s.Recipe
	for k, v := range s.Carry.Scope {
		if contains(t.Spec.Scope, k) {
			in.Scope[k] = v
		}
	}
	for k, v := range s.Args {
		if contains(t.Spec.Scope, k) {
			in.Scope[k] = []string{v}
			continue
		}
		p := t.Spec.Params.Get(k)
		if p == nil {
			return in, fmt.Errorf("%s has no param %q", t.Name(), k)
		}
		val, err := p.ParseValue(v)
		if err != nil {
			return in, err
		}
		in.Params[k] = val
	}
	w, err := ResolveWindow(t.Spec.Timeframe, s.Carry.From, s.Carry.To, now)
	if err != nil {
		return in, err
	}
	in.Window = w
	return in, nil
}

// carryWindow drops an explicit window the target would read differently: a
// trend recipe's 60d is not a sensible window for a "top now" recipe, nor the
// other way round. Scope and segments always carry.
func carryWindow(from, to *Recipe, c Carry) Carry {
	a, b := from.Spec.Timeframe, to.Spec.Timeframe
	if (a.Min > 0) != (b.Min > 0) || a.Align != b.Align || a.Inline != b.Inline {
		c.From, c.To = "", ""
	}
	return c
}

// bindRows bounds how far bindRow looks for a usable row.
const bindRows = 20

// bindRow returns the bind values from the first usable row, and the row;
// nil when no row is usable. A row is usable when every bound field holds a
// value the target accepts and, with a window, the start field holds a time.
// An array binds to a list param as its elements.
func bindRow(target *Recipe, bind map[string]string, win *NextWindow, rows []map[string]any) (map[string]string, map[string]any) {
	for i, row := range rows {
		if i == bindRows {
			break
		}
		if win != nil {
			if _, ok := rowTime(row[win.From]); !ok {
				continue
			}
		}
		vals := map[string]string{}
		for k, field := range bind {
			v, found := row[field]
			if !found || v == nil {
				vals = nil
				break
			}
			p := target.Spec.Params.Get(k)
			s, ok := bindValue(p, v)
			if !ok || s == "" || (p != nil && !accepts(p, s)) {
				vals = nil
				break
			}
			vals[k] = s
		}
		if vals != nil {
			return vals, row
		}
	}
	return nil, nil
}

// bindValue spells a row value as a param value. An array is accepted only
// by a list param, as its elements joined by commas; an element that holds
// a comma itself would split, so such an array does not bind.
func bindValue(p *Param, v any) (string, bool) {
	arr, isArr := v.([]any)
	if !isArr {
		return strings.TrimSpace(fmt.Sprint(v)), true
	}
	if p == nil || p.Type != TypeList {
		return "", false
	}
	parts := make([]string, 0, len(arr))
	for _, e := range arr {
		if e == nil {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(e))
		if s == "" || strings.Contains(s, ",") {
			return "", false
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ","), len(parts) > 0
}

// rowWindow renders the follow-up's --from/--to from the row: start and end
// widened by the pad, end empty (now) when the row has none.
// A window longer than the target's max keeps its start, where the onset
// is, and ends max later.
func rowWindow(w *NextWindow, row map[string]any, max time.Duration, now time.Time) (from, to string) {
	pad, _ := ParseDuration(w.Pad)
	start, _ := rowTime(row[w.From])
	start = start.Add(-pad)
	end, bounded := time.Time{}, false
	if w.To != "" {
		if e, ok := rowTime(row[w.To]); ok {
			end, bounded = e.Add(pad), true
		}
	}
	if !bounded || end.After(now) {
		end, bounded = now, false
	}
	if max > 0 && end.Sub(start) > max {
		end, bounded = start.Add(max), true
	}
	from = start.UTC().Format(time.RFC3339)
	if bounded {
		to = end.UTC().Format(time.RFC3339)
	}
	return from, to
}

// literalWindow renders a window relative to now (from: 30d), its length cut
// at the target's max by moving the start, as rowWindow keeps the onset.
func literalWindow(w *NextWindow, max time.Duration) (from, to string) {
	start, _ := ParseDuration(w.From)
	end, _ := ParseDuration(w.To)
	if max > 0 && start-end > max {
		start = end + max
	}
	from = FormatDuration(start)
	if w.To != "" {
		to = FormatDuration(end)
	}
	return from, to
}

// widens reports whether a relative start reaches further back than the
// window this invocation ran with (its --from, or the recipe's default).
func widens(r *Recipe, carry Carry, from string, now time.Time) bool {
	want, err := ParseDuration(from)
	if err != nil {
		return true
	}
	ran := r.Spec.Timeframe.Default
	if carry.From != "" {
		if d, err := ParseDuration(carry.From); err == nil {
			ran = d
		} else if t, err := time.Parse(time.RFC3339Nano, carry.From); err == nil {
			ran = now.Sub(t)
		}
	}
	return want > ran
}

// windowSpan is the length of a literal window's --from/--to.
func windowSpan(from, to string) time.Duration {
	start, _ := ParseDuration(from)
	end, _ := ParseDuration(to)
	return start - end
}

// rowTime reads a timestamp as DQL returns it: an RFC 3339 string.
func rowTime(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

func accepts(p *Param, s string) bool {
	_, err := p.ParseValue(s)
	return err == nil
}

// renderWith renders a with-binding ("{{.service}}") against raw param values.
func renderWith(expr string, params map[string]any) (string, error) {
	// An unset param is absent from data and renders as "<no value>", which
	// means unset (the lint checks the names a with-template references).
	t, err := template.New("").Parse(expr)
	if err != nil {
		return "", err
	}
	data := map[string]any{}
	for k, v := range params {
		if l, ok := v.([]string); ok {
			v = strings.Join(l, ",")
		}
		data[k] = v
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	s := buf.String()
	if s == "<no value>" {
		return "", nil
	}
	return s, nil
}

// CommandLine builds `dtctl run <recipe> …` for the target with the given
// param (or scope) values, carrying the window, scope and segments the target
// accepts.
//
// Values come from result rows, so every flag is spelled --name=value: a
// value starting with "-" can never be read as a flag of its own. The
// positional slot is used only for a value that cannot be mistaken for one.
func CommandLine(target *Recipe, args map[string]string, carry Carry) string {
	parts := []string{"dtctl", "run", target.Name()}
	if p := target.Spec.Params.Positional(); p != nil {
		if v, ok := args[p.Name]; ok && v != "" && !strings.HasPrefix(v, "-") {
			parts = append(parts, ShellQuote(v))
			delete(args, p.Name)
		}
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	scopeSet := map[string][]string{}
	for k, v := range carry.Scope {
		if contains(target.Spec.Scope, k) {
			scopeSet[k] = v
		}
	}
	for _, k := range keys {
		if contains(target.Spec.Scope, k) {
			scopeSet[k] = []string{args[k]}
			continue
		}
		parts = append(parts, flagArg(k, args[k]))
	}
	for _, d := range target.Spec.Scope {
		for _, v := range scopeSet[d] {
			parts = append(parts, flagArg(d, v))
		}
	}
	tf := target.Spec.Timeframe
	if !tf.None && !tf.Fixed {
		if carry.From != "" {
			parts = append(parts, flagArg("from", carry.From))
		}
		if carry.To != "" {
			parts = append(parts, flagArg("to", carry.To))
		}
	}
	if !target.Spec.Segments.Off() {
		for _, s := range carry.Segments {
			parts = append(parts, flagArg("segment", s))
		}
	}
	return strings.Join(parts, " ")
}

// flagArg spells one flag as a single shell word, --name=value.
func flagArg(name, value string) string {
	return ShellQuote("--" + strings.ReplaceAll(name, "_", "-") + "=" + value)
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9._:/=@,+-]+$`)

// ShellQuote quotes s for a POSIX shell when it needs quoting.
func ShellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Suggestion is the step's command line as offered to run next: with
// --follow when the target has a usual next step of its own.
func (s NextStep) Suggestion() string { return WithFollow(s.Recipe, s.Line) }

// WithFollow adds --follow to a command line for r when one of r's edges is
// marked follow: the usual next step then arrives in the same call.
func WithFollow(r *Recipe, line string) string {
	for _, n := range r.Spec.Next {
		if n.Follow {
			return line + " --follow"
		}
	}
	return line
}

// FollowOrder is steps with the follow-marked ones first, in their order.
func FollowOrder(steps []NextStep) []NextStep {
	out := make([]NextStep, 0, len(steps))
	for _, s := range steps {
		if s.Follow {
			out = append(out, s)
		}
	}
	for _, s := range steps {
		if !s.Follow {
			out = append(out, s)
		}
	}
	return out
}
