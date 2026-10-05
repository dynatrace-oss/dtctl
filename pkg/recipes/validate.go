package recipes

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"text/template/parse"
)

var (
	nameRe   = regexp.MustCompile(`^[a-z][a-z0-9-]{1,48}$`)
	domainRe = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	paramRe  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	// topLevelFrom finds a from:/to: in the first command of a query, the one
	// the default timeframe would otherwise apply to.
	topLevelFrom = regexp.MustCompile(`\b(from|to)\s*:`)
)

// ReservedFlags are the framework flags every recipe command carries. A param
// may not be named after one (in its flag spelling).
var ReservedFlags = map[string]bool{
	"from": true, "to": true, "dry-run": true, "output": true, "segment": true,
	"segments-file": true, "segment-var": true, "help": true, "metadata": true,
	"max-result-records": true, "max-result-bytes": true, "default-scan-limit-gbytes": true,
	"no-query-limits": true, "include-contributions": true, "context": true,
	"agent": true, "plain": true, "jq": true, "chunk-size": true, "verbose": true,
	"debug": true, "config": true, "max-items": true, "max-bytes": true,
	"max-line-width": true, "full": true, "spill": true, "spill-dir": true,
	"no-spill": true, "compact": true, "no-compact": true,
	fieldScope: true, fieldWindow: true,
}

var validTypes = map[string]bool{TypeString: true, TypeInt: true, TypeBool: true, TypeEnum: true, TypeList: true}

// Validate checks a recipe against the schema and against this book's
// registries (domains, scopes, fragments). It does not check `next` targets
// or `requires` names; see Lint.
func (b *Book) Validate(r *Recipe) error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	name := r.Name()
	switch {
	case !nameRe.MatchString(name):
		add("metadata.name %q must match %s", name, nameRe)
	case !strings.Contains(name, "-"):
		add("metadata.name %q must be <domain>-<name>", name)
	case b.Domains[r.Domain()] == nil:
		add("domain %q is not registered (known: %s); a layer adds one in _domains.yaml", r.Domain(), strings.Join(sortedKeys(b.Domains), ", "))
	}
	if r.Metadata.Version < 1 {
		add("metadata.version must be a positive integer")
	}
	s := &r.Spec
	switch {
	case strings.TrimSpace(s.Summary) == "":
		add("spec.summary is required")
	case strings.Contains(s.Summary, "\n") || len(s.Summary) > 100:
		add("spec.summary must be one line of at most 100 characters")
	}
	if strings.TrimSpace(s.DQL) == "" {
		add("spec.dql is required")
	}
	if strings.TrimSpace(s.Means) == "" {
		add("spec.means is required")
	}
	if strings.TrimSpace(s.EmptyMeans) == "" {
		add("spec.emptyMeans is required")
	}
	if !s.Timeframe.Declared {
		add("spec.timeframe is required (a duration, none, or a map)")
	}
	tf := s.Timeframe
	if tf.Align != "" && tf.Align != AlignUTCDay {
		add("spec.timeframe.align must be %s", AlignUTCDay)
	}
	if tf.Fixed && tf.Inline {
		add("spec.timeframe cannot be both fixed and inline")
	}
	if tf.Min > 0 && tf.Min > tf.Default {
		add("spec.timeframe.min is longer than its default")
	}
	if tf.Max > 0 && tf.Max < tf.Default {
		add("spec.timeframe.max is shorter than its default")
	}
	if tf.Max > 0 && tf.Fixed {
		add("spec.timeframe.max has no effect on a fixed window")
	}

	positional := 0
	for _, p := range s.Params {
		if !paramRe.MatchString(p.Name) {
			add("param %q: name must match %s", p.Name, paramRe)
		}
		if ReservedFlags[p.FlagName()] || b.Scopes[p.Name] != nil {
			add("param %q: name is reserved for a framework or scope flag", p.Name)
		}
		if p.Type == "" {
			p.Type = TypeString
		}
		if !validTypes[p.Type] {
			add("param %q: unknown type %q", p.Name, p.Type)
			continue
		}
		if p.Positional {
			positional++
			if p.Type == TypeBool || p.Type == TypeList {
				add("param %q: a %s param cannot be positional", p.Name, p.Type)
			}
		}
		if p.Type == TypeEnum && len(p.Values) == 0 {
			add("param %q: an enum needs values", p.Name)
		}
		if p.Render != "" && (p.Render != "identifier" || p.Type != TypeEnum) {
			add("param %q: render: identifier is only valid on an enum", p.Name)
		}
		if p.Render == "identifier" {
			for _, v := range p.Values {
				if !plainIdentifier.MatchString(v) {
					add("param %q: value %q cannot render as an identifier", p.Name, v)
				}
			}
		}
		if (p.Min != nil || p.Max != nil) && p.Type != TypeInt {
			add("param %q: min/max apply to int params only", p.Name)
		}
		if p.Pattern != "" {
			if p.Type != TypeString && p.Type != TypeList {
				add("param %q: pattern applies to string and list params only", p.Name)
			} else if _, err := regexp.Compile(p.Pattern); err != nil {
				add("param %q: invalid pattern: %v", p.Name, err)
			}
		}
		if p.Required && p.HasDefault() {
			add("param %q: a required param has no default", p.Name)
		}
		if _, err := p.DefaultValue(); err != nil {
			add("%v", err)
		}
	}
	if positional > 1 {
		add("at most one param may be positional")
	}
	for _, d := range s.Scope {
		if b.Scopes[d] == nil {
			add("spec.scope: unknown dimension %q (known: %s)", d, strings.Join(sortedKeys(b.Scopes), ", "))
		}
	}
	switch s.Empty {
	case "", EmptyNoRows, EmptyZeroRow:
	default:
		add("spec.empty must be %s or %s", EmptyNoRows, EmptyZeroRow)
	}
	for i, n := range s.Next {
		if n.Recipe == "" {
			add("spec.next[%d]: recipe is required", i)
		}
		if w := n.Window; w != nil {
			if w.From == "" {
				add("spec.next[%d].window.from is required (the row field holding the start, or a duration such as 30d)", i)
			}
			_, toErr := ParseDuration(w.To)
			switch {
			case w.From == "":
			case w.Literal() && w.To != "" && toErr != nil:
				add("spec.next[%d].window: with a duration from, to must be a duration too or absent", i)
			case w.Literal() && w.Pad != "":
				add("spec.next[%d].window: pad widens row times; a duration from has none", i)
			case !w.Literal() && w.To != "" && toErr == nil:
				add("spec.next[%d].window: to is a duration but from is a row field; use durations for both or fields for both", i)
			}
			if w.Pad != "" {
				if d, err := ParseDuration(w.Pad); err != nil || d < 0 {
					add("spec.next[%d].window.pad must be a duration such as 5m", i)
				}
			}
		}
		switch n.When {
		case "", "always", "empty", "nonempty":
		default:
			add("spec.next[%d]: when must be always, empty or nonempty", i)
		}
		for k, v := range n.With {
			if _, err := template.New("").Option("missingkey=error").Parse(v); err != nil {
				add("spec.next[%d].with.%s: %v", i, k, err)
			}
		}
	}
	for i := range s.Checks {
		errs = append(errs, validateCheck(i, &s.Checks[i])...)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if err := b.validateTemplate(r); err != nil {
		return err
	}
	return nil
}

// validateTemplate parses the DQL with its fragments and checks what it
// references: only declared params and the framework fields the recipe opted
// into.
func (b *Book) validateTemplate(r *Recipe) error {
	t, err := b.template(r)
	if err != nil {
		return err
	}
	fields, invoked := templateRefs(t, r.Name())
	for name := range invoked {
		if t.Lookup(name) == nil {
			return fmt.Errorf("dql: template %q is not defined by any fragment", name)
		}
	}
	var errs []error
	for f := range fields {
		switch {
		case f == fieldScope:
			if len(r.Spec.Scope) == 0 {
				errs = append(errs, fmt.Errorf("dql references .scope but spec.scope declares no dimension"))
			}
		case f == fieldWindow:
			if !r.Spec.Timeframe.Inline {
				errs = append(errs, fmt.Errorf("dql references .window, which only an inline timeframe provides"))
			}
		case r.Spec.Params.Get(f) == nil:
			errs = append(errs, fmt.Errorf("dql references .%s, which is not a declared param", f))
		}
	}
	if len(r.Spec.Scope) > 0 && !fields[fieldScope] {
		errs = append(errs, fmt.Errorf("spec.scope is declared but the dql never places .scope.stage or .scope.expr"))
	}
	if r.Spec.Timeframe.Inline && !fields[fieldWindow] {
		errs = append(errs, fmt.Errorf("an inline timeframe must write .window.from/.window.to into the dql"))
	}
	if !r.Spec.Timeframe.Inline && !r.Spec.Timeframe.None {
		if first := firstCommand(r.Spec.DQL); topLevelFrom.MatchString(first) {
			errs = append(errs, fmt.Errorf("the dql sets its own from:/to:; a recipe takes its window from --from/--to (or declares timeframe: {inline: true})"))
		}
	}
	return errors.Join(errs...)
}

// firstCommand is the query's first command (up to the first pipe on a new
// line), where a from:/to: would override the default timeframe.
func firstCommand(dql string) string {
	if i := strings.Index(dql, "\n|"); i >= 0 {
		return dql[:i]
	}
	return dql
}

// defineNames lists the {{define}} names in a fragment file.
func defineNames(text string) ([]string, error) {
	trees, err := parse.Parse("fragment", text, "", "", templateFuncsForParse)
	if err != nil {
		return nil, err
	}
	var out []string
	for name := range trees {
		if name != "fragment" {
			out = append(out, name)
		}
	}
	return out, nil
}

var templateFuncsForParse = map[string]any{"timeAdd": timeAdd}

// LintIssue is a cross-recipe finding: a broken `next` edge, a requirement no
// capability definition names, a fragment nobody uses.
type LintIssue struct {
	Recipe  string
	Message string
}

// Lint checks the merged book for issues that span recipes. capabilities is
// the set of capability names inventory knows (built-in plus app-defined).
func (b *Book) Lint(capabilities map[string]bool) []LintIssue {
	var out []LintIssue
	used := map[string]bool{}
	for _, r := range b.Sorted() {
		for _, req := range r.Spec.Requires {
			if !capabilities[req] {
				out = append(out, LintIssue{r.Name(), fmt.Sprintf("requires %q, which no capability definition names", req)})
			}
		}
		for _, n := range r.Spec.Next {
			target := b.Recipes[n.Recipe]
			if target == nil {
				out = append(out, LintIssue{r.Name(), fmt.Sprintf("next: recipe %q does not exist", n.Recipe)})
				continue
			}
			for k, expr := range n.With {
				if target.Spec.Params.Get(k) == nil && !contains(target.Spec.Scope, k) {
					out = append(out, LintIssue{r.Name(), fmt.Sprintf("next: %s has no param or scope %q", n.Recipe, k)})
				}
				t, err := template.New("").Parse(expr)
				if err != nil {
					out = append(out, LintIssue{r.Name(), fmt.Sprintf("next: %s: with %s: %v", n.Recipe, k, err)})
					continue
				}
				fields, _ := templateRefs(t, "")
				for f := range fields {
					if r.Spec.Params.Get(f) == nil {
						out = append(out, LintIssue{r.Name(), fmt.Sprintf("next: %s: with %s uses .%s, which is not a param of %s", n.Recipe, k, f, r.Name())})
					}
				}
			}
			for k := range n.Bind {
				if target.Spec.Params.Get(k) == nil && !contains(target.Spec.Scope, k) {
					out = append(out, LintIssue{r.Name(), fmt.Sprintf("next: %s has no param or scope %q", n.Recipe, k)})
				}
			}
			if n.Window != nil && (target.Spec.Timeframe.None || target.Spec.Timeframe.Fixed) {
				out = append(out, LintIssue{r.Name(), fmt.Sprintf("next: %s takes no window, so window: has nothing to set", n.Recipe)})
			}
			if w := n.Window; w != nil && w.Literal() {
				from, to := literalWindow(w, target.Spec.Timeframe.Max)
				if span := windowSpan(from, to); target.Spec.Timeframe.Min > 0 && span < target.Spec.Timeframe.Min {
					out = append(out, LintIssue{r.Name(), fmt.Sprintf("next: window %s is shorter than %s's minimum %s", FormatDuration(span), n.Recipe, FormatDuration(target.Spec.Timeframe.Min))})
				}
			}
		}
		for _, msg := range b.DQLLint(r) {
			out = append(out, LintIssue{r.Name(), msg})
		}
		for _, msg := range b.lintChecks(r) {
			out = append(out, LintIssue{r.Name(), msg})
		}
		if r.Spec.Deprecated != nil && r.Spec.Deprecated.ReplacedBy != "" && b.Recipes[r.Spec.Deprecated.ReplacedBy] == nil {
			out = append(out, LintIssue{r.Name(), fmt.Sprintf("deprecated.replacedBy %q does not exist", r.Spec.Deprecated.ReplacedBy)})
		}
		if t, err := b.template(r); err == nil {
			_, invoked := templateRefs(t, r.Name())
			for n := range invoked {
				used[n] = true
			}
		}
	}
	for _, f := range b.fragments {
		names, _ := defineNames(f.text)
		for _, n := range names {
			if !used[n] {
				out = append(out, LintIssue{"", fmt.Sprintf("fragment %q (%s) is used by no recipe", n, f.source.Location)})
			}
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
