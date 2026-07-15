package session

import (
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Round-trip preservation of unknown YAML fields (docs/dev/CONFIG_CONTRACT.md).
//
// A config file may carry fields this build does not know about — written by
// a newer dtctl, or by another schema-v1 writer. Loading is tolerant (yaml.v3
// ignores unknown keys), and saving must not destroy them: SaveTo grafts
// unknown keys from the file being overwritten back into the freshly
// marshaled document.
//
// Only keys unknown to the schema are preserved. Known keys are fully owned
// by this build: a key the marshal omitted (an emptied omitempty field, a
// deleted context) stays gone — grafting it back would resurrect state the
// user removed. Which keys are "known" is derived from the struct yaml tags
// via reflection, so the preservation logic never drifts from the schema.

// preserveUnknownFields merges unknown keys from the existing config file
// into the freshly marshaled document and returns the result. It is
// best-effort by design: if the existing file does not parse, or the merged
// document fails to re-marshal, or the merged document does not parse back
// (grafting an alias node without its anchor produces a document that
// marshals fine but fails on the next load), the fresh marshal is returned
// unchanged — a save must never fail or corrupt the file for the sake of
// preservation.
func preserveUnknownFields(existing, fresh []byte) []byte {
	var oldDoc, newDoc yaml.Node
	if err := yaml.Unmarshal(existing, &oldDoc); err != nil {
		return fresh
	}
	if err := yaml.Unmarshal(fresh, &newDoc); err != nil {
		return fresh
	}
	oldMap := mappingRoot(&oldDoc)
	newMap := mappingRoot(&newDoc)
	if oldMap == nil || newMap == nil {
		return fresh
	}

	graftUnknown(oldMap, newMap, reflect.TypeOf(Config{}))

	merged, err := yaml.Marshal(newMap)
	if err != nil {
		return fresh
	}
	// Grafting can detach an alias from its anchor when the anchor sits on a
	// known field the fresh marshal rewrote: yaml.Marshal still succeeds, but
	// the written file would fail every subsequent load. Only accept the
	// merged document if it parses back.
	var check yaml.Node
	if err := yaml.Unmarshal(merged, &check); err != nil {
		return fresh
	}
	return merged
}

// mappingRoot returns the top-level mapping node of a parsed document.
func mappingRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	if root := doc.Content[0]; root.Kind == yaml.MappingNode {
		return root
	}
	return nil
}

// graftUnknown walks two mapping nodes describing the same struct type,
// appends old keys the struct does not declare into the new mapping, and
// recurses into known struct-valued (and name-keyed sequence) fields.
func graftUnknown(oldMap, newMap *yaml.Node, t reflect.Type) {
	fields := yamlFields(t)
	for i := 0; i+1 < len(oldMap.Content); i += 2 {
		key, oldVal := oldMap.Content[i], oldMap.Content[i+1]

		field, known := fields[key.Value]
		if !known {
			if findMapValue(newMap, key.Value) == nil {
				newMap.Content = append(newMap.Content, key, oldVal)
			}
			continue
		}

		newVal := findMapValue(newMap, key.Value)
		if newVal == nil {
			continue // known key omitted on purpose — new document wins
		}

		ft := field.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		switch {
		case ft.Kind() == reflect.Struct &&
			oldVal.Kind == yaml.MappingNode && newVal.Kind == yaml.MappingNode:
			graftUnknown(oldVal, newVal, ft)
		case ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.Struct &&
			oldVal.Kind == yaml.SequenceNode && newVal.Kind == yaml.SequenceNode:
			graftUnknownByName(oldVal, newVal, ft.Elem())
		}
	}
}

// graftUnknownByName pairs the elements of two sequences of name-keyed
// structs (contexts, tokens) by their "name" value and grafts unknown keys
// element-wise. Elements present only in the new sequence (added) or only in
// the old one (deleted) are left alone — the new document owns membership.
func graftUnknownByName(oldSeq, newSeq *yaml.Node, elem reflect.Type) {
	if _, hasName := yamlFields(elem)["name"]; !hasName {
		return
	}
	for _, newEl := range newSeq.Content {
		if newEl.Kind != yaml.MappingNode {
			continue
		}
		name := findMapValue(newEl, "name")
		if name == nil {
			continue
		}
		for _, oldEl := range oldSeq.Content {
			if oldEl.Kind != yaml.MappingNode {
				continue
			}
			if oldName := findMapValue(oldEl, "name"); oldName != nil && oldName.Value == name.Value {
				graftUnknown(oldEl, newEl, elem)
				break
			}
		}
	}
}

// findMapValue returns the value node for key in a mapping node, or nil.
func findMapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// yamlFields maps the yaml key of every marshaled field of a struct type to
// its StructField, mirroring yaml.v3's naming rules (explicit tag, else the
// lowercased field name; "-" and unexported fields are not marshaled).
func yamlFields(t reflect.Type) map[string]reflect.StructField {
	fields := make(map[string]reflect.StructField, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue // unexported
		}
		tag := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if tag == "-" {
			continue
		}
		if tag == "" {
			tag = strings.ToLower(f.Name)
		}
		fields[tag] = f
	}
	return fields
}
