package exec

import (
	"regexp"
	"sort"
	"strings"
)

// ScopeNeed is one Grail storage scope a DQL query provably needs, with the
// parts of the query that need it (e.g. "fetch logs", "getNodeName()").
type ScopeNeed struct {
	Scope   string
	Because []string
}

// dataObjectScopes maps a fetchable data object to the storage scope that reads
// it. Only objects whose scope is certain are listed: an object missing here
// yields no requirement, which is the safe answer — a wrong entry would make
// the scope precheck refuse a query that would have run.
var dataObjectScopes = map[string]string{
	"logs":            "storage:logs:read",
	"spans":           "storage:spans:read",
	"events":          "storage:events:read",
	"bizevents":       "storage:bizevents:read",
	"security.events": "storage:security.events:read",
	"user.events":     "storage:user.events:read",
	"user.sessions":   "storage:user.sessions:read",
}

// entityDataObjectPrefix covers the classic entity views (dt.entity.host,
// dt.entity.service, ...), which all read through one scope.
const entityDataObjectPrefix = "dt.entity."

const (
	scopeSmartscape = "storage:smartscape:read"
	scopeMetrics    = "storage:metrics:read"
	scopeEntities   = "storage:entities:read"
)

// commandScopes are DQL commands that read a storage scope on their own.
var commandScopes = map[string]string{
	"smartscapeNodes": scopeSmartscape,
	"smartscapeEdges": scopeSmartscape,
	"timeseries":      scopeMetrics,
}

// functionScopes are DQL functions that look up Smartscape data for every
// record, so a query calling one needs the Smartscape scope whatever it fetches.
var functionScopes = map[string]string{
	"getNodeName":  scopeSmartscape,
	"getNodeField": scopeSmartscape,
}

var (
	// A command sits at the start of the query, after a pipe, or at the start of
	// a subquery ([...]). Anything else named "fetch" is a field or a value.
	fetchRe   = regexp.MustCompile(`(?:^|[|\[])\s*fetch\s+([A-Za-z_][A-Za-z0-9_.]*)`)
	commandRe = regexp.MustCompile(`(?:^|[|\[])\s*(smartscapeNodes|smartscapeEdges|timeseries)\b`)
	// A function is a name directly followed by "(". The leading class keeps
	// e.g. "x.getNodeName(" or "mygetNodeName(" from matching.
	functionRe = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.])(getNodeName|getNodeField)\s*\(`)

	storageScopeRe = regexp.MustCompile(`storage:[a-z0-9_.-]+:read`)
)

// StorageScopeForDataObject returns the storage scope that reads the named data
// object, and false when that is not known for certain.
func StorageScopeForDataObject(name string) (string, bool) {
	if s, ok := dataObjectScopes[name]; ok {
		return s, true
	}
	if strings.HasPrefix(name, entityDataObjectPrefix) && len(name) > len(entityDataObjectPrefix) {
		return scopeEntities, true
	}
	return "", false
}

// StorageScopesInText returns the distinct storage read scopes spelled out in s,
// sorted. Grail's authorization errors sometimes name the scope they wanted.
func StorageScopesInText(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range storageScopeRe.FindAllString(s, -1) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// RequiredStorageScopes returns the storage scopes the query provably needs,
// sorted by scope. It is deliberately a lower bound: it recognizes only the
// commands, data objects and functions whose scope is certain, ignores text in
// string literals, comments and backtick identifiers, and returns nothing for a
// query it cannot tokenize. A scope missing from the answer does not mean the
// query can run; a scope in it means the query cannot run without it.
func RequiredStorageScopes(query string) []ScopeNeed {
	code, ok := stripDQLLiterals(query)
	if !ok {
		return nil
	}

	because := map[string]map[string]bool{}
	add := func(scope, reason string) {
		if because[scope] == nil {
			because[scope] = map[string]bool{}
		}
		because[scope][reason] = true
	}

	for _, m := range fetchRe.FindAllStringSubmatch(code, -1) {
		if scope, ok := StorageScopeForDataObject(m[1]); ok {
			add(scope, "fetch "+m[1])
		}
	}
	for _, m := range commandRe.FindAllStringSubmatch(code, -1) {
		add(commandScopes[m[1]], m[1])
	}
	for _, m := range functionRe.FindAllStringSubmatch(code, -1) {
		add(functionScopes[m[1]], m[1]+"()")
	}

	if len(because) == 0 {
		return nil
	}
	needs := make([]ScopeNeed, 0, len(because))
	for scope, reasons := range because {
		n := ScopeNeed{Scope: scope}
		for r := range reasons {
			n.Because = append(n.Because, r)
		}
		sort.Strings(n.Because)
		needs = append(needs, n)
	}
	sort.Slice(needs, func(i, j int) bool { return needs[i].Scope < needs[j].Scope })
	return needs
}

// stripDQLLiterals blanks out string literals ("...", """..."""), backtick
// identifiers and comments, so that only DQL code is left to match against.
// Each is replaced by a single space. It reports false for an unterminated
// literal or comment: the query will not parse, and guessing at where the
// literal ends could invent a requirement.
func stripDQLLiterals(q string) (string, bool) {
	var b strings.Builder
	b.Grow(len(q))
	for i := 0; i < len(q); {
		switch {
		case strings.HasPrefix(q[i:], `"""`):
			end := strings.Index(q[i+3:], `"""`)
			if end < 0 {
				return "", false
			}
			i += 3 + end + 3
			b.WriteByte(' ')
		case q[i] == '"':
			j := i + 1
			for ; j < len(q) && q[j] != '"'; j++ {
				if q[j] == '\\' {
					j++
				}
			}
			if j >= len(q) {
				return "", false
			}
			i = j + 1
			b.WriteByte(' ')
		case q[i] == '`':
			end := strings.IndexByte(q[i+1:], '`')
			if end < 0 {
				return "", false
			}
			i += 1 + end + 1
			b.WriteByte(' ')
		case strings.HasPrefix(q[i:], "//"):
			end := strings.IndexByte(q[i:], '\n')
			if end < 0 {
				i = len(q)
			} else {
				i += end
			}
			b.WriteByte(' ')
		case strings.HasPrefix(q[i:], "/*"):
			end := strings.Index(q[i+2:], "*/")
			if end < 0 {
				return "", false
			}
			i += 2 + end + 2
			b.WriteByte(' ')
		default:
			b.WriteByte(q[i])
			i++
		}
	}
	return b.String(), true
}
