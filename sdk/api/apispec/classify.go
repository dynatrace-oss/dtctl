package apispec

import "strings"

// Access is how much a scope verb permits. It is the specification-derived half
// of classifying a generic request; mapping it onto a safety operation is the
// caller's job, because that mapping is CLI policy rather than API knowledge.
type Access string

const (
	// AccessRead permits reading only.
	AccessRead Access = "read"
	// AccessWrite permits creating or modifying.
	AccessWrite Access = "write"
	// AccessDelete permits deletion.
	AccessDelete Access = "delete"
	// AccessUnknown means the scope verb could not be classified — either no
	// scope was declared, or its verb is not in the table below.
	//
	// This is a first-class outcome, not an error case. It must never be
	// collapsed into AccessRead: a caller that treats "unknown" as "read" turns
	// every unclassifiable mutation into a permitted one.
	AccessUnknown Access = "unknown"
)

// scopeVerbAccess maps the trailing verb of a Dynatrace scope to what it permits.
//
// A scope reads `<service>:<resource>:<verb>`, e.g. `document:documents:read`.
// The verbs are not a clean read/write/delete triple — `use`, `claim`, `restore`
// and `admin` all appear — so the mapping is a table with an explicit unknown
// case rather than a heuristic.
//
// A verb absent from this table classifies as [AccessUnknown], which callers
// gate strictly. That default is the point: a newly minted scope verb must not
// silently land in the most permissive bucket.
var scopeVerbAccess = map[string]Access{
	"read": AccessRead,
	// `use` grants invocation of a shared resource without changing it
	// (iam:service-users:use).
	"use":   AccessRead,
	"write": AccessWrite,
	// `claim` and `restore` change state without creating or destroying
	// (document:trash.documents:restore).
	"claim":   AccessWrite,
	"restore": AccessWrite,
	"share":   AccessWrite,
	"install": AccessWrite,
	"delete":  AccessDelete,
	// `truncate` destroys the contents of a store: destructive, so it is
	// classified with deletion rather than with writes.
	"truncate": AccessDelete,
	// `admin` and `execute` are deliberately absent. `admin` is unbounded, and
	// `execute` says nothing about whether the executed thing mutates — both
	// must fall through to AccessUnknown and be gated strictly.
}

// ScopeVerb returns the trailing verb of a scope string, or "" when the scope is
// not in `<service>:<resource>:<verb>` form.
func ScopeVerb(scope string) string {
	i := strings.LastIndex(scope, ":")
	if i < 0 || i == len(scope)-1 {
		return ""
	}
	return scope[i+1:]
}

// AccessForScopes classifies a declared scope set.
//
// The *most* permissive verb across the set decides, because a caller holding
// several scopes for one operation can exercise all of them, and the gate has to
// cover the strongest. An unclassifiable verb anywhere makes the whole set
// unknown — one unrecognised scope is enough to invalidate a conclusion drawn
// from the others.
//
// An empty scope set is [AccessUnknown]: it means the document declared nothing,
// which is common (two of six APIs sampled declare no per-operation scopes) and
// is exactly the case that must not be mistaken for "harmless".
func AccessForScopes(scopes []string) Access {
	if len(scopes) == 0 {
		return AccessUnknown
	}

	rank := map[Access]int{AccessRead: 0, AccessWrite: 1, AccessDelete: 2}
	best := AccessRead
	found := false

	for _, s := range scopes {
		access, ok := scopeVerbAccess[ScopeVerb(s)]
		if !ok {
			return AccessUnknown
		}
		if !found || rank[access] > rank[best] {
			best, found = access, true
		}
	}
	if !found {
		return AccessUnknown
	}
	return best
}

// Access classifies the operation from the scopes its specification declares.
func (o Operation) Access() Access { return AccessForScopes(o.Scopes) }

// MethodIsRead reports whether an HTTP method is conventionally a read.
//
// This is a one-sided floor, never a classification: GET and HEAD are reads by
// convention, and PUT/PATCH/DELETE are never reads. POST carries no information
// at all and must never be inferred from — measured across 64 specified
// operations, 12 POSTs required only a `:read` scope while one required
// `:delete`, so method inference is wrong in both directions on POST, including
// the dangerous one.
func MethodIsRead(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "HEAD":
		return true
	default:
		return false
	}
}

// MethodIsAlwaysMutating reports whether a method cannot be a read whatever a
// specification claims. It is the lower bound the specification cannot lower.
func MethodIsAlwaysMutating(method string) bool {
	switch strings.ToUpper(method) {
	case "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}
