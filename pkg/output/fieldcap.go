package output

import "sort"

// DefaultAgentMaxFieldChars is the per-value cap agent mode applies to string
// values in an inline query result (--max-field-chars). A single log `content`,
// stack trace or serialised object can run to many KB, and `| limit N` bounds
// rows, not bytes; 500 runes keeps the head of such a value — usually the part
// that identifies it — while a clipped value still says how much it lost.
const DefaultAgentMaxFieldChars = 500

// ClipRecordValues returns a copy of records in which every string value longer
// than max runes, at any nesting depth, is cut to max runes and ends in the same
// "…(+N chars)" marker the spill summary uses, so a consumer can tell a clipped
// value from a short one and how much is missing. fields names the keys whose
// values were clipped (the nearest enclosing key for a nested value), sorted, so
// the caller can say which columns to re-fetch in full.
//
// The input is never modified: the caller's rows still back a spill file, which
// keeps every value in full. Rows with nothing to clip are shared, not copied.
// max <= 0 disables clipping and returns records as-is.
func ClipRecordValues(records []map[string]interface{}, max int) (out []map[string]interface{}, fields []string) {
	if max <= 0 {
		return records, nil
	}
	seen := map[string]struct{}{}
	out = make([]map[string]interface{}, len(records))
	for i, rec := range records {
		clipped, _ := clipLeaves(rec, max, "", seen)
		out[i], _ = clipped.(map[string]interface{})
	}
	return out, sortedKeys(seen)
}

// ClipValue is ClipRecordValues for a value of arbitrary shape — the output of a
// --jq filter, which need not be a list of records.
func ClipValue(v interface{}, max int) (out interface{}, fields []string) {
	if max <= 0 {
		return v, nil
	}
	seen := map[string]struct{}{}
	out, _ = clipLeaves(v, max, "", seen)
	return out, sortedKeys(seen)
}

// clipLeaves returns v with its long string leaves clipped and whether anything
// changed. Maps and slices are rebuilt only when a descendant changed, so an
// unclipped subtree is returned by reference. key is the nearest enclosing map
// key, recorded in seen for every clipped leaf ("" at the top level or directly
// inside a top-level array is not recorded).
func clipLeaves(v interface{}, max int, key string, seen map[string]struct{}) (interface{}, bool) {
	switch x := v.(type) {
	case string:
		c := clipRunes(x, max)
		if c == x {
			return x, false
		}
		if key != "" {
			seen[key] = struct{}{}
		}
		return c, true
	case map[string]interface{}:
		var cp map[string]interface{}
		for k, child := range x {
			nv, changed := clipLeaves(child, max, k, seen)
			if !changed {
				continue
			}
			if cp == nil {
				cp = make(map[string]interface{}, len(x))
				for kk, vv := range x {
					cp[kk] = vv
				}
			}
			cp[k] = nv
		}
		if cp == nil {
			return x, false
		}
		return cp, true
	case []interface{}:
		var cp []interface{}
		for i, child := range x {
			nv, changed := clipLeaves(child, max, key, seen)
			if !changed {
				continue
			}
			if cp == nil {
				cp = append([]interface{}(nil), x...)
			}
			cp[i] = nv
		}
		if cp == nil {
			return x, false
		}
		return cp, true
	case []map[string]interface{}:
		var cp []map[string]interface{}
		for i, child := range x {
			nv, changed := clipLeaves(child, max, key, seen)
			if !changed {
				continue
			}
			if cp == nil {
				cp = append([]map[string]interface{}(nil), x...)
			}
			cp[i], _ = nv.(map[string]interface{})
		}
		if cp == nil {
			return x, false
		}
		return cp, true
	default:
		return v, false
	}
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
