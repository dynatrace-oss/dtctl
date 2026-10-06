package recipes

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"

	"gopkg.in/yaml.v3"
)

// Reserved template fields: framework data, never a param name.
const (
	fieldScope  = "scope"
	fieldWindow = "window"
)

// Param values as handed to a template: String() is the escaped DQL literal,
// while `eq` still compares the raw value.
type (
	strLit   string
	intLit   int64
	boolLit  bool
	identLit string
	listLit  []strLit
)

func (s strLit) String() string   { return QuoteString(string(s)) }
func (i intLit) String() string   { return strconv.FormatInt(int64(i), 10) }
func (b boolLit) String() string  { return strconv.FormatBool(bool(b)) }
func (s identLit) String() string { return string(s) }
func (l listLit) String() string {
	parts := make([]string, len(l))
	for i, s := range l {
		parts[i] = s.String()
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// QuoteString renders s as a DQL string literal.
func QuoteString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

var plainIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// QuoteField renders a field name, backtick-quoting non-identifiers (`cost-center`).
func QuoteField(name string) string {
	if plainIdentifier.MatchString(name) {
		return name
	}
	return "`" + name + "`"
}

// ParseValue converts a flag string to the param's typed value and validates it.
func (p *Param) ParseValue(s string) (any, error) {
	switch p.Type {
	case TypeInt:
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--%s: %q is not an integer", p.FlagName(), s)
		}
		return n, p.check(n)
	case TypeBool:
		b, err := strconv.ParseBool(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("--%s: %q is not true or false", p.FlagName(), s)
		}
		return b, nil
	case TypeList:
		var items []string
		for _, it := range strings.Split(s, ",") {
			if it = strings.TrimSpace(it); it != "" {
				items = append(items, it)
			}
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("--%s: no value given (omit the flag to leave it unset)", p.FlagName())
		}
		return items, p.check(items)
	default:
		// An empty value would render as unset and silently widen the query.
		if strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("--%s: empty value (omit the flag to leave it unset)", p.FlagName())
		}
		return s, p.check(s)
	}
}

// check validates a typed value against min/max, enum values and pattern.
func (p *Param) check(v any) error {
	switch x := v.(type) {
	case int64:
		if p.Min != nil && x < *p.Min {
			return fmt.Errorf("--%s: %d is below the minimum %d", p.FlagName(), x, *p.Min)
		}
		if p.Max != nil && x > *p.Max {
			return fmt.Errorf("--%s: %d is above the maximum %d", p.FlagName(), x, *p.Max)
		}
	case string:
		if p.Type == TypeEnum {
			for _, allowed := range p.Values {
				if x == allowed {
					return nil
				}
			}
			return fmt.Errorf("--%s: %q is not one of %s", p.FlagName(), x, strings.Join(p.Values, ", "))
		}
		if p.Pattern != "" {
			re, err := regexp.Compile(p.Pattern)
			if err != nil {
				return fmt.Errorf("param %q: invalid pattern: %w", p.Name, err)
			}
			if !re.MatchString(x) {
				return fmt.Errorf("--%s: %q does not match %s", p.FlagName(), x, p.Pattern)
			}
		}
	case []string:
		for _, it := range x {
			if p.Pattern != "" {
				if err := (&Param{Name: p.Name, Type: TypeString, Pattern: p.Pattern}).check(it); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// DefaultValue decodes the param's default (nil when none).
func (p *Param) DefaultValue() (any, error) {
	if !p.HasDefault() {
		return nil, nil
	}
	switch p.Type {
	case TypeList:
		var items []string
		if p.Default.Kind == yaml.SequenceNode {
			if err := p.Default.Decode(&items); err != nil {
				return nil, fmt.Errorf("param %q: default: %w", p.Name, err)
			}
			return items, p.check(items)
		}
		return p.ParseValue(p.Default.Value)
	default:
		if p.Default.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("param %q: default must be a scalar", p.Name)
		}
		v, err := p.ParseValue(p.Default.Value)
		if err != nil {
			return nil, fmt.Errorf("param %q: default: %w", p.Name, err)
		}
		return v, nil
	}
}

// placeholder is a stand-in for a required param in a rendering nobody runs.
func (p *Param) placeholder() any {
	switch p.Type {
	case TypeInt:
		if p.Min != nil {
			return *p.Min
		}
		return int64(1)
	case TypeBool:
		return false
	case TypeEnum:
		if len(p.Values) > 0 {
			return p.Values[0]
		}
	case TypeList:
		return []string{"<" + p.Name + ">"}
	}
	return "<" + p.Name + ">"
}

func (p *Param) templateValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case int64:
		return intLit(x)
	case bool:
		return boolLit(x)
	case []string:
		if len(x) == 0 {
			return nil
		}
		l := make(listLit, len(x))
		for i, s := range x {
			l[i] = strLit(s)
		}
		return l
	case string:
		if p.Type == TypeEnum && p.Render == "identifier" {
			return identLit(x)
		}
		return strLit(x)
	}
	return v
}

// Input is one invocation's arguments.
type Input struct {
	// Params holds the params the caller set; unset ones take their default.
	Params map[string]any
	// Scope maps dimension name to values; key=value dimensions use "key=value".
	Scope map[string][]string
	// Window is the resolved window; nil for `timeframe: none`.
	Window *Window
	// Placeholders stands in for unset required params (describe, verify) instead of failing.
	Placeholders bool
}

// Rendered is a recipe ready to run.
type Rendered struct {
	DQL string
	// Params are the effective params (set or defaulted), raw-typed.
	Params map[string]any
	// Scope echoes the applied scope dimensions (nil when none).
	Scope map[string][]string
	// Window is the default timeframe to send; nil when none or inline.
	Window *Window
	// InlineWindow is the window an inline recipe wrote into its DQL.
	InlineWindow *Window
}

// Render validates the input and renders the recipe's DQL.
func (b *Book) Render(r *Recipe, in Input) (*Rendered, error) {
	set := in.Params
	if in.Placeholders {
		set = map[string]any{}
		for k, v := range in.Params {
			set[k] = v
		}
		for _, p := range r.Spec.Params {
			if _, ok := set[p.Name]; !ok && p.Required && !p.HasDefault() {
				set[p.Name] = p.placeholder()
			}
		}
	}
	params, err := r.effectiveParams(set, in.Placeholders)
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	for _, p := range r.Spec.Params {
		data[p.Name] = p.templateValue(params[p.Name])
	}
	scope, err := b.ScopeData(r, in.Scope)
	if err != nil {
		return nil, err
	}
	if len(r.Spec.Scope) > 0 {
		data[fieldScope] = scope
	}
	out := &Rendered{Params: params}
	if len(in.Scope) > 0 {
		out.Scope = in.Scope
	}
	if r.Spec.Timeframe.Inline {
		if in.Window == nil {
			return nil, fmt.Errorf("recipe %q writes its window inline but none was resolved", r.Name())
		}
		data[fieldWindow] = map[string]any{
			"from": timestampExpr(in.Window.From),
			"to":   timestampExpr(in.Window.To),
			// minutes is the window length for a rate's denominator.
			"minutes": intLit(windowMinutes(in.Window)),
		}
		out.InlineWindow = in.Window
	} else {
		out.Window = in.Window
	}

	tmpl, err := b.template(r)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("recipe %q: %w", r.Name(), err)
	}
	dql := strings.TrimSpace(buf.String())
	if strings.Contains(dql, "<no value>") {
		return nil, fmt.Errorf("recipe %q rendered an unset value; guard optional params with {{if .name}}", r.Name())
	}
	out.DQL = dql
	return out, nil
}

// EffectiveParams applies defaults and checks required params.
func (r *Recipe) EffectiveParams(set map[string]any) (map[string]any, error) {
	return r.effectiveParams(set, false)
}

func (r *Recipe) effectiveParams(set map[string]any, placeholders bool) (map[string]any, error) {
	out := map[string]any{}
	for name := range set {
		if r.Spec.Params.Get(name) == nil {
			return nil, fmt.Errorf("recipe %q has no param %q", r.Name(), name)
		}
	}
	for _, p := range r.Spec.Params {
		if v, ok := set[p.Name]; ok && v != nil {
			if placeholders && p.Required && !p.HasDefault() {
				out[p.Name] = v // a stand-in need not pass the pattern
				continue
			}
			if err := p.check(v); err != nil {
				return nil, err
			}
			out[p.Name] = v
			continue
		}
		dv, err := p.DefaultValue()
		if err != nil {
			return nil, err
		}
		if dv == nil && p.Required {
			if p.Positional {
				return nil, fmt.Errorf("missing required argument <%s> (or --%s)", p.Name, p.FlagName())
			}
			return nil, fmt.Errorf("missing required flag --%s", p.FlagName())
		}
		out[p.Name] = dv
	}
	return out, nil
}

// ScopeData builds the .scope template data {stage, expr}; empty when no dimension is set.
func (b *Book) ScopeData(r *Recipe, set map[string][]string) (map[string]any, error) {
	allowed := map[string]bool{}
	for _, d := range r.Spec.Scope {
		allowed[d] = true
	}
	names := make([]string, 0, len(set))
	for name, vals := range set {
		if len(vals) == 0 {
			continue
		}
		if !allowed[name] {
			return nil, fmt.Errorf("recipe %q does not take --%s", r.Name(), strings.ReplaceAll(name, "_", "-"))
		}
		names = append(names, name)
	}
	// Order by the recipe's declared order so the DQL is stable.
	sort.SliceStable(names, func(i, j int) bool { return indexOf(r.Spec.Scope, names[i]) < indexOf(r.Spec.Scope, names[j]) })

	var preds []string
	for _, name := range names {
		dim := b.Scopes[name]
		if dim == nil {
			return nil, fmt.Errorf("unknown scope dimension %q", name)
		}
		if dim.KeyValue() {
			byKey := map[string][]string{}
			var keys []string
			for _, kv := range set[name] {
				k, v, ok := strings.Cut(kv, "=")
				k = strings.TrimSpace(k)
				if !ok || k == "" {
					return nil, fmt.Errorf("--%s: %q must be key=value", dim.FlagName(), kv)
				}
				if strings.ContainsAny(k, "`\n\r\t ") {
					return nil, fmt.Errorf("--%s: invalid key %q", dim.FlagName(), k)
				}
				if _, seen := byKey[k]; !seen {
					keys = append(keys, k)
				}
				byKey[k] = append(byKey[k], v)
			}
			for _, k := range keys {
				field := strings.ReplaceAll(dim.FieldPattern, "{key}", k)
				preds = append(preds, predicate(QuoteField(field), byKey[k]))
			}
			continue
		}
		preds = append(preds, predicate(QuoteField(dim.Field), set[name]))
	}
	expr := strings.Join(preds, " and ")
	stage := ""
	if expr != "" {
		stage = "| filter " + expr
	}
	return map[string]any{"stage": stage, "expr": expr}, nil
}

// predicate matches any of values, always via in(): some scope fields are arrays, where == never matches.
func predicate(field string, values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = QuoteString(v)
	}
	return "in(" + field + ", {" + strings.Join(quoted, ", ") + "})"
}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return len(list)
}

var templateFuncs = template.FuncMap{"timeAdd": timeAdd}

// template parses the recipe together with the fragments visible to it.
func (b *Book) template(r *Recipe) (*template.Template, error) {
	t := template.New(r.Name()).Option("missingkey=error").Funcs(templateFuncs)
	for _, f := range b.fragmentSources(r) {
		if _, err := t.New("").Parse(f); err != nil {
			return nil, fmt.Errorf("fragments: %w", err)
		}
	}
	if _, err := t.Parse(r.Spec.DQL); err != nil {
		return nil, fmt.Errorf("recipe %q: dql: %w", r.Name(), err)
	}
	return t, nil
}

func (b *Book) fragmentSources(r *Recipe) []string {
	out := make([]string, 0, len(b.fragments)+len(r.fragments))
	for _, f := range b.fragments {
		out = append(out, f.text)
	}
	return append(out, r.fragments...)
}

// templateRefs returns the top-level fields the named template references,
// and the fragments it invokes, via the parse tree.
func templateRefs(t *template.Template, root string) (fields, invoked map[string]bool) {
	fields, invoked = map[string]bool{}, map[string]bool{}
	walked, followed := map[string]bool{}, map[string]bool{}
	var walk, uses func(n parse.Node)
	visit := func(name string) {
		if tt := t.Lookup(name); tt != nil && tt.Tree != nil {
			walk(tt.Tree.Root)
		}
	}
	walk = func(n parse.Node) {
		switch x := n.(type) {
		case *parse.ListNode:
			if x == nil {
				return
			}
			for _, c := range x.Nodes {
				walk(c)
			}
		case *parse.ActionNode:
			walk(x.Pipe)
		case *parse.PipeNode:
			if x == nil {
				return
			}
			for _, c := range x.Cmds {
				walk(c)
			}
		case *parse.CommandNode:
			for _, a := range x.Args {
				walk(a)
			}
		case *parse.FieldNode:
			fields[x.Ident[0]] = true
		case *parse.ChainNode:
			walk(x.Node)
		case *parse.IfNode:
			walk(x.Pipe)
			walk(x.List)
			walk(x.ElseList)
		case *parse.RangeNode:
			walk(x.Pipe)
			walk(x.List)
			walk(x.ElseList)
		case *parse.WithNode:
			// Inside {{with}} "." is rebound: only the pipe references root fields.
			walk(x.Pipe)
			walk(x.ElseList)
			uses(x.List)
		case *parse.TemplateNode:
			walk(x.Pipe)
			invoked[x.Name] = true
			if !walked[x.Name] {
				walked[x.Name] = true
				visit(x.Name)
			}
		}
	}
	// uses records the fragments invoked under n, transitively, without their field references.
	uses = func(n parse.Node) {
		switch x := n.(type) {
		case *parse.ListNode:
			if x == nil {
				return
			}
			for _, c := range x.Nodes {
				uses(c)
			}
		case *parse.IfNode:
			uses(x.List)
			uses(x.ElseList)
		case *parse.RangeNode:
			uses(x.List)
			uses(x.ElseList)
		case *parse.WithNode:
			uses(x.List)
			uses(x.ElseList)
		case *parse.TemplateNode:
			invoked[x.Name] = true
			if !followed[x.Name] {
				followed[x.Name] = true
				if tt := t.Lookup(x.Name); tt != nil && tt.Tree != nil {
					uses(tt.Tree.Root)
				}
			}
		}
	}
	visit(root)
	return fields, invoked
}

// windowMinutes is the window length in whole minutes, at least 1 (a rate divisor).
func windowMinutes(w *Window) int64 {
	m := int64(math.Round(w.To.Sub(w.From).Minutes()))
	if m < 1 {
		return 1
	}
	return m
}
