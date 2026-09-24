package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/olekukonko/tablewriter"
	toon "github.com/toon-format/toon-go"
	"gopkg.in/yaml.v3"
)

// ShapeOptions configures the list shaping applied by NewShapingPrinter:
// the `--limit` and `--fields` flags of the list verbs.
type ShapeOptions struct {
	// Limit keeps only the first Limit items of a list (0 = all).
	Limit int
	// Fields projects every item onto these paths, in this order. A path is a
	// top-level key or a dotted path into nested objects
	// ("modificationInfo.lastModifiedTime").
	Fields []string
	// Tabular selects the flat representation for table/wide/csv/toon: each
	// field becomes one column named by its dotted path, and a field that
	// resolves to an object is expanded into one column per leaf. Structured
	// formats (json/yaml) keep the projected fields nested instead, so a path
	// that works on the full object works on the projection.
	Tabular bool
	// Format is the output format being rendered; --fields is rejected for
	// formats that cannot render a projection (see SupportsFieldProjection).
	Format string
	// Notices receives the truncation notice and unknown-field warnings when
	// the wrapped printer is not an agent envelope (typically stderr). Nil
	// discards them.
	Notices io.Writer
}

// ShapingPrinter applies ShapeOptions to whatever it prints, then delegates to
// the wrapped printer. Agent-mode notices go into the envelope context instead
// of Notices.
type ShapingPrinter struct {
	inner Printer
	opts  ShapeOptions
}

// NewShapingPrinter wraps inner with the list shaping described by opts.
func NewShapingPrinter(inner Printer, opts ShapeOptions) Printer {
	return &ShapingPrinter{inner: inner, opts: opts}
}

// Unwrap returns the wrapped printer, so callers that need the concrete
// printer (e.g. the agent envelope) can still reach it.
func (p *ShapingPrinter) Unwrap() Printer {
	return p.inner
}

// Print shapes obj and prints it through the wrapped printer's Print.
func (p *ShapingPrinter) Print(obj interface{}) error {
	shaped, err := p.shape(obj)
	if err != nil {
		return err
	}
	return p.inner.Print(shaped)
}

// PrintList shapes obj and prints it through the wrapped printer's PrintList.
func (p *ShapingPrinter) PrintList(obj interface{}) error {
	shaped, err := p.shape(obj)
	if err != nil {
		return err
	}
	return p.inner.PrintList(shaped)
}

func (p *ShapingPrinter) shape(obj interface{}) (interface{}, error) {
	if p.opts.Limit < 0 {
		return nil, fmt.Errorf("--limit must be >= 0, got %d", p.opts.Limit)
	}

	if len(p.opts.Fields) > 0 && !SupportsFieldProjection(p.opts.Format) {
		return nil, fmt.Errorf("--fields is not supported with -o %s (use json, yaml, jsonl, csv, toon, table or wide)", p.opts.Format)
	}

	obj = p.applyLimit(obj)
	if len(p.opts.Fields) == 0 {
		return obj, nil
	}
	return p.project(obj)
}

// applyLimit truncates a slice to opts.Limit, keeping its element type so a
// --limit-only invocation prints exactly what the unlimited one would have
// printed for those items.
func (p *ShapingPrinter) applyLimit(obj interface{}) interface{} {
	if p.opts.Limit == 0 {
		return obj
	}
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Pointer && !v.IsNil() && v.Elem().Kind() == reflect.Slice {
		v = v.Elem()
	}
	if v.Kind() != reflect.Slice || v.Len() <= p.opts.Limit {
		return obj
	}

	total := v.Len()
	p.notifyTruncated(p.opts.Limit, total)
	return v.Slice(0, p.opts.Limit).Interface()
}

func (p *ShapingPrinter) notifyTruncated(shown, total int) {
	if ap := AsAgentPrinter(p.inner); ap != nil {
		// A command may already have reported a larger server-side total;
		// never shrink it to the count that was fetched.
		if ap.ctx.Total == nil || *ap.ctx.Total < total {
			ap.SetTotal(total)
		}
		ap.SetHasMore(true)
		ap.ctx.Suggestions = append(ap.ctx.Suggestions, fmt.Sprintf(
			"Showing %d of %d items; --limit 0 returns all", shown, total))
		return
	}
	if p.opts.Notices != nil {
		FprintHint(p.opts.Notices, "Showing %d of %d items (--limit %d); use --limit 0 for all", shown, total, shown)
	}
}

func (p *ShapingPrinter) warn(msg string) {
	if ap := AsAgentPrinter(p.inner); ap != nil {
		ap.addWarning(msg)
		return
	}
	if p.opts.Notices != nil {
		FprintWarning(p.opts.Notices, "%s", msg)
	}
}

// AsAgentPrinter returns the agent envelope printer behind pr — pr itself or
// the printer a wrapper such as ShapingPrinter delegates to — or nil.
func AsAgentPrinter(pr Printer) *AgentPrinter {
	for {
		switch v := pr.(type) {
		case *AgentPrinter:
			return v
		case interface{ Unwrap() Printer }:
			pr = v.Unwrap()
		default:
			return nil
		}
	}
}

// project maps obj (a list or a single object) onto opts.Fields.
func (p *ShapingPrinter) project(obj interface{}) (interface{}, error) {
	generic, err := toGeneric(obj)
	if err != nil {
		return nil, fmt.Errorf("--fields: %w", err)
	}

	var items []interface{}
	single := false
	switch g := generic.(type) {
	case []interface{}:
		items = g
	case nil:
		return obj, nil
	default:
		items = []interface{}{g}
		single = true
	}

	p.warnUnknownFields(items)

	if p.opts.Tabular {
		return p.flatRows(items, single), nil
	}

	objs := make([]*orderedObject, len(items))
	for i, item := range items {
		objs[i] = p.nestedProjection(item)
	}
	if single {
		return objs[0], nil
	}
	return objs, nil
}

// warnUnknownFields reports each requested field that no item carries. A field
// may legitimately be absent from every item (an optional, omitted value), so
// this is a warning rather than an error; a typo is the common cause.
func (p *ShapingPrinter) warnUnknownFields(items []interface{}) {
	if len(items) == 0 {
		return
	}
	for _, f := range p.opts.Fields {
		found := false
		for _, item := range items {
			if _, ok := lookupPath(item, f); ok {
				found = true
				break
			}
		}
		if found {
			continue
		}
		msg := fmt.Sprintf("--fields: %q is not present on any item", f)
		if m, ok := items[0].(map[string]interface{}); ok {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			msg += "; top-level fields: " + strings.Join(keys, ", ")
		}
		p.warn(msg)
	}
}

// lookupPath resolves a dotted path in a generic value. At each level a key
// equal to the whole remaining path wins over splitting it, so keys that
// themselves contain dots stay addressable.
func lookupPath(v interface{}, path string) (interface{}, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, false
	}
	if val, ok := m[path]; ok {
		return val, true
	}
	head, rest, found := strings.Cut(path, ".")
	for found {
		if next, ok := m[head]; ok {
			if val, ok := lookupPath(next, rest); ok {
				return val, true
			}
		}
		var more string
		more, rest, found = strings.Cut(rest, ".")
		head += "." + more
	}
	return nil, false
}

// nestedProjection keeps the requested fields in their original nesting:
// "modificationInfo.lastModifiedTime" becomes
// {"modificationInfo": {"lastModifiedTime": ...}}. Missing fields are omitted.
func (p *ShapingPrinter) nestedProjection(item interface{}) *orderedObject {
	out := newOrderedObject()
	for _, f := range p.opts.Fields {
		val, ok := lookupPath(item, f)
		if !ok {
			continue
		}
		// A literal top-level key (even one containing dots) stays flat.
		if m, isMap := item.(map[string]interface{}); isMap {
			if _, literal := m[f]; literal {
				out.set(f, val)
				continue
			}
		}
		out.setPath(strings.Split(f, "."), val)
	}
	return out
}

// flatRows builds the tabular representation: one column per requested path,
// with object values expanded into one column per leaf.
func (p *ShapingPrinter) flatRows(items []interface{}, single bool) *FlatRows {
	rows := make([]map[string]interface{}, len(items))
	var columns []string
	seen := map[string]bool{}
	for i, item := range items {
		row := map[string]interface{}{}
		for _, f := range p.opts.Fields {
			val, ok := lookupPath(item, f)
			if !ok {
				if !seen[f] {
					seen[f] = true
					columns = append(columns, f)
				}
				continue
			}
			flattenInto(row, f, val, func(col string) {
				if !seen[col] {
					seen[col] = true
					columns = append(columns, col)
				}
			})
		}
		rows[i] = row
	}

	fr := &FlatRows{Columns: columns, single: single}
	for _, row := range rows {
		vals := make([]interface{}, len(columns))
		for j, c := range columns {
			vals[j] = row[c]
		}
		fr.Rows = append(fr.Rows, vals)
	}
	return fr
}

func flattenInto(row map[string]interface{}, prefix string, val interface{}, addColumn func(string)) {
	m, ok := val.(map[string]interface{})
	if !ok || len(m) == 0 {
		addColumn(prefix)
		row[prefix] = val
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		flattenInto(row, prefix+"."+k, m[k], addColumn)
	}
}

// FlatRows is the tabular form of a --fields projection: named columns in a
// fixed order, one row per item. The table, CSV and TOON printers render it
// with its column order intact; any other encoder sees an array of flat
// objects keyed by column name.
type FlatRows struct {
	Columns []string
	Rows    [][]interface{}
	single  bool
}

func (fr *FlatRows) objects() []*orderedObject {
	objs := make([]*orderedObject, len(fr.Rows))
	for i, row := range fr.Rows {
		o := newOrderedObject()
		for j, c := range fr.Columns {
			o.set(c, row[j])
		}
		objs[i] = o
	}
	return objs
}

// MarshalJSON encodes the rows as flat objects (a single object when the
// projection was of one item).
func (fr *FlatRows) MarshalJSON() ([]byte, error) {
	objs := fr.objects()
	if fr.single && len(objs) == 1 {
		return json.Marshal(objs[0])
	}
	return json.Marshal(objs)
}

// toonValue returns the rows as ordered TOON objects, so the encoder emits a
// tabular array whose header lists the columns in the requested order.
func (fr *FlatRows) toonValue() interface{} {
	objs := make([]interface{}, len(fr.Rows))
	for i, row := range fr.Rows {
		fields := make([]toon.Field, len(fr.Columns))
		for j, c := range fr.Columns {
			fields[j] = toon.Field{Key: c, Value: row[j]}
		}
		objs[i] = toon.NewObject(fields...)
	}
	if fr.single && len(objs) == 1 {
		return objs[0]
	}
	return objs
}

// cells returns the rows as display strings using format.
func (fr *FlatRows) cells(format func(interface{}) string) [][]string {
	out := make([][]string, len(fr.Rows))
	for i, row := range fr.Rows {
		out[i] = make([]string, len(row))
		for j, v := range row {
			out[i][j] = format(v)
		}
	}
	return out
}

// toonGeneric is toGeneric for the TOON encoders: values that know their own
// ordered TOON form keep it, everything else round-trips through JSON.
func toonGeneric(v interface{}) (interface{}, error) {
	if fr, ok := v.(*FlatRows); ok {
		return fr.toonValue(), nil
	}
	return toGeneric(v)
}

// orderedObject is a JSON/YAML object that keeps its keys in insertion order.
type orderedObject struct {
	keys []string
	vals map[string]interface{}
}

func newOrderedObject() *orderedObject {
	return &orderedObject{vals: map[string]interface{}{}}
}

func (o *orderedObject) set(key string, val interface{}) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = val
}

func (o *orderedObject) setPath(path []string, val interface{}) {
	if len(path) == 1 {
		o.set(path[0], val)
		return
	}
	child, ok := o.vals[path[0]].(*orderedObject)
	if !ok {
		child = newOrderedObject()
		o.set(path[0], child)
	}
	child.setPath(path[1:], val)
}

// MarshalJSON encodes the object with its keys in insertion order.
func (o *orderedObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := json.Marshal(o.vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// MarshalYAML encodes the object as a mapping with its keys in insertion order.
func (o *orderedObject) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range o.keys {
		var val yaml.Node
		if err := val.Encode(o.vals[k]); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, &val)
	}
	return node, nil
}

// ParseFields splits a --fields value into trimmed, de-duplicated paths.
func ParseFields(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// IsTabularFormat reports whether format renders rows and columns (and so
// wants the flat --fields representation). plain mirrors NewPrinterWithOpts,
// which turns table/wide into JSON in plain mode.
func IsTabularFormat(format string, plain bool) bool {
	switch format {
	case "csv", "toon":
		return true
	case "table", "wide", "":
		return !plain
	}
	return false
}

// SupportsFieldProjection reports whether format can render a --fields
// projection. Charts need the full typed records they were built for.
func SupportsFieldProjection(format string) bool {
	switch format {
	case "", "table", "wide", "json", "yaml", "yml", "jsonl", "csv", "toon":
		return true
	}
	return false
}

// printFlatRows renders a --fields projection with its columns in order.
func (p *TablePrinter) printFlatRows(fr *FlatRows) error {
	if len(fr.Rows) == 0 {
		fmt.Fprintln(p.writer, Colorize(Dim, "No resources found."))
		return nil
	}
	table := tablewriter.NewTable(p.writer, kubectlStyleOptions()...)
	// Headers keep the dots of the requested paths (tw.Title would turn
	// them into spaces), so a column names exactly what --fields accepts.
	headers := make([]string, len(fr.Columns))
	for i, c := range fr.Columns {
		headers[i] = strings.ToUpper(c)
		if ColorEnabled() {
			headers[i] = Colorize(Bold, headers[i])
		}
	}
	table.Header(toAny(headers)...)
	for _, row := range fr.cells(func(v interface{}) string { return colorizeTableValue(formatTableMapValue(v)) }) {
		_ = table.Append(toAny(row)...)
	}
	return table.Render()
}

// printFlatRows renders a --fields projection with its columns in order.
func (p *CSVPrinter) printFlatRows(fr *FlatRows) error {
	if len(fr.Rows) == 0 {
		return nil
	}
	writer := csv.NewWriter(p.writer)
	if err := writer.Write(fr.Columns); err != nil {
		return err
	}
	for _, row := range fr.cells(formatCSVValue) {
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
