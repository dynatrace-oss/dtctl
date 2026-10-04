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
}

var dqlLints = []dqlLint{
	{"sampling-source", lintSamplingSource},
	{"sampling-unscaled", lintSamplingUnscaled},
	{"limit-before-aggregate", lintLimitBeforeAggregate},
	{"coalesce-filter", lintCoalesceFilter},
	{"unaliased-aggregate", lintUnaliasedAggregate},
	{"multi-key-timeseries", lintMultiKeyTimeseries},
	{"interval-equals-window", lintIntervalEqualsWindow},
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
