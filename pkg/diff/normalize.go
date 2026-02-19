package diff

import (
	"encoding/json"
	"reflect"
	"sort"
)

func normalize(data interface{}, ignoreMetadata, ignoreOrder bool) interface{} {
	normalized := deepCopy(data)

	if ignoreMetadata {
		removeMetadataFields(normalized)
	}

	if ignoreOrder {
		sortArrays(normalized)
	}

	return normalized
}

func deepCopy(data interface{}) interface{} {
	bytes, err := json.Marshal(data)
	if err != nil {
		return data
	}

	var result interface{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return data
	}

	return result
}

func removeMetadataFields(data interface{}) {
	fieldsToRemove := []string{
		"metadata.createdAt",
		"metadata.updatedAt",
		"metadata.version",
		"metadata.modifiedBy",
		"metadata.creationTimestamp",
		"metadata.resourceVersion",
		"metadata.generation",
		"metadata.uid",
	}

	for _, field := range fieldsToRemove {
		removePath(data, field)
	}
}

func removePath(data interface{}, path string) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return
	}

	removePathRecursive(data, parts)
}

func removePathRecursive(data interface{}, parts []string) {
	if len(parts) == 0 {
		return
	}

	m, ok := data.(map[string]interface{})
	if !ok {
		return
	}

	if len(parts) == 1 {
		delete(m, parts[0])
		return
	}

	if next, exists := m[parts[0]]; exists {
		removePathRecursive(next, parts[1:])
	}
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	parts := []string{}
	current := ""
	for _, ch := range path {
		if ch == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func sortArrays(data interface{}) {
	visitArrays(data, func(arr []interface{}) {
		if hasStableKey(arr) {
			sortByKey(arr)
		}
	})
}

func visitArrays(data interface{}, fn func([]interface{})) {
	switch v := data.(type) {
	case map[string]interface{}:
		for _, val := range v {
			visitArrays(val, fn)
		}
	case []interface{}:
		fn(v)
		for _, item := range v {
			visitArrays(item, fn)
		}
	}
}

func hasStableKey(arr []interface{}) bool {
	if len(arr) == 0 {
		return false
	}

	stableKeys := []string{"id", "name", "key"}

	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			return false
		}

		hasKey := false
		for _, key := range stableKeys {
			if _, exists := m[key]; exists {
				hasKey = true
				break
			}
		}

		if !hasKey {
			return false
		}
	}

	return true
}

func sortByKey(arr []interface{}) {
	stableKeys := []string{"id", "name", "key"}

	sort.Slice(arr, func(i, j int) bool {
		mi, oki := arr[i].(map[string]interface{})
		mj, okj := arr[j].(map[string]interface{})

		if !oki || !okj {
			return false
		}

		for _, key := range stableKeys {
			vi, oki := mi[key]
			vj, okj := mj[key]

			if oki && okj {
				return compareValues(vi, vj)
			}
		}

		return false
	})
}

func compareValues(a, b interface{}) bool {
	aStr, aOk := a.(string)
	bStr, bOk := b.(string)

	if aOk && bOk {
		return aStr < bStr
	}

	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)

	if aOk && bOk {
		return aNum < bNum
	}

	return reflect.ValueOf(a).String() < reflect.ValueOf(b).String()
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	default:
		return 0, false
	}
}
