package recipes

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DQL lints encode query traps that were each measured to give a wrong or
// needlessly expensive answer without any error. They run on the recipe as
// `describe` shows it (defaults, placeholders, default window), so they see
// the statement an agent would adapt, not the template.
//
// Every rule names what goes wrong, because an author fixing a lint is
// deciding whether the trap applies, and "style" is not a reason to change a
// working query.

// dqlLint is one rule over the rendered statement's top-level stages.
type dqlLint struct {
	code  string
	check func(r *Recipe, stages []string) string
	// style marks a rule about how a recipe reads, not a wrong answer: it
	// is not applied to an agent's own query.
	style bool
}

var dqlLints = []dqlLint{
	{"sampling-source", lintSamplingSource, false},
	{"sampling-unscaled", lintSamplingUnscaled, false},
	{"limit-before-aggregate", lintLimitBeforeAggregate, false},
	{"coalesce-filter", lintCoalesceFilter, false},
	{"case-folded-filter", lintCaseFoldedFilter, false},
	{"unaliased-aggregate", lintUnaliasedAggregate, true},
	{"multi-key-timeseries", lintMultiKeyTimeseries, false},
	{"interval-equals-window", lintIntervalEqualsWindow, false},
	{"filter-beyond-window", lintFilterBeyondWindow, false},
}

// DQLLint returns the trap lints for one recipe, as "code: message".
func (b *Book) DQLLint(r *Recipe) []string {
	rendered, err := b.Example(r, time.Now())
	if err != nil {
		return nil // Validate reports a recipe that cannot render
	}
	stages := dqlStages(stripDQLComments(rendered.DQL))
	var out []string
	for _, l := range dqlLints {
		if msg := l.check(r, stages); msg != "" {
			out = append(out, l.code+": "+msg)
		}
	}
	return out
}

// LintQuery applies the trap lints to an ad-hoc query, whose window is
// window (0 when unknown), as "code: message".
func LintQuery(dql string, window time.Duration) []string {
	r := &Recipe{Spec: Spec{Timeframe: Timeframe{Default: window}}}
	stages := dqlStages(stripDQLComments(dql))
	var out []string
	for _, l := range dqlLints {
		if l.style {
			continue
		}
		if msg := l.check(r, stages); msg != "" {
			out = append(out, l.code+": "+msg)
		}
	}
	return out
}

var fetchSource = regexp.MustCompile(`^fetch\s+([A-Za-z_][\w.]*)`)

func lintSamplingSource(_ *Recipe, stages []string) string {
	for _, s := range stages {
		m := fetchSource.FindStringSubmatch(s)
		if m == nil || !strings.Contains(s, "samplingRatio") {
			continue
		}
		if m[1] != "logs" && m[1] != "spans" {
			return fmt.Sprintf("samplingRatio on fetch %s: only logs and spans honour it (events ignore it with a warning, problems reject it)", m[1])
		}
	}
	return ""
}

func lintSamplingUnscaled(_ *Recipe, stages []string) string {
	all := strings.Join(stages, "\n")
	if !strings.Contains(all, "samplingRatio") || strings.Contains(all, "sampling_ratio") {
		return ""
	}
	if strings.Contains(all, "count(") || strings.Contains(all, "countIf(") || strings.Contains(all, "sum(") {
		return "a sampled count must be scaled back: sum(coalesce(dt.system.sampling_ratio, 1)) instead of count(); unscaled, it under-reports by the ratio with no error"
	}
	return ""
}

var aggregateStage = map[string]bool{"summarize": true, "makeTimeseries": true, "fieldsSummary": true}

func lintLimitBeforeAggregate(_ *Recipe, stages []string) string {
	limited := false
	for _, s := range stages {
		switch cmd := stageCmd(s); {
		case cmd == "limit":
			limited = true
		case limited && aggregateStage[cmd]:
			return "| limit before | " + cmd + " aggregates an arbitrary subset: a limit is not a sample; limit after aggregating"
		}
	}
	return ""
}

var coalesceCompare = regexp.MustCompile(`coalesce\([^()]*(\([^()]*\)[^()]*)*\)\s*(==|!=)`)

func lintCoalesceFilter(_ *Recipe, stages []string) string {
	for _, s := range stages {
		if c := stageCmd(s); (c == "filter" || c == "filterOut") && coalesceCompare.MatchString(s) {
			return "coalesce(...) == in a filter defeats the field index; compare each field (a == x or b == x)"
		}
	}
	return ""
}

// caseFold matches lower(/upper( applied to anything but a string literal:
// lower("<id>") folds the value, not the field, and leaves the index alone.
var caseFold = regexp.MustCompile(`\b(lower|upper)\(\s*[^"\s)]`)

func lintCaseFoldedFilter(_ *Recipe, stages []string) string {
	for _, s := range stages {
		if c := stageCmd(s); (c == "filter" || c == "filterOut") && caseFold.MatchString(s) {
			return "lower()/upper() on a field in a filter defeats the n-gram index and scans every record (115 GB against an index skip, measured on a rare term over 10m); use contains(f, \"x\", caseSensitive: false), matchesPhrase or matchesValue, which match case-insensitively on the raw field"
		}
	}
	return ""
}

var namedArg = regexp.MustCompile(`^[A-Za-z_]\w*\s*:`)

func lintUnaliasedAggregate(_ *Recipe, stages []string) string {
	for _, s := range stages {
		cmd := stageCmd(s)
		if cmd != "summarize" && cmd != "timeseries" && cmd != "makeTimeseries" {
			continue
		}
		args := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), cmd))
		for _, part := range splitDQL(args, ',') {
			part = strings.TrimSpace(part)
			if part == "" || namedArg.MatchString(part) || strings.HasPrefix(part, "{") {
				continue
			}
			if assignIndex(part) < 0 && strings.Contains(part, "(") {
				return fmt.Sprintf("%s aggregate %q has no alias: its column is named by the expression, which an agent's adapted query or --jq then has to backquote", cmd, part)
			}
		}
	}
	return ""
}

var metricKeyArg = regexp.MustCompile(`\b(?:avg|sum|min|max|count|percentile|median)\(\s*([A-Za-z][\w.:-]*)`)

func lintMultiKeyTimeseries(_ *Recipe, stages []string) string {
	for _, s := range stages {
		if stageCmd(s) != "timeseries" {
			continue
		}
		keys := map[string]bool{}
		for _, m := range metricKeyArg.FindAllStringSubmatch(s, -1) {
			keys[m[1]] = true
		}
		if len(keys) > 1 && !unionArg.MatchString(s) {
			return fmt.Sprintf("a timeseries over %d metric keys keeps only the series every key reports: a host without one of them, or a window without an OOM kill, drops out with no error (default: does not help); add union: true", len(keys))
		}
	}
	return ""
}

var unionArg = regexp.MustCompile(`\bunion\s*:\s*true\b`)

var intervalArg = regexp.MustCompile(`\binterval\s*:\s*(\d+[smhd])\b`)

func lintIntervalEqualsWindow(r *Recipe, stages []string) string {
	tf := r.Spec.Timeframe
	if tf.Default <= 0 {
		return ""
	}
	for _, s := range stages {
		if c := stageCmd(s); c != "timeseries" && c != "makeTimeseries" {
			continue
		}
		m := intervalArg.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		if d, err := ParseDuration(m[1]); err == nil && d == tf.Default {
			return "interval equal to the window: the aligned grid straddles two buckets and reads up to 2x; use a smaller interval and sum, or summarize instead"
		}
	}
	return ""
}

var (
	// timestampFilter is a lower bound on the record time in a filter:
	// timestamp > now() - 24h.
	timestampFilter = regexp.MustCompile(`\b(?:timestamp|start_time|end_time)\s*>=?\s*now\(\)\s*-\s*(\d+[smhdw])\b`)
	// windowArg is a fetch's own window.
	windowArg = regexp.MustCompile(`\b(?:from|to|timeframe)\s*:`)
)

// lintFilterBeyondWindow: a filter cannot reach past the window the fetch
// reads, so `filter timestamp > now() - 24h` with the default window searches
// the last 2h only and says nothing about it. Measured: agents concluded
// "nothing happened yesterday" from exactly this query.
func lintFilterBeyondWindow(r *Recipe, stages []string) string {
	window := r.Spec.Timeframe.Default
	if window <= 0 {
		return ""
	}
	for i, s := range stages {
		if stageCmd(s) != "fetch" || windowArg.MatchString(s) {
			continue
		}
		for _, f := range stages[i+1:] {
			if stageCmd(f) != "filter" {
				continue
			}
			m := timestampFilter.FindStringSubmatch(f)
			if m == nil {
				continue
			}
			if d, err := ParseDuration(m[1]); err == nil && d > window {
				return fmt.Sprintf("filters the last %s but the query reads only %s, so older records were never fetched; widen the window instead: fetch ..., from: now()-%s (or --from %s)", m[1], formatWindow(window), m[1], m[1])
			}
		}
	}
	return ""
}

func formatWindow(d time.Duration) string {
	if d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	}
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", d/time.Hour)
	}
	return d.String()
}

// stageCmd is the command word of a pipeline stage.
func stageCmd(stage string) string {
	f := strings.Fields(stage)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// dqlStages splits a statement at its top-level pipes.
func dqlStages(dql string) []string {
	var out []string
	for _, s := range splitDQL(dql, '|') {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// splitDQL splits at sep outside strings, backquotes and brackets.
func splitDQL(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '`':
			quote = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == sep && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// assignIndex is the index of a top-level "=" that is an alias, not part of
// ==, !=, <= or >=; -1 if none.
func assignIndex(s string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '`':
			quote = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == '=' && depth == 0:
			prev, next := byte(0), byte(0)
			if i > 0 {
				prev = s[i-1]
			}
			if i+1 < len(s) {
				next = s[i+1]
			}
			if next != '=' && prev != '=' && prev != '!' && prev != '<' && prev != '>' {
				return i
			}
		}
	}
	return -1
}

// stripDQLComments removes // line comments outside strings.
func stripDQLComments(dql string) string {
	var b strings.Builder
	for _, line := range strings.Split(dql, "\n") {
		var quote byte
		cut := len(line)
		for i := 0; i < len(line); i++ {
			c := line[i]
			if quote != 0 {
				if c == '\\' && quote == '"' {
					i++
				} else if c == quote {
					quote = 0
				}
				continue
			}
			if c == '"' || c == '`' {
				quote = c
				continue
			}
			if c == '/' && i+1 < len(line) && line[i+1] == '/' {
				cut = i
				break
			}
		}
		b.WriteString(line[:cut])
		b.WriteByte('\n')
	}
	return b.String()
}
