package exec

import (
	"fmt"
	"regexp"
	"strings"
)

// unsortedSummarizeAdvice explains a record-limit cut on a grouped aggregation
// that was never ranked. `summarize ..., by:{...}` returns its groups in no
// particular order, so when the result limit cuts it the rows kept are an
// arbitrary subset, not the top ones — and the generic "aggregate in DQL"
// advice does not help, because the query already aggregates. The fix is to
// rank before the cut. Returns "" when the advice does not apply.
func unsortedSummarizeAdvice(query string, notifications []QueryNotification) string {
	if !resultLimitHit(notifications) {
		return ""
	}
	agg, ok := unsortedGroupedAggregation(query)
	if !ok {
		return ""
	}
	return fmt.Sprintf("the truncated rows come from `summarize ... by:{...}` with no sort, so the groups kept are arbitrary, not the top ones — rank them before the cut: append | sort %s desc | limit N", agg)
}

// resultLimitHit reports whether a warning-or-worse notification says the
// record/byte cap cut the result.
func resultLimitHit(notifications []QueryNotification) bool {
	for _, n := range notifications {
		switch strings.ToUpper(n.Severity) {
		case "WARNING", "WARN", "ERROR":
		default:
			continue
		}
		if classifyNotification(n.NotificationType, n.Message) == notifResultLimit {
			return true
		}
	}
	return false
}

// dqlParamRe matches a named command parameter such as `by:{...}`.
var dqlParamRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*:`)

// unsortedGroupedAggregation finds the last `summarize` stage of a query and,
// when it groups (by:) and no later stage sorts, returns its first aggregation
// as a field reference a `sort` stage accepts.
func unsortedGroupedAggregation(query string) (string, bool) {
	stages := splitTopLevel(query, '|')
	last := -1
	for i, s := range stages {
		if stageCommand(s) == "summarize" {
			last = i
		}
	}
	if last < 0 {
		return "", false
	}
	for _, s := range stages[last+1:] {
		if stageCommand(s) == "sort" {
			return "", false
		}
	}

	args := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stages[last]), "summarize"))
	grouped := false
	agg := ""
	for _, part := range splitTopLevel(args, ',') {
		part = strings.TrimSpace(part)
		if m := dqlParamRe.FindStringSubmatch(part); m != nil {
			grouped = grouped || m[1] == "by"
			continue
		}
		if agg == "" && part != "" {
			agg = aggregationFieldName(part)
		}
	}
	if !grouped || agg == "" {
		return "", false
	}
	return agg, true
}

// aggregationFieldName returns the output field an aggregation produces: its
// alias when it has one, otherwise the expression itself, backquoted as DQL
// requires for a name like `count()`.
func aggregationFieldName(expr string) string {
	if i := topLevelAssign(expr); i >= 0 {
		return strings.TrimSpace(expr[:i])
	}
	return "`" + expr + "`"
}

// stageCommand returns the command word that starts a pipeline stage.
func stageCommand(stage string) string {
	fields := strings.Fields(stage)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// splitTopLevel splits s on sep where sep is outside quotes and brackets.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '(' || c == '{' || c == '[':
			depth++
		case c == ')' || c == '}' || c == ']':
			depth--
		case c == sep && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

// topLevelAssign returns the index of an alias `=` outside brackets and
// quotes — not part of `==`, `!=`, `<=` or `>=` — or -1.
func topLevelAssign(s string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '(' || c == '{' || c == '[':
			depth++
		case c == ')' || c == '}' || c == ']':
			depth--
		case c == '=' && depth == 0:
			if i+1 < len(s) && s[i+1] == '=' {
				i++
				continue
			}
			if i > 0 && strings.IndexByte("=!<>", s[i-1]) >= 0 {
				continue
			}
			return i
		}
	}
	return -1
}

// queryNotificationAdvice is notificationAdvice plus the advice that needs the
// query text to decide: the ranking hint for a truncated, unsorted summarize
// follows the generic result-limit advice it refines.
func queryNotificationAdvice(query string, notifications []QueryNotification) (warnings, suggestions []string) {
	warnings, suggestions = notificationAdvice(notifications)
	if advice := unsortedSummarizeAdvice(query, notifications); advice != "" {
		suggestions = append(suggestions, "# "+advice)
	}
	return warnings, suggestions
}
