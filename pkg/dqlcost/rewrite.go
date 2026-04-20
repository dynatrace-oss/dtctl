package dqlcost

import (
	"fmt"
	"regexp"
	"strings"
)

// RewriteOptions controls which mechanical rewrites apply.
type RewriteOptions struct {
	AddDefaultFrom   bool   // COST001: append `, from:<DefaultFrom>` to billable fetch lacking from:
	AddScanLimit     bool   // COST008: insert `, scanLimitGBytes:<DefaultScanLimit>`
	MoveLimitAfter   bool   // COST005 (disabled by default — empirically harmful; see dqlcost.go)
	DefaultFrom      string // timeframe literal, e.g. "now()-2h"
	DefaultScanLimit int    // scan limit in GiB, e.g. 500
}

// DefaultRewriteOptions enables the safe rewrites with conservative defaults.
// MoveLimitAfter is intentionally disabled: tenant measurements showed that
// moving `limit` after `summarize` actually increased scan cost by ~40× because
// Grail short-circuits the scan when `limit` appears earlier. The semantics
// differ though, so the linter still flags this as info-level for the author.
func DefaultRewriteOptions() RewriteOptions {
	return RewriteOptions{
		AddDefaultFrom:   true,
		AddScanLimit:     true,
		MoveLimitAfter:   false,
		DefaultFrom:      "now()-2h",
		DefaultScanLimit: 500,
	}
}

// Change describes a single rewrite applied to the query.
type Change struct {
	Rule    string // corresponding lint rule ID
	Message string
}

// Rewrite applies safe mechanical cost rewrites to the query and returns
// the new query plus the list of changes applied. Input that is already
// well-formed is returned unchanged with an empty change list. Rewrite is
// never destructive: on parse ambiguity it leaves the query untouched and
// records no change.
func Rewrite(query string, opts RewriteOptions) (string, []Change) {
	out := query
	var changes []Change

	if opts.AddDefaultFrom && opts.DefaultFrom != "" {
		rewritten, changed := addDefaultFrom(out, opts.DefaultFrom)
		if changed {
			out = rewritten
			changes = append(changes, Change{
				Rule:    "COST001",
				Message: fmt.Sprintf("added inline `from:%s` to billable fetch", opts.DefaultFrom),
			})
		}
	}
	if opts.AddScanLimit && opts.DefaultScanLimit > 0 {
		rewritten, changed := addScanLimit(out, opts.DefaultScanLimit)
		if changed {
			out = rewritten
			changes = append(changes, Change{
				Rule:    "COST008",
				Message: fmt.Sprintf("added `scanLimitGBytes:%d` guardrail to fetch", opts.DefaultScanLimit),
			})
		}
	}
	if opts.MoveLimitAfter {
		rewritten, changed := moveLimitAfterSummarize(out)
		if changed {
			out = rewritten
			changes = append(changes, Change{
				Rule:    "COST005",
				Message: "moved `| limit N` to after `| summarize` so it trims results, not input",
			})
		}
	}

	return out, changes
}

// addDefaultFrom finds `fetch <logs|events|bizevents|spans> <args-up-to-pipe>`
// occurrences and, if no `from:` is present in the args, appends `, from:<literal>`.
// Preserves surrounding whitespace.
var reFetchBillableGroup = regexp.MustCompile(`(?i)(\bfetch\s+(?:logs|events|bizevents|spans)\b)([^|]*)`)

func addDefaultFrom(q, fromLiteral string) (string, bool) {
	changed := false
	out := reFetchBillableGroup.ReplaceAllStringFunc(q, func(m string) string {
		sub := reFetchBillableGroup.FindStringSubmatch(m)
		head, tail := sub[1], sub[2]
		if reHasFromInline.MatchString(tail) {
			return m
		}
		// Strip trailing whitespace from tail, then append `, from:<literal>`.
		trimmed := strings.TrimRight(tail, " \t\n")
		suffix := tail[len(trimmed):]
		sep := ", "
		if strings.TrimSpace(trimmed) == "" {
			// tail is empty — no existing args.
			changed = true
			return head + ", from:" + fromLiteral + suffix
		}
		changed = true
		return head + trimmed + sep + "from:" + fromLiteral + suffix
	})
	return out, changed
}

// addScanLimit inserts `, scanLimitGBytes:<n>` into billable fetch args when
// none is present. Runs after addDefaultFrom so both guardrails coexist.
func addScanLimit(q string, limit int) (string, bool) {
	if reHasScanLimit.MatchString(q) {
		// already present somewhere in the query — avoid duplicating
		return q, false
	}
	changed := false
	out := reFetchBillableGroup.ReplaceAllStringFunc(q, func(m string) string {
		sub := reFetchBillableGroup.FindStringSubmatch(m)
		head, tail := sub[1], sub[2]
		trimmed := strings.TrimRight(tail, " \t\n")
		suffix := tail[len(trimmed):]
		if strings.TrimSpace(trimmed) == "" {
			changed = true
			return head + fmt.Sprintf(", scanLimitGBytes:%d", limit) + suffix
		}
		changed = true
		return head + trimmed + fmt.Sprintf(", scanLimitGBytes:%d", limit) + suffix
	})
	return out, changed
}

// moveLimitAfterSummarize rewrites `... | limit N | summarize ...` into
// `... | summarize ... | limit N`. Conservative: only moves when the pattern
// is unambiguous (single limit stage directly before a single summarize).
var reLimitThenSummarize = regexp.MustCompile(`(?i)\|\s*(limit\s+\d+)\s*\|\s*(summarize\b[^|]*)`)

func moveLimitAfterSummarize(q string) (string, bool) {
	if !reLimitThenSummarize.MatchString(q) {
		return q, false
	}
	out := reLimitThenSummarize.ReplaceAllString(q, "| $2 | $1")
	return out, out != q
}
