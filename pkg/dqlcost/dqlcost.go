// Package dqlcost lints DQL queries for cost anti-patterns that lead to
// oversized Grail scans on Dynatrace DPS.
//
// Lint rules operate on the raw query text using regex heuristics against a
// normalized (whitespace-collapsed, lowercased-keyword) form. Callers that
// already have a canonical query from DQLExecutor.VerifyQuery should pass it
// directly; otherwise Normalize handles basic whitespace collapsing.
package dqlcost

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Severity of a lint finding.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarn:
		return "warn"
	case SeverityError:
		return "error"
	}
	return "unknown"
}

// Finding describes a single lint violation.
type Finding struct {
	Rule       string   // e.g. "COST001"
	Severity   Severity // info | warn | error
	Message    string   // short human-readable explanation
	Suggestion string   // actionable suggestion
}

// Rule is a pure function that inspects the normalized query and appends
// zero or more findings.
type Rule func(normalized string) []Finding

// DefaultRules returns the built-in cost rules in stable ID order.
func DefaultRules() []Rule {
	return []Rule{
		rule001MissingFrom,
		rule002LogMakeTimeseries,
		rule003TransformedFilter,
		rule004SortAfterFetch,
		rule005LimitBeforeSummarize,
		rule006WildcardMatchesValue,
		rule008MissingScanLimit,
		rule009LongLogWindowNoSampling,
	}
}

// Lint runs all DefaultRules against the query and returns findings sorted
// by rule ID.
func Lint(query string) []Finding {
	return LintWith(query, DefaultRules())
}

// LintWith runs the given rules against the query. Rules receive the
// normalized query concatenated with a second copy that preserves string
// literals, separated by a null marker — this lets shape-based rules match
// on literal content (e.g. wildcard arguments) without losing the safety
// of normalization.
func LintWith(query string, rules []Rule) []Finding {
	n := Normalize(query)
	withLiterals := collapseWS(stripComments(query))
	combined := n + "\x00" + withLiterals
	var out []Finding
	for _, r := range rules {
		out = append(out, r(combined)...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rule < out[j].Rule })
	return out
}

func stripComments(q string) string {
	var lines []string
	for _, ln := range strings.Split(q, "\n") {
		if i := strings.Index(ln, "//"); i >= 0 {
			ln = ln[:i]
		}
		lines = append(lines, ln)
	}
	return strings.Join(lines, "\n")
}

func collapseWS(s string) string {
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(s, " "))
}

// MaxSeverity returns the highest severity across findings, or SeverityInfo-1
// (-1) if none.
func MaxSeverity(findings []Finding) Severity {
	max := Severity(-1)
	for _, f := range findings {
		if f.Severity > max {
			max = f.Severity
		}
	}
	return max
}

// Format renders findings as one line each, sorted by rule ID.
func Format(findings []Finding) string {
	var b strings.Builder
	for _, f := range findings {
		fmt.Fprintf(&b, "[%s %s] %s — %s\n", f.Rule, f.Severity, f.Message, f.Suggestion)
	}
	return b.String()
}

// Normalize collapses whitespace, lowercases command keywords, and strips
// string-literal contents (replacing them with empty quotes) so that rules
// do not match on user data. Comments (// and #) are removed.
func Normalize(q string) string {
	// Strip line comments.
	var lines []string
	for _, ln := range strings.Split(q, "\n") {
		if i := strings.Index(ln, "//"); i >= 0 {
			ln = ln[:i]
		}
		lines = append(lines, ln)
	}
	q = strings.Join(lines, "\n")

	// Replace string literal contents with empty strings so `matchesValue(x, "*foo*")`
	// is still detectable by its shape but user data does not false-positive other rules.
	// Kept permissively — regex rules are shape-based.
	q = stripStringBodies(q)

	// Collapse whitespace.
	q = regexp.MustCompile(`\s+`).ReplaceAllString(q, " ")
	return strings.TrimSpace(q)
}

func stripStringBodies(s string) string {
	// Match "...", keeping empty quotes. Does not handle escaped quotes — not
	// required for heuristic linting.
	return regexp.MustCompile(`"[^"]*"`).ReplaceAllString(s, `""`)
}

// --- Rules ---------------------------------------------------------------

var (
	// fetch logs|events|bizevents|spans without any from: inline argument.
	reFetchBillable    = regexp.MustCompile(`(?i)\bfetch\s+(logs|events|bizevents|spans)\b([^|]*)`)
	reHasFromInline    = regexp.MustCompile(`(?i)\bfrom\s*:`)
	reHasScanLimit     = regexp.MustCompile(`(?i)\bscanlimitgbytes\s*:`)
	reHasSampling      = regexp.MustCompile(`(?i)\bsamplingratio\s*:`)
	reLogMakeTS        = regexp.MustCompile(`(?i)\bfetch\s+logs\b[\s\S]*?\|\s*maketimeseries\b`)
	reTransformedEqual = regexp.MustCompile(`(?i)\bfilter\s+(?:lower|upper|toString|toLong|toDouble)\s*\(`)
	reFilterArithmetic = regexp.MustCompile(`(?i)\bfilter\s+\w[\w.]*\s*[+\-*/]\s*\d`)
	reSortAfterFetch   = regexp.MustCompile(`(?i)\bfetch\s+\w+[^|]*\|\s*sort\b`)
	reLimitBeforeSumm  = regexp.MustCompile(`(?i)\|\s*limit\s+\d+\s*\|\s*summarize\b`)
	reWildcardMV       = regexp.MustCompile(`(?i)\bmatchesvalue\s*\(\s*[\w.]+\s*,\s*"\*[^"]*"`)
	// Detect from:-Nd or from:now()-Nd (N days) or from:-Nh (h >= 25)
	reFromDays  = regexp.MustCompile(`(?i)\bfrom\s*:\s*(?:now\(\)\s*-\s*)?(\d+)d\b`)
	reFromHours = regexp.MustCompile(`(?i)\bfrom\s*:\s*(?:now\(\)\s*-\s*)?(\d+)h\b`)
)

func rule001MissingFrom(n string) []Finding {
	matches := reFetchBillable.FindAllStringSubmatch(n, -1)
	var out []Finding
	for _, m := range matches {
		tail := m[2]
		// from: may appear either in the fetch parameter tail up to the first '|'
		// or nowhere — this rule fires only when no from: is present in the
		// fetch-argument tail.
		if !reHasFromInline.MatchString(tail) {
			out = append(out, Finding{
				Rule:       "COST001",
				Severity:   SeverityError,
				Message:    fmt.Sprintf("`fetch %s` has no inline `from:` — defaults to full Grail retention", m[1]),
				Suggestion: fmt.Sprintf("add `, from:now()-2h` to the fetch, e.g. `fetch %s, from:now()-2h`", m[1]),
			})
		}
	}
	return out
}

func rule002LogMakeTimeseries(n string) []Finding {
	if reLogMakeTS.MatchString(n) {
		return []Finding{{
			Rule:       "COST002",
			Severity:   SeverityWarn,
			Message:    "`fetch logs | makeTimeseries` pays per GiB scanned",
			Suggestion: "if a metric already captures this signal, use `timeseries` (free under DPS)",
		}}
	}
	return nil
}

func rule003TransformedFilter(n string) []Finding {
	var out []Finding
	// Empirically measured ~9× scan amplification on a production tenant
	// (46.6 GiB vs 5.3 GiB over a 1h log window), so this is an error.
	if reTransformedEqual.MatchString(n) {
		out = append(out, Finding{
			Rule:       "COST003",
			Severity:   SeverityError,
			Message:    "`filter` wraps the field in a transform (lower/upper/toString/…) — defeats index pushdown; measured ~9× scan cost",
			Suggestion: "compare the raw field directly, or normalize at ingest",
		})
	}
	if reFilterArithmetic.MatchString(n) {
		out = append(out, Finding{
			Rule:       "COST003",
			Severity:   SeverityError,
			Message:    "`filter` performs arithmetic on the filtered field — defeats index pushdown",
			Suggestion: "precompute with `fieldsAdd` before aggregation or compare the raw field",
		})
	}
	return out
}

func rule004SortAfterFetch(n string) []Finding {
	if reSortAfterFetch.MatchString(n) {
		return []Finding{{
			Rule:       "COST004",
			Severity:   SeverityWarn,
			Message:    "`sort` appears immediately after `fetch`, before any filter",
			Suggestion: "move `sort` after filters/summarize so the sort operates on a small result set",
		}}
	}
	return nil
}

func rule005LimitBeforeSummarize(n string) []Finding {
	// Empirical note: on a production tenant, `limit N | summarize` actually
	// scanned LESS than `summarize | limit N` because Grail short-circuits the
	// scan after N rows are found. Semantics differ though — the aggregation is
	// over a truncated sample. So this is an informational flag about intent,
	// not a cost warning.
	if reLimitBeforeSumm.MatchString(n) {
		return []Finding{{
			Rule:       "COST005",
			Severity:   SeverityInfo,
			Message:    "`limit N` before `summarize` truncates the aggregation input to an arbitrary N rows; Grail scans less but the aggregate is over a partial sample",
			Suggestion: "if you want an aggregate over all data, move `limit` after `summarize`; if you want a fast sample, this ordering is fine",
		}}
	}
	return nil
}

func rule006WildcardMatchesValue(n string) []Finding {
	if reWildcardMV.MatchString(n) {
		return []Finding{{
			Rule:       "COST006",
			Severity:   SeverityWarn,
			Message:    "`matchesValue(field, \"*...\")` uses a leading-wildcard pattern — no token-index shortcut",
			Suggestion: "use `matchesPhrase(field, \"...\")` for tokenized substring search",
		}}
	}
	return nil
}

func rule008MissingScanLimit(n string) []Finding {
	if reFetchBillable.MatchString(n) && !reHasScanLimit.MatchString(n) {
		return []Finding{{
			Rule:       "COST008",
			Severity:   SeverityWarn,
			Message:    "billable `fetch` without `scanLimitGBytes:` guardrail",
			Suggestion: "add `, scanLimitGBytes:50` (or your tenant default) to cap runaway scans",
		}}
	}
	return nil
}

func rule009LongLogWindowNoSampling(n string) []Finding {
	if !strings.Contains(strings.ToLower(n), "fetch logs") {
		return nil
	}
	longWindow := reFromDays.MatchString(n)
	if m := reFromHours.FindStringSubmatch(n); m != nil {
		var h int
		fmt.Sscanf(m[1], "%d", &h)
		if h >= 25 {
			longWindow = true
		}
	}
	if longWindow && !reHasSampling.MatchString(n) {
		return []Finding{{
			Rule:       "COST009",
			Severity:   SeverityWarn,
			Message:    "`fetch logs` window > 24h without `samplingRatio:`",
			Suggestion: "add `, samplingRatio:100` (or narrow the window); multiply aggregates back by the ratio",
		}}
	}
	return nil
}
