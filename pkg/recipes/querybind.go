package recipes

import (
	"regexp"
	"strings"
	"time"
)

// A pointer an agent has to complete costs it a turn: which flag takes the
// problem ID, which window, which cluster? The query it just wrote already
// says. BindQuery reads the values a recipe takes from the query's
// `field == "value"` filters and its window, so the hint is a command the
// agent can run as it stands.

var (
	// eqLiteral is `field == "value"` (either side).
	eqLiteral    = regexp.MustCompile(`([A-Za-z_][\w.]*)\s*==\s*"((?:[^"\\]|\\.)*)"`)
	eqLiteralRev = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"\s*==\s*([A-Za-z_][\w.]*)`)
	// eqParam is `field == "<param>"` in a recipe rendered with placeholders.
	eqParam    = regexp.MustCompile(`([A-Za-z_][\w.]*)\s*==\s*"<(\w+)>"`)
	eqParamRev = regexp.MustCompile(`"<(\w+)>"\s*==\s*([A-Za-z_][\w.]*)`)
	// queryFrom is a relative start: from: now()-2h, from: -2h.
	queryFrom = regexp.MustCompile(`\bfrom\s*:\s*(?:now\(\)\s*)?-\s*(\d+[smhdw])\b`)
)

// paramFields maps the fields a recipe compares a param with to the param:
// {display_id: id, dt.service.name: service, service.name: service}.
func paramFields(exampleDQL string) map[string]string {
	out := map[string]string{}
	for _, m := range eqParam.FindAllStringSubmatch(exampleDQL, -1) {
		out[m[1]] = m[2]
	}
	for _, m := range eqParamRev.FindAllStringSubmatch(exampleDQL, -1) {
		out[m[2]] = m[1]
	}
	return out
}

// queryLiterals maps each field a query compares with a string literal to
// the first such literal.
func queryLiterals(dql string) map[string]string {
	dql = stripDQLComments(dql)
	out := map[string]string{}
	for _, m := range eqLiteral.FindAllStringSubmatch(dql, -1) {
		if _, seen := out[m[1]]; !seen {
			out[m[1]] = unescapeDQL(m[2])
		}
	}
	for _, m := range eqLiteralRev.FindAllStringSubmatch(dql, -1) {
		if _, seen := out[m[2]]; !seen {
			out[m[2]] = unescapeDQL(m[1])
		}
	}
	return out
}

func unescapeDQL(s string) string {
	return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(s)
}

// QueryBinding is what a query says about a recipe's invocation.
type QueryBinding struct {
	Args  map[string]string
	Carry Carry
	// Subject is true when a required param was bound: the query looks up
	// the very thing the recipe takes (a problem ID, a service).
	Subject bool
}

// BindQuery reads r's params, scope and window from an ad-hoc query. from is
// the query's --from flag, if any.
func (b *Book) BindQuery(r *Recipe, dql, from string) QueryBinding {
	qb := QueryBinding{Args: map[string]string{}}
	lits := queryLiterals(dql)
	if len(lits) > 0 {
		for field, name := range b.recipeSigs().sigs[r.Name()].params {
			p := r.Spec.Params.Get(name)
			v, ok := lits[field]
			if p == nil || !ok || qb.Args[name] != "" || !accepts(p, v) {
				continue
			}
			qb.Args[name] = v
			if p.Required {
				qb.Subject = true
			}
		}
		for _, d := range r.Spec.Scope {
			if dim := b.Scopes[d]; dim != nil && dim.Field != "" {
				if v, ok := lits[dim.Field]; ok && v != "" {
					if qb.Carry.Scope == nil {
						qb.Carry.Scope = map[string][]string{}
					}
					qb.Carry.Scope[d] = []string{v}
				}
			}
		}
	}
	if from == "" {
		if m := queryFrom.FindStringSubmatch(stripDQLComments(dql)); m != nil {
			from = m[1]
		}
	}
	if d, err := ParseDuration(from); err == nil {
		tf := r.Spec.Timeframe
		if !tf.None && !tf.Fixed && (tf.Max == 0 || d <= tf.Max) && d >= tf.Min && d != tf.Default {
			qb.Carry.From = from
		}
	}
	return qb
}

// HintCommand is CommandLine plus a <name> stand-in for each required param
// the binding left open, so the agent sees what it still has to supply.
func HintCommand(r *Recipe, qb QueryBinding) string {
	args := map[string]string{}
	for k, v := range qb.Args {
		args[k] = v
	}
	line := CommandLine(r, args, qb.Carry)
	for _, p := range r.Spec.Params {
		if !p.Required || p.HasDefault() || qb.Args[p.Name] != "" {
			continue
		}
		if p.Positional {
			line = strings.Replace(line, "dtctl run "+r.Name(), "dtctl run "+r.Name()+" <"+p.Name+">", 1)
		} else {
			line += " --" + p.FlagName() + "=<" + p.Name + ">"
		}
	}
	return WithFollow(r, line)
}

// DefaultQueryWindow is what a query reads when neither --from nor its
// fetch names a window.
const DefaultQueryWindow = 2 * time.Hour

// EffectiveQueryWindow is QueryWindow, or DefaultQueryWindow when the query
// names no window at all; 0 when the window is absolute.
func EffectiveQueryWindow(dql, from string) time.Duration {
	if from == "" && !windowArg.MatchString(stripDQLComments(dql)) {
		return DefaultQueryWindow
	}
	return QueryWindow(dql, from)
}

// QueryWindow is the length of a query's relative window, from its --from
// flag or its own from:, or 0 when it has none or an absolute one.
func QueryWindow(dql, from string) time.Duration {
	if from == "" {
		if m := queryFrom.FindStringSubmatch(stripDQLComments(dql)); m != nil {
			from = m[1]
		}
	}
	d, err := ParseDuration(from)
	if err != nil {
		return 0
	}
	return d
}
