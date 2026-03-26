package pii

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CustomRule defines a user-specified PII field rule.
// Rules can target exact field paths ("data.customerRef") or use a glob
// suffix ("usr.*") to match all fields under a prefix.
type CustomRule struct {
	// Field is the field path to match. Supports exact match ("data.customerRef")
	// or glob suffix ("usr.*") where * matches any single segment.
	Field string `yaml:"field"`

	// Category is the PII category label (e.g., "PERSON", "EMAIL").
	// Defaults to "REDACTED" if empty.
	Category string `yaml:"category,omitempty"`

	// Skip, when true, excludes the field from PII redaction even if
	// built-in patterns would match it.
	Skip bool `yaml:"skip,omitempty"`
}

// CustomRulesFile is the on-disk format for .dtctl-pii.yaml project files.
type CustomRulesFile struct {
	Version string       `yaml:"version"`
	Rules   []CustomRule `yaml:"rules"`
}

// customRulesFileName is the name of the project-level custom rules file.
const customRulesFileName = ".dtctl-pii.yaml"

// LoadCustomRulesFile reads and parses a custom rules YAML file.
func LoadCustomRulesFile(path string) ([]CustomRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading custom rules file: %w", err)
	}

	var file CustomRulesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing custom rules file %s: %w", path, err)
	}

	// Normalize: default category to "REDACTED"
	for i := range file.Rules {
		if file.Rules[i].Category == "" && !file.Rules[i].Skip {
			file.Rules[i].Category = "REDACTED"
		}
	}

	return file.Rules, nil
}

// FindCustomRulesFile searches upward from the given directory for a
// .dtctl-pii.yaml file, returning its path or empty string if not found.
// This mirrors how .dtctl.yaml is discovered.
func FindCustomRulesFile(startDir string) string {
	dir := startDir
	for {
		candidate := filepath.Join(dir, customRulesFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return ""
		}
		dir = parent
	}
}

// MergeRules combines project-level and config-level custom rules.
// Config rules override project rules when they target the same field.
// The returned slice has project rules first (possibly overridden), then
// any config-only rules appended.
func MergeRules(project, config []CustomRule) []CustomRule {
	if len(project) == 0 {
		return config
	}
	if len(config) == 0 {
		return project
	}

	// Index config rules by field for quick lookup
	configByField := make(map[string]CustomRule, len(config))
	for _, r := range config {
		configByField[r.Field] = r
	}

	// Start with project rules, overriding from config where applicable
	merged := make([]CustomRule, 0, len(project)+len(config))
	seen := make(map[string]bool, len(project))

	for _, pr := range project {
		if cr, ok := configByField[pr.Field]; ok {
			// Config overrides project for this field
			merged = append(merged, cr)
		} else {
			merged = append(merged, pr)
		}
		seen[pr.Field] = true
	}

	// Append config-only rules (not in project)
	for _, cr := range config {
		if !seen[cr.Field] {
			merged = append(merged, cr)
		}
	}

	return merged
}

// MatchCustomRule checks a field name against custom rules.
// Returns the first matching rule, or nil if no match.
// Matching order: exact match first, then glob suffix.
func MatchCustomRule(rules []CustomRule, fieldName string) *CustomRule {
	// First pass: exact match
	for i := range rules {
		if rules[i].Field == fieldName {
			return &rules[i]
		}
	}

	// Second pass: glob suffix match (e.g., "usr.*" matches "usr.name")
	for i := range rules {
		if matchGlob(rules[i].Field, fieldName) {
			return &rules[i]
		}
	}

	return nil
}

// matchGlob checks if a glob pattern matches a field name.
// Only suffix glob is supported: "prefix.*" matches "prefix.anything".
// The * matches exactly one dot-segment (not nested paths).
func matchGlob(pattern, fieldName string) bool {
	if !strings.HasSuffix(pattern, ".*") {
		return false
	}
	prefix := pattern[:len(pattern)-1] // "usr.*" -> "usr."
	if !strings.HasPrefix(fieldName, prefix) {
		return false
	}
	// Ensure * matches exactly one segment (no dots in the remainder)
	remainder := fieldName[len(prefix):]
	return len(remainder) > 0 && !strings.Contains(remainder, ".")
}
