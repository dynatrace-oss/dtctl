package dqlcost

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// RewriteDocument walks a YAML dashboard/notebook document, rewrites every
// `query:` string using Rewrite+DefaultRewriteOptions, and returns the new
// bytes. Returns `changed=true` when at least one query was modified.
// Preserves YAML formatting and comments because it operates on the node
// tree rather than unmarshalling to `any`.
//
// For JSON input, callers should pre-convert to YAML or accept the
// formatting roundtrip.
func RewriteDocument(data []byte) ([]byte, bool, []Change, error) {
	return RewriteDocumentWith(data, DefaultRewriteOptions())
}

// RewriteDocumentWith accepts explicit rewrite options.
func RewriteDocumentWith(data []byte, opts RewriteOptions) ([]byte, bool, []Change, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return data, false, nil, fmt.Errorf("parse yaml: %w", err)
	}
	var allChanges []Change
	changed := false
	walkNode(&root, opts, &changed, &allChanges)
	if !changed {
		return data, false, nil, nil
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return data, false, nil, fmt.Errorf("encode yaml: %w", err)
	}
	_ = enc.Close()
	return buf.Bytes(), true, allChanges, nil
}

// walkNode recurses through the node tree, rewriting string values whose
// mapping key is "query".
func walkNode(n *yaml.Node, opts RewriteOptions, changed *bool, allChanges *[]Change) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			walkNode(c, opts, changed, allChanges)
		}
	case yaml.MappingNode:
		// Mapping content alternates key, value, key, value, ...
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			v := n.Content[i+1]
			if k.Kind == yaml.ScalarNode && k.Value == "query" && v.Kind == yaml.ScalarNode && v.Value != "" {
				rewritten, changes := Rewrite(v.Value, opts)
				if len(changes) > 0 {
					v.Value = rewritten
					// Keep original style (literal/folded) so multiline queries
					// do not collapse to a flow scalar on re-emit.
					*changed = true
					*allChanges = append(*allChanges, changes...)
				}
				continue
			}
			walkNode(v, opts, changed, allChanges)
		}
	}
}
