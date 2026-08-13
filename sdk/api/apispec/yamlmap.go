package apispec

import (
	"fmt"
	"sort"
)

// This file holds the accessors for walking a specification parsed into
// map[string]any.
//
// An OpenAPI document is parsed generically rather than into typed structs for
// two reasons: schemas must survive verbatim (they are handed to the caller as
// JSON/YAML, so any lossy struct would silently drop fields), and $ref
// resolution needs the whole document addressable by JSON pointer anyway. Every
// accessor below returns a zero value for a missing or wrongly-typed key, so a
// malformed document degrades field by field instead of failing the parse — the
// fail-soft requirement that the whole package rests on.

// mapAt returns the mapping at key, or nil.
func mapAt(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	sub, _ := m[key].(map[string]any)
	return sub
}

// strAt returns the string at key. Numbers and booleans are rendered rather than
// dropped: `info.version` is frequently written unquoted (`version: 1.54`), which
// YAML types as a float.
func strAt(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch v := m[key].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// boolAt returns the boolean at key, defaulting to false.
func boolAt(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	b, _ := m[key].(bool)
	return b
}

// sliceAt returns the sequence at key, or nil.
func sliceAt(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	s, _ := m[key].([]any)
	return s
}

// toStrings renders a scalar or sequence as a string slice. A single scalar is
// treated as a one-element list, since `security` scope lists are occasionally
// written that way.
func toStrings(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		return []string{t}
	default:
		return nil
	}
}

// sortedKeys returns m's keys in lexical order. Parsing into a map loses
// document order, so every projection sorts to stay deterministic — golden
// output depends on it.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
