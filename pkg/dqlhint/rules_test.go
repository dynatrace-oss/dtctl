package dqlhint

import (
	"strings"
	"testing"
)

// span is shorthand for a single-line span, the common case in the fixtures.
func span(startCol, endCol int) *Span {
	return &Span{StartLine: 1, StartColumn: startCol, EndLine: 1, EndColumn: endCol}
}

// The fixtures below are the error payloads the Grail query API returns for
// these queries (error type, arguments, syntaxErrorPosition), captured from a
// live environment. Each rewrite must be a query that fixes the reported
// mistake and nothing else.
func TestSuggest_Rewrites(t *testing.T) {
	tests := []struct {
		name  string
		err   Error
		want  string // the corrected query
		match string // the rule expected to fire
	}{
		{
			name: "summarize by without by: parameter",
			err: Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
				Query: "fetch logs | summarize count() by service.name", Span: span(32, 33)},
			want:  "fetch logs | summarize count(), by:{service.name}",
			match: "by-parameter",
		},
		{
			name: "by with several fields",
			err: Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
				Query: "fetch logs | summarize count() by service.name, host.name", Span: span(32, 33)},
			want:  "fetch logs | summarize count(), by:{service.name, host.name}",
			match: "by-parameter",
		},
		{
			name: "by with braces but no comma",
			err: Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
				Query: "fetch spans | summarize count() by {service.name}", Span: span(33, 34)},
			want:  "fetch spans | summarize count(), by:{service.name}",
			match: "by-parameter",
		},
		{
			name: "by: without the separating comma",
			err: Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
				Query: "fetch logs | summarize count() by: service.name", Span: span(32, 33)},
			want:  "fetch logs | summarize count(), by:{service.name}",
			match: "by-parameter",
		},
		{
			name: "by followed by further commands",
			err: Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
				Query: "fetch logs | summarize n = count() by service.name | sort n desc", Span: span(36, 37)},
			want:  "fetch logs | summarize n = count(), by:{service.name} | sort n desc",
			match: "by-parameter",
		},
		{
			name: "by on a later line of a multi-line query",
			err: Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
				Query: "fetch logs\n| limit 3\n| summarize count() by service.name",
				Span:  &Span{StartLine: 3, StartColumn: 21, EndLine: 3, EndColumn: 22}},
			want:  "fetch logs\n| limit 3\n| summarize count(), by:{service.name}",
			match: "by-parameter",
		},
		{
			name: "by on makeTimeseries",
			err: Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
				Query: "fetch logs | makeTimeseries count() by loglevel", Span: span(37, 38)},
			want:  "fetch logs | makeTimeseries count(), by:{loglevel}",
			match: "by-parameter",
		},
		{
			name: "single = in filter (type error)",
			err: Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE_BUT_WAS", Arguments: []string{"a boolean", "a string"},
				Query: `fetch logs | filter loglevel = "ERROR"`, Span: span(32, 38)},
			want:  `fetch logs | filter loglevel == "ERROR"`,
			match: "filter-equals",
		},
		{
			name: "single = in filter (parse error), every comparison fixed at once",
			err: Error{Type: "PARSE_ERROR", Arguments: []string{"`=`"},
				Query: `fetch logs | filter loglevel = "ERROR" and x = 1`, Span: span(46, 46)},
			want:  `fetch logs | filter loglevel == "ERROR" and x == 1`,
			match: "filter-equals",
		},
		{
			name: "single = in filter leaves assignments and other operators alone",
			err: Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE_BUT_WAS", Arguments: []string{"a boolean", "a long"},
				Query: `fetch logs | fieldsAdd a = 1 | filter a = 1 and b != 2 and c >= 3 and d == "x=y"`, Span: span(43, 43)},
			want:  `fetch logs | fieldsAdd a = 1 | filter a == 1 and b != 2 and c >= 3 and d == "x=y"`,
			match: "filter-equals",
		},
		{
			name: "single = in filterOut",
			err: Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE_BUT_WAS", Arguments: []string{"a boolean", "a string"},
				Query: `fetch logs | filterOut loglevel = "DEBUG"`, Span: span(35, 41)},
			want:  `fetch logs | filterOut loglevel == "DEBUG"`,
			match: "filter-equals",
		},
		{
			name: "colon instead of = for a named aggregation",
			err: Error{Type: "UNKNOWN_PARAMETER_DEFINED", Arguments: []string{"c"},
				Query: `fetch logs | summarize c: countIf(loglevel == "ERROR")`, Span: span(24, 54)},
			want:  `fetch logs | summarize c = countIf(loglevel == "ERROR")`,
			match: "colon-assignment",
		},
		{
			name: "colon assignments fixed together, by: kept",
			err: Error{Type: "UNKNOWN_PARAMETER_DEFINED", Arguments: []string{"errors"},
				Query: `fetch logs | summarize errors: countIf(loglevel == "ERROR"), total: count(), by: {host.name}`, Span: span(24, 59)},
			want:  `fetch logs | summarize errors = countIf(loglevel == "ERROR"), total = count(), by: {host.name}`,
			match: "colon-assignment",
		},
		{
			name: "array.contains to in()",
			err: Error{Type: "UNKNOWN_FUNCTION", Arguments: []string{"array.contains"},
				Query: `fetch logs | filter array.contains(tags, "a")`, Span: span(21, 34)},
			want:  `fetch logs | filter in("a", tags)`,
			match: "array-contains",
		},
		{
			name: "arrayContains to in(), nested arguments",
			err: Error{Type: "UNKNOWN_FUNCTION", Arguments: []string{"arrayContains"},
				Query: `fetch logs | filter arrayContains(splitString(tags, ","), lower("A"))`, Span: span(21, 33)},
			want:  `fetch logs | filter in(lower("A"), splitString(tags, ","))`,
			match: "array-contains",
		},
		{
			name: "rollup: duration at the command level is interval:",
			err: Error{Type: "NAMED_PARAMETER_HAS_TO_BE", Arguments: []string{"rollup", "a predefined identifier (allowed: min, max, sum, avg, total)"},
				Query: "timeseries x = avg(dt.host.cpu.usage), rollup: 5m", Span: span(48, 49)},
			want:  "timeseries x = avg(dt.host.cpu.usage), interval: 5m",
			match: "rollup-duration",
		},
		{
			name: "rollup: duration inside the aggregation moves to interval:",
			err: Error{Type: "NAMED_PARAMETER_HAS_TO_BE", Arguments: []string{"rollup", "a predefined identifier (allowed: min, max, sum, avg, total)"},
				Query: "timeseries avg(dt.host.cpu.usage, rollup: 5m), by: {host.name}", Span: span(43, 44)},
			want:  "timeseries avg(dt.host.cpu.usage), by: {host.name}, interval: 5m",
			match: "rollup-duration",
		},
		{
			name: "positional condition on timeseries is filter:",
			err: Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE", Arguments: []string{"a metric-based timeseries aggregation"},
				Query: `timeseries avg(dt.host.cpu.usage), dt.entity.host == "HOST-1"`, Span: span(36, 61)},
			want:  `timeseries avg(dt.host.cpu.usage), filter: dt.entity.host == "HOST-1"`,
			match: "timeseries-condition",
		},
		{
			name: "classic entity table to smartscapeNodes",
			err: Error{Type: "UNKNOWN_DATA_OBJECT", Arguments: []string{"dt.entity.service"},
				Query: "fetch dt.entity.service | fields id, entity.name", Span: span(7, 23)},
			want:  `smartscapeNodes "SERVICE" | fields id, name`,
			match: "entity-table",
		},
		{
			name: "classic host table to smartscapeNodes",
			err: Error{Type: "UNKNOWN_DATA_OBJECT", Arguments: []string{"dt.entity.host"},
				Query: "fetch dt.entity.host\n| filter contains(entity.name, \"prod\")", Span: span(7, 20)},
			want:  "smartscapeNodes \"HOST\"\n| filter contains(name, \"prod\")",
			match: "entity-table",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := hints(tt.err)
			if len(hs) != 1 {
				t.Fatalf("hints = %+v, want exactly one", hs)
			}
			if hs[0].rule != tt.match {
				t.Errorf("rule = %q, want %q", hs[0].rule, tt.match)
			}
			if hs[0].query != tt.want {
				t.Errorf("rewrite =\n  %s\nwant\n  %s", hs[0].query, tt.want)
			}
			s := Suggest(tt.err)
			if len(s) != 1 || !strings.HasSuffix(s[0], "dtctl query "+shellQuote(tt.want)) {
				t.Errorf("Suggest = %q, want a runnable dtctl query suggestion", s)
			}
		})
	}
}

// TestSuggest_Advice covers the traps whose fix depends on something the error
// does not reveal (which data source holds the field), so the hint explains the
// fix instead of rewriting the query.
func TestSuggest_Advice(t *testing.T) {
	tests := []struct {
		name  string
		err   Error
		wants []string
	}{
		{
			name: "named non-timeseries aggregation",
			err: Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE", Arguments: []string{"a metric-based timeseries aggregation"},
				Query: "timeseries x = last(loglevel)", Span: span(16, 29)},
			wants: []string{"last()", "timeseries x = avg(loglevel)", "makeTimeseries count(), by:{loglevel}"},
		},
		{
			name: "unnamed non-timeseries aggregation",
			err: Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE", Arguments: []string{"a metric-based timeseries aggregation"},
				Query: "timeseries takeLast(dt.host.cpu.usage)", Span: span(12, 38)},
			wants: []string{"takeLast()", "timeseries avg(dt.host.cpu.usage)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Suggest(tt.err)
			if len(s) != 1 {
				t.Fatalf("Suggest = %q, want one advice", s)
			}
			for _, want := range tt.wants {
				if !strings.Contains(s[0], want) {
					t.Errorf("advice %q lacks %q", s[0], want)
				}
			}
		})
	}
}

// TestSuggest_Conservative pins that a rule stays silent unless its pattern
// clearly matches: a wrong "fix" costs an agent more than no hint.
func TestSuggest_Conservative(t *testing.T) {
	tests := []struct {
		name string
		err  Error
	}{
		{"no query", Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"}, Span: span(1, 2)}},
		{"no position", Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
			Query: "fetch logs | summarize count() by service.name"}},
		{"unrelated error type", Error{Type: "FIELD_DOES_NOT_EXIST", Arguments: []string{"foo"},
			Query: "fetch logs | fields foo", Span: span(21, 23)}},
		{"position does not point at by", Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
			Query: "fetch logs | summarize count() by service.name", Span: span(7, 10)}},
		{"by followed by an expression", Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
			Query: "fetch logs | summarize count() by lower(service.name)", Span: span(32, 33)}},
		{"by on a command without a by: parameter", Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
			Query: "fetch logs | sort timestamp by service.name", Span: span(29, 30)}},
		{"type error in filter without a single =", Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE_BUT_WAS",
			Arguments: []string{"a boolean", "a string"}, Query: `fetch logs | filter "x"`, Span: span(21, 23)}},
		{"single = outside a filter", Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE_BUT_WAS",
			Arguments: []string{"a boolean", "a string"}, Query: `fetch logs | fieldsAdd a = "x" | filter a`, Span: span(41, 41)}},
		{"type error that is not about a boolean", Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE_BUT_WAS",
			Arguments: []string{"a long", "a string"}, Query: `fetch logs | filter x = "a"`, Span: span(25, 27)}},
		{"unknown parameter that is a real parameter name", Error{Type: "UNKNOWN_PARAMETER_DEFINED",
			Arguments: []string{"filter"}, Query: "fetch logs | summarize count(), filter: x", Span: span(33, 41)}},
		{"unknown parameter not at the reported position", Error{Type: "UNKNOWN_PARAMETER_DEFINED",
			Arguments: []string{"c"}, Query: "fetch logs | summarize c: count()", Span: span(1, 5)}},
		{"array.contains with three arguments", Error{Type: "UNKNOWN_FUNCTION", Arguments: []string{"array.contains"},
			Query: `fetch logs | filter array.contains(tags, "a", "b")`, Span: span(21, 34)}},
		{"nested array.contains calls", Error{Type: "UNKNOWN_FUNCTION", Arguments: []string{"array.contains"},
			Query: `fetch logs | filter array.contains(array.contains(tags, "a"), "b")`, Span: span(21, 34)}},
		{"mismatched bracket kinds", Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE_BUT_WAS",
			Arguments: []string{"a boolean", "a long"}, Query: `fetch logs | filter (x = 1]`, Span: span(26, 26)}},
		{"unknown function that is not an array membership test", Error{Type: "UNKNOWN_FUNCTION",
			Arguments: []string{"containz"}, Query: `fetch logs | filter containz(content, "a")`, Span: span(21, 28)}},
		{"rollup with a non-duration value", Error{Type: "NAMED_PARAMETER_HAS_TO_BE",
			Arguments: []string{"rollup", "a predefined identifier"}, Query: "timeseries avg(dt.host.cpu.usage), rollup: mean", Span: span(44, 47)}},
		{"rollup duration with interval already set", Error{Type: "NAMED_PARAMETER_HAS_TO_BE",
			Arguments: []string{"rollup", "a predefined identifier"}, Query: "timeseries avg(dt.host.cpu.usage), interval: 1m, rollup: 5m", Span: span(58, 59)}},
		{"timeseries condition with filter: already set", Error{Type: "MANDATORY_PARAMETER_HAS_TO_BE",
			Arguments: []string{"a metric-based timeseries aggregation"},
			Query:     `timeseries avg(dt.host.cpu.usage), filter: true, dt.entity.host == "HOST-1"`, Span: span(50, 75)}},
		{"entity table without a known smartscape type", Error{Type: "UNKNOWN_DATA_OBJECT",
			Arguments: []string{"dt.entity.process_group"}, Query: "fetch dt.entity.process_group", Span: span(7, 29)}},
		{"entity table with fetch parameters", Error{Type: "UNKNOWN_DATA_OBJECT",
			Arguments: []string{"dt.entity.service"}, Query: "fetch dt.entity.service, from: now()-1d", Span: span(7, 23)}},
		{"query with a comment", Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
			Query: "fetch logs // recent\n| summarize count() by service.name", Span: &Span{StartLine: 2, StartColumn: 21, EndLine: 2, EndColumn: 22}}},
		{"position past the end of the query", Error{Type: "PARSE_ERROR", Arguments: []string{"`by`"},
			Query: "fetch logs", Span: span(40, 41)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if s := Suggest(tt.err); len(s) != 0 {
				t.Errorf("Suggest = %q, want none", s)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct{ in, want string }{
		{`fetch logs | filter a == "x"`, `'fetch logs | filter a == "x"'`},
		{`fetch logs | filter a == "it's"`, `'fetch logs | filter a == "it'\''s"'`},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

// TestApply_OverlappingEditsLeaveQueryUnchanged is the backstop for any rule
// that emits overlapping edits: no panic, and no rewrite.
func TestApply_OverlappingEditsLeaveQueryUnchanged(t *testing.T) {
	q := "abcdef"
	if got := apply(q, []edit{{1, 4, "X"}, {2, 3, "Y"}}); got != q {
		t.Errorf("apply = %q, want %q unchanged", got, q)
	}
	if got := apply(q, []edit{{4, 5, "Y"}, {1, 2, "X"}}); got != "aXcdYf" {
		t.Errorf("apply = %q, want aXcdYf", got)
	}
}
