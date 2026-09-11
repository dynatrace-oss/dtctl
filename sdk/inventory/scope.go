package inventory

import (
	"regexp"
	"strings"
)

// Scope applicability.
//
// A windowed probe asks "is data matching this scope arriving into this
// signal". For most scope/signal pairs that question is not even askable:
// there is never RUM data under k8s.namespace.name, never a Kubernetes metric
// under service.name, and Davis problems and security findings are generated
// backend-side and carry no ingest-side scope at all.
//
// Grail answers such a probe with zero records and no notification — a filter
// on a field the stream does not have is simply never true. Read naively that
// is indistinguishable from a source that stopped, which is how the feature
// came to report "the stream works, this scope is not producing into it"
// about a stream that could not have carried the scope in the first place.
//
// Field existence cannot be settled up front: dt.system.data_objects carries
// no field list, and `describe` returns only the core fields (6 for
// user.events, none of them the attributes anyone scopes by). So
// applicability is established per signal, at the point a probe comes back
// empty, and only then.

// scopeStringLiteral matches a single- or double-quoted DQL string, escapes
// included, so values never contribute identifiers.
var scopeStringLiteral = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)

// scopeBacktickField matches the backtick-quoted field form, which may hold
// characters a bare identifier cannot.
var scopeBacktickField = regexp.MustCompile("`([^`]+)`")

// scopeIdentifier matches a bare (optionally dotted) DQL field reference.
var scopeIdentifier = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*`)

// scopeReserved are the bare words a scope expression may contain that are
// not field references.
var scopeReserved = map[string]bool{
	"and": true, "or": true, "not": true, "in": true, "is": true,
	"null": true, "true": true, "false": true, "like": true, "between": true,
	"case": true, "when": true, "then": true, "else": true, "end": true,
}

// scopeFields returns the record fields a scope expression references, in
// first-appearance order.
//
// This is a lexical extraction, not a parser, and it is deliberately biased
// toward under-reporting: a field it misses only costs the applicability
// check (the signal keeps today's `empty` verdict), whereas a phantom field
// would make every signal look inapplicable. Function names are dropped by
// looking at the following character rather than by keeping a builtin list,
// so the set stays correct as DQL grows.
func scopeFields(scope string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(f string) {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			return
		}
		seen[f] = true
		out = append(out, f)
	}

	// Backtick-quoted fields are unambiguous; take them, then blank them out
	// so the bare-identifier pass does not see their contents.
	for _, m := range scopeBacktickField.FindAllStringSubmatch(scope, -1) {
		add(m[1])
	}
	masked := blankOut(scopeBacktickField, scope)
	masked = blankOut(scopeStringLiteral, masked)

	for _, loc := range scopeIdentifier.FindAllStringIndex(masked, -1) {
		tok := masked[loc[0]:loc[1]]
		if scopeReserved[strings.ToLower(tok)] {
			continue
		}
		// A name followed by "(" is a function, not a field.
		if strings.HasPrefix(strings.TrimLeft(masked[loc[1]:], " \t"), "(") {
			continue
		}
		add(tok)
	}
	return out
}

// blankOut replaces every match with spaces of the same length, so byte
// offsets into the result still line up with the original.
func blankOut(re *regexp.Regexp, s string) string {
	return re.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat(" ", len(m))
	})
}

// quoteField renders a field name for embedding in generated DQL. Backticks
// are always applied: they are valid around a dotted name and they are the
// only form that survives a field the user backtick-quoted for a reason.
func quoteField(f string) string {
	return "`" + strings.ReplaceAll(f, "`", "") + "`"
}

// scopeFieldPredicate builds the "does this record carry any field the scope
// names" predicate. Any, not all: a scope naming two fields of which one
// exists is a fair question to ask of the signal, just a narrow one.
func scopeFieldPredicate(fields []string) string {
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, "isNotNull("+quoteField(f)+")")
	}
	return strings.Join(parts, " or ")
}

// applicability is the outcome of asking whether a signal can carry a scope
// at all.
type applicability int

const (
	// applicabilityUnknown: the question could not be settled, so the caller
	// keeps whatever verdict it already had. Never an absence claim.
	applicabilityUnknown applicability = iota
	// applicabilityYes: at least one scope field exists on this signal, so an
	// empty result really is an empty result.
	applicabilityYes
	// applicabilityNo: none of the scope's fields exist here.
	applicabilityNo
)

// metricDimensionApplicability reads a timeseries probe's column types to
// decide whether a metric carries any of the scope's fields as a dimension.
//
// This is exact and costs nothing: grouping by a dimension a metric does not
// have returns that column typed "undefined" (verified on a live tenant — 0
// scanned bytes), while a real dimension comes back with its own type.
func metricDimensionApplicability(types map[string]string, fields []string) applicability {
	if len(types) == 0 || len(fields) == 0 {
		return applicabilityUnknown
	}
	anyKnown := false
	for _, f := range fields {
		t, ok := types[f]
		if !ok {
			// The probe did not report on this field, so it cannot be ruled
			// out; without it the "none of them exist" claim is unsupported.
			return applicabilityUnknown
		}
		anyKnown = true
		if t != TypeUndefined {
			return applicabilityYes
		}
	}
	if !anyKnown {
		return applicabilityUnknown
	}
	return applicabilityNo
}

// notApplicableEvidence phrases why a signal was not asked to account for the
// scope, naming the fields so the reader can see it is about the question,
// not about the data.
func notApplicableEvidence(what string, fields []string) string {
	subject := "none of the scope's fields (" + strings.Join(fields, ", ") + ") exist"
	if len(fields) == 1 {
		subject = "the scope's field (" + fields[0] + ") does not exist"
	}
	return "not asked: " + subject + " on " + what +
		", so this signal cannot carry this scope — an empty result here is a property of the question, not of the data"
}
