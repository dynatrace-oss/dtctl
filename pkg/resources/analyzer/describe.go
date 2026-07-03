package analyzer

import "sort"

// AnalyzerDescription is the enriched view returned by "dtctl describe analyzer".
// Unlike the raw AnalyzerDefinition (which carries the analyzer's internal
// input/output definition format), this bundles the resolved JSON Schemas so
// that agents and humans can see what inputs an analyzer requires without a
// second call. It is emitted in all output formats — a deliberate departure
// from the "describe == get in JSON mode" convention, because the schemas are
// the whole point of the command.
type AnalyzerDescription struct {
	Name         string                 `json:"name" table:"NAME"`
	DisplayName  string                 `json:"displayName" table:"DISPLAY NAME"`
	Description  string                 `json:"description,omitempty" table:"DESCRIPTION"`
	Category     string                 `json:"category,omitempty" table:"CATEGORY"`
	Type         string                 `json:"type,omitempty" table:"TYPE"`
	Labels       []string               `json:"labels,omitempty" table:"-"`
	InputSchema  map[string]interface{} `json:"inputSchema,omitempty" table:"-"`
	ResultSchema map[string]interface{} `json:"resultSchema,omitempty" table:"-"`
}

// SchemaField is a single flattened property from a JSON Schema.
type SchemaField struct {
	Name        string
	Type        string
	Required    bool
	Description string
	// Composite is true when the property uses oneOf/anyOf/allOf/$ref and cannot
	// be represented as a simple typed field — callers should point the user at
	// the full schema (-o json) or the documentation (--doc).
	Composite bool
}

// FlattenSchema turns a top-level JSON Schema object into ordered fields for
// human display. It deliberately handles only the common case: top-level
// "properties" plus the "required" array. Required fields sort first, then
// alphabetically. The second return value (introspectable) is false when the
// schema has no top-level "properties" map, signalling that the caller should
// fall back to "-o json"/"--doc" rather than print an empty table.
func FlattenSchema(schema map[string]interface{}) (fields []SchemaField, introspectable bool) {
	if schema == nil {
		return nil, false
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return nil, false
	}

	required := map[string]bool{}
	if reqList, ok := schema["required"].([]interface{}); ok {
		for _, r := range reqList {
			if name, ok := r.(string); ok {
				required[name] = true
			}
		}
	}

	for name, raw := range props {
		prop, _ := raw.(map[string]interface{})
		field := SchemaField{
			Name:     name,
			Required: required[name],
		}
		if prop != nil {
			field.Type = schemaTypeString(prop)
			if desc, ok := prop["description"].(string); ok {
				field.Description = desc
			}
			field.Composite = isCompositeSchema(prop)
		}
		if field.Type == "" {
			field.Type = "-"
		}
		fields = append(fields, field)
	}

	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Required != fields[j].Required {
			return fields[i].Required // required first
		}
		return fields[i].Name < fields[j].Name
	})

	return fields, true
}

// isCompositeSchema reports whether a property schema uses a composition keyword
// or a $ref, which the simple flattener does not attempt to resolve.
func isCompositeSchema(prop map[string]interface{}) bool {
	for _, key := range []string{"oneOf", "anyOf", "allOf", "$ref"} {
		if _, ok := prop[key]; ok {
			return true
		}
	}
	return false
}

// schemaTypeString renders the "type" of a JSON Schema property. Composite
// properties render as "composite"; a plain string type is used verbatim; a
// list of types (e.g. ["string","null"]) is joined with "|".
func schemaTypeString(prop map[string]interface{}) string {
	if isCompositeSchema(prop) {
		return "composite"
	}
	switch t := prop["type"].(type) {
	case string:
		return t
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, v := range t {
			if s, ok := v.(string); ok {
				parts = append(parts, s)
			}
		}
		return joinNonEmpty(parts, "|")
	default:
		return ""
	}
}

func joinNonEmpty(parts []string, sep string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += p
	}
	return out
}
