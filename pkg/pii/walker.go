package pii

import (
	"fmt"
	"os"
)

// walkRecord recursively walks a record map, detecting and replacing PII values.
// It modifies the map in place. Top-level keys in DQL results are flat dotted
// strings (e.g., "usr.name"), so the key itself is the full path.
func (r *Redactor) walkRecord(record map[string]interface{}) {
	for key, val := range record {
		record[key] = r.walkValue(key, key, val)
	}
}

// walkValue processes a single value, returning the (possibly redacted) replacement.
// fullPath is the dot-separated path from the record root (for custom rule + pattern matching).
// fieldName is the immediate key name (used for array recursion context).
func (r *Redactor) walkValue(fullPath, fieldName string, val interface{}) interface{} {
	switch v := val.(type) {
	case string:
		return r.redactString(fullPath, v)

	case map[string]interface{}:
		// Recurse into nested objects, extending the full path
		for k, nested := range v {
			childPath := fullPath + "." + k
			v[k] = r.walkValue(childPath, k, nested)
		}
		return v

	case []interface{}:
		// Recurse into arrays
		for i, elem := range v {
			v[i] = r.walkValue(fullPath, fieldName, elem)
		}
		return v

	default:
		// Non-string primitives (numbers, bools, nil) are never PII
		return val
	}
}

// redactString checks a string value against custom rules, built-in patterns,
// and returns the redacted replacement if PII is detected, or the original value.
// fullPath is the dot-separated path from the record root (e.g., "data.customerRef").
func (r *Redactor) redactString(fullPath, value string) string {
	// Phase 0: Check custom rules (highest priority — checked on full path)
	if rule := MatchCustomRule(r.customRules, fullPath); rule != nil {
		if rule.Skip {
			return value // explicitly excluded from redaction
		}
		return r.replace(rule.Category, value)
	}

	// Phase 1: Check field name against built-in key-name patterns.
	// matchFieldName checks usr.* prefix on the full path, then extracts
	// the last dot-segment for regex matching.
	if category := matchFieldName(r.patterns, fullPath); category != "" {
		return r.replace(category, value)
	}

	// Phase 2: Check value against value-based patterns (email regex, IP regex)
	if category := matchValue(r.patterns, value); category != "" {
		return r.replace(category, value)
	}

	// Phase 3 candidate collection: if in full mode with Presidio,
	// collect free-text values for NER analysis (handled separately in applyPresidioNER)
	// We don't collect here — Presidio analysis is done in a second pass.

	return value
}

// freeTextField represents a free-text field location for Presidio NER analysis.
type freeTextField struct {
	recordIdx int
	fieldPath string // dot-separated path to the field
	value     string
}

// applyPresidioNER runs Presidio NER analysis on free-text fields in the records.
// It modifies the records in place, replacing detected entities with pseudonyms.
func (r *Redactor) applyPresidioNER(records []map[string]interface{}) {
	if r.presidio == nil {
		return
	}

	// Collect free-text fields from all records
	var fields []freeTextField
	for i, record := range records {
		collectFreeTextFields(record, "", i, &fields)
	}

	if len(fields) == 0 {
		return
	}

	// Extract text values for batch analysis
	texts := make([]string, len(fields))
	for i, f := range fields {
		texts[i] = f.value
	}

	// Call Presidio for NER analysis
	results, err := r.presidio.AnalyzeBatch(texts)
	if err != nil {
		// Presidio failure is non-fatal — we already did regex-based detection
		warnf("Warning: Presidio NER analysis failed: %v\n", err)
		return
	}

	// Apply NER results: replace detected entities in the original records
	for i, entities := range results {
		if len(entities) == 0 {
			continue
		}
		field := fields[i]
		redacted := applyEntities(field.value, entities, r)
		setNestedField(records[field.recordIdx], field.fieldPath, redacted)
	}
}

// applyEntities replaces detected entity spans in a text value.
// Processes entities right-to-left to preserve offset positions.
func applyEntities(text string, entities []PresidioEntity, r *Redactor) string {
	// Sort entities by start position descending (right-to-left replacement)
	sortEntitiesDesc(entities)

	result := text
	for _, e := range entities {
		if e.Start < 0 || e.End > len(result) || e.Start >= e.End {
			continue
		}
		original := result[e.Start:e.End]
		replacement := r.replace(e.EntityType, original)
		result = result[:e.Start] + replacement + result[e.End:]
	}
	return result
}

// sortEntitiesDesc sorts entities by Start position in descending order.
func sortEntitiesDesc(entities []PresidioEntity) {
	// Simple insertion sort (typically very few entities per text)
	for i := 1; i < len(entities); i++ {
		for j := i; j > 0 && entities[j].Start > entities[j-1].Start; j-- {
			entities[j], entities[j-1] = entities[j-1], entities[j]
		}
	}
}

// collectFreeTextFields recursively collects string values that look like free text.
func collectFreeTextFields(obj map[string]interface{}, pathPrefix string, recordIdx int, out *[]freeTextField) {
	for key, val := range obj {
		path := key
		if pathPrefix != "" {
			path = pathPrefix + "." + key
		}

		switch v := val.(type) {
		case string:
			// Only collect if it wasn't already redacted by pattern matching
			// and looks like free text
			if isLikelyFreeText(v) && !isRedacted(v) {
				*out = append(*out, freeTextField{
					recordIdx: recordIdx,
					fieldPath: path,
					value:     v,
				})
			}
		case map[string]interface{}:
			collectFreeTextFields(v, path, recordIdx, out)
		case []interface{}:
			for i, elem := range v {
				if m, ok := elem.(map[string]interface{}); ok {
					elemPath := fmt.Sprintf("%s[%d]", path, i)
					collectFreeTextFields(m, elemPath, recordIdx, out)
				}
			}
		}
	}
}

// setNestedField sets a value at a dot-separated path in a nested map.
func setNestedField(record map[string]interface{}, path string, value interface{}) {
	parts := splitFieldPath(path)
	if len(parts) == 0 {
		return
	}

	current := record
	for i := 0; i < len(parts)-1; i++ {
		next, ok := current[parts[i]]
		if !ok {
			return
		}
		m, ok := next.(map[string]interface{})
		if !ok {
			return
		}
		current = m
	}
	current[parts[len(parts)-1]] = value
}

// splitFieldPath splits a dot-separated field path into components.
// Array indices like "items[0]" are treated as single components.
func splitFieldPath(path string) []string {
	if path == "" {
		return nil
	}

	var parts []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			if i > start {
				parts = append(parts, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		parts = append(parts, path[start:])
	}
	return parts
}

// isRedacted checks if a string value has already been redacted.
func isRedacted(s string) bool {
	if len(s) < 3 {
		return false
	}
	// Lite mode: [CATEGORY]
	if s[0] == '[' && s[len(s)-1] == ']' {
		return true
	}
	// Full mode: <CATEGORY_N>
	if s[0] == '<' && s[len(s)-1] == '>' {
		return true
	}
	return false
}

// warnf prints a warning message to stderr. It can be overridden in tests.
var warnf = func(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}
