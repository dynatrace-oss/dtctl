package api

import (
	"fmt"
	"sort"
	"strings"
)

// SchemaField is one property of a schema, projected one level deep.
//
// A request-body schema is the part of an operation a caller most needs and the
// part that is least readable raw: nested objects, allOf compositions and
// unresolved `$ref`s. One level is enough to answer "what do I put in the body",
// and the full schema is always one `-o yaml` away — the same
// complete-at-a-coarser-grain trade the operation index makes.
type SchemaField struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type,omitempty" yaml:"type,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Nested marks a field whose own shape is not shown here, so a caller knows
	// to look at the full schema rather than assuming a scalar.
	Nested bool `json:"nested,omitempty" yaml:"nested,omitempty"`
}

// TopLevelFields projects an object schema's own properties. The bool reports
// whether the schema was introspectable at all: a schema that is a bare `$ref`,
// an `allOf` composition or an untyped free-form object yields false, which a
// caller should render as "not introspectable" rather than "no fields".
func TopLevelFields(schema any) ([]SchemaField, bool) {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil, false
	}

	props, ok := m["properties"].(map[string]any)
	if !ok {
		return nil, false
	}

	required := map[string]bool{}
	if list, ok := m["required"].([]any); ok {
		for _, r := range list {
			if s, ok := r.(string); ok {
				required[s] = true
			}
		}
	}

	fields := make([]SchemaField, 0, len(props))
	for name, raw := range props {
		f := SchemaField{Name: name, Required: required[name]}
		if pm, ok := raw.(map[string]any); ok {
			f.Type, f.Nested = schemaTypeName(pm)
			if d, ok := pm["description"].(string); ok {
				f.Description = firstLine(d)
			}
		}
		fields = append(fields, f)
	}

	// Required fields first, then alphabetically: composing a call starts with
	// what is mandatory.
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Required != fields[j].Required {
			return fields[i].Required
		}
		return fields[i].Name < fields[j].Name
	})
	return fields, true
}

// schemaTypeName renders a property's type compactly, and reports whether its
// own shape was elided.
func schemaTypeName(prop map[string]any) (name string, nested bool) {
	if ref, ok := prop["$ref"].(string); ok {
		return refName(ref), true
	}

	t, _ := prop["type"].(string)
	switch t {
	case "array":
		items, _ := prop["items"].(map[string]any)
		if items == nil {
			return "array", true
		}
		inner, innerNested := schemaTypeName(items)
		return inner + "[]", innerNested
	case "object":
		return "object", true
	case "":
		// No declared type: an enum, a composition, or free-form.
		if _, ok := prop["enum"]; ok {
			return "enum", false
		}
		return "", true
	}

	if f, ok := prop["format"].(string); ok && f != "" {
		return fmt.Sprintf("%s(%s)", t, f), false
	}
	return t, false
}

// refName reduces "#/components/schemas/Widget" to "Widget".
func refName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 && i < len(ref)-1 {
		return ref[i+1:]
	}
	return ref
}

// firstLine trims a description to its first line: OpenAPI descriptions are
// often multi-paragraph markdown, and a table cell holds one line.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
