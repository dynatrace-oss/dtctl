package pii

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- MatchCustomRule tests ---

func TestMatchCustomRuleExact(t *testing.T) {
	rules := []CustomRule{
		{Field: "data.customerRef", Category: "PERSON"},
		{Field: "data.action", Skip: true},
	}

	// Exact match
	rule := MatchCustomRule(rules, "data.customerRef")
	require.NotNil(t, rule)
	assert.Equal(t, "PERSON", rule.Category)
	assert.False(t, rule.Skip)

	// Skip rule
	rule = MatchCustomRule(rules, "data.action")
	require.NotNil(t, rule)
	assert.True(t, rule.Skip)

	// No match
	rule = MatchCustomRule(rules, "data.other")
	assert.Nil(t, rule)
}

func TestMatchCustomRuleGlob(t *testing.T) {
	rules := []CustomRule{
		{Field: "usr.*", Category: "PERSON"},
		{Field: "data.nested.*", Category: "REDACTED"},
	}

	// Glob match
	rule := MatchCustomRule(rules, "usr.name")
	require.NotNil(t, rule)
	assert.Equal(t, "PERSON", rule.Category)

	rule = MatchCustomRule(rules, "usr.email")
	require.NotNil(t, rule)

	rule = MatchCustomRule(rules, "data.nested.field1")
	require.NotNil(t, rule)
	assert.Equal(t, "REDACTED", rule.Category)

	// Glob does NOT match nested paths beyond one segment
	rule = MatchCustomRule(rules, "usr.deep.nested")
	assert.Nil(t, rule)

	// Glob does NOT match the prefix itself
	rule = MatchCustomRule(rules, "usr.")
	assert.Nil(t, rule)

	// No match
	rule = MatchCustomRule(rules, "other.field")
	assert.Nil(t, rule)
}

func TestMatchCustomRuleExactPrecedence(t *testing.T) {
	rules := []CustomRule{
		{Field: "usr.email", Category: "EMAIL"}, // exact
		{Field: "usr.*", Category: "PERSON"},    // glob
	}

	// Exact match takes precedence over glob
	rule := MatchCustomRule(rules, "usr.email")
	require.NotNil(t, rule)
	assert.Equal(t, "EMAIL", rule.Category)

	// Other usr.* fields fall through to glob
	rule = MatchCustomRule(rules, "usr.name")
	require.NotNil(t, rule)
	assert.Equal(t, "PERSON", rule.Category)
}

func TestMatchCustomRuleEmpty(t *testing.T) {
	rule := MatchCustomRule(nil, "anything")
	assert.Nil(t, rule)

	rule = MatchCustomRule([]CustomRule{}, "anything")
	assert.Nil(t, rule)
}

// --- matchGlob tests ---

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern   string
		fieldName string
		expected  bool
	}{
		{"usr.*", "usr.name", true},
		{"usr.*", "usr.email", true},
		{"usr.*", "usr.", false},            // empty remainder
		{"usr.*", "usr.deep.nested", false}, // multi-segment
		{"usr.*", "other.name", false},      // wrong prefix
		{"usr.*", "usrname", false},         // no dot
		{"data", "data", false},             // no glob suffix
		{"data.*", "data.x", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.fieldName, func(t *testing.T) {
			assert.Equal(t, tt.expected, matchGlob(tt.pattern, tt.fieldName))
		})
	}
}

// --- MergeRules tests ---

func TestMergeRulesEmpty(t *testing.T) {
	project := []CustomRule{{Field: "a", Category: "X"}}
	config := []CustomRule{{Field: "b", Category: "Y"}}

	// Both empty
	assert.Nil(t, MergeRules(nil, nil))

	// Only project
	result := MergeRules(project, nil)
	assert.Equal(t, project, result)

	// Only config
	result = MergeRules(nil, config)
	assert.Equal(t, config, result)
}

func TestMergeRulesConfigOverridesProject(t *testing.T) {
	project := []CustomRule{
		{Field: "data.customerRef", Category: "PERSON"},
		{Field: "data.action", Category: "REDACTED"},
	}
	config := []CustomRule{
		{Field: "data.customerRef", Category: "ORGANIZATION"}, // override
		{Field: "data.extra", Category: "EMAIL"},              // config-only
	}

	merged := MergeRules(project, config)

	require.Len(t, merged, 3)
	// data.customerRef: config overrides project
	assert.Equal(t, "data.customerRef", merged[0].Field)
	assert.Equal(t, "ORGANIZATION", merged[0].Category)
	// data.action: project only
	assert.Equal(t, "data.action", merged[1].Field)
	assert.Equal(t, "REDACTED", merged[1].Category)
	// data.extra: config only
	assert.Equal(t, "data.extra", merged[2].Field)
	assert.Equal(t, "EMAIL", merged[2].Category)
}

// --- LoadCustomRulesFile tests ---

func TestLoadCustomRulesFile(t *testing.T) {
	content := `version: v1
rules:
  - field: "data.customerRef"
    category: PERSON
  - field: "usr.*"
    category: PERSON
  - field: "data.internalNote"
  - field: "data.action"
    skip: true
`
	dir := t.TempDir()
	path := filepath.Join(dir, ".dtctl-pii.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	rules, err := LoadCustomRulesFile(path)
	require.NoError(t, err)
	require.Len(t, rules, 4)

	assert.Equal(t, "data.customerRef", rules[0].Field)
	assert.Equal(t, "PERSON", rules[0].Category)

	assert.Equal(t, "usr.*", rules[1].Field)
	assert.Equal(t, "PERSON", rules[1].Category)

	// Default category
	assert.Equal(t, "data.internalNote", rules[2].Field)
	assert.Equal(t, "REDACTED", rules[2].Category)

	// Skip rule (no category needed)
	assert.Equal(t, "data.action", rules[3].Field)
	assert.True(t, rules[3].Skip)
	assert.Equal(t, "", rules[3].Category) // skip rules don't get default
}

func TestLoadCustomRulesFileNotFound(t *testing.T) {
	_, err := LoadCustomRulesFile("/nonexistent/.dtctl-pii.yaml")
	assert.Error(t, err)
}

func TestLoadCustomRulesFileInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".dtctl-pii.yaml")
	require.NoError(t, os.WriteFile(path, []byte("{{invalid"), 0644))

	_, err := LoadCustomRulesFile(path)
	assert.Error(t, err)
}

// --- FindCustomRulesFile tests ---

func TestFindCustomRulesFile(t *testing.T) {
	// Create a temp dir tree: root/sub1/sub2
	root := t.TempDir()
	sub1 := filepath.Join(root, "sub1")
	sub2 := filepath.Join(sub1, "sub2")
	require.NoError(t, os.MkdirAll(sub2, 0755))

	// Place rules file in root
	rulesPath := filepath.Join(root, ".dtctl-pii.yaml")
	require.NoError(t, os.WriteFile(rulesPath, []byte("version: v1\nrules: []\n"), 0644))

	// Search from sub2 should find root's file
	found := FindCustomRulesFile(sub2)
	assert.Equal(t, rulesPath, found)

	// Search from sub1 should also find it
	found = FindCustomRulesFile(sub1)
	assert.Equal(t, rulesPath, found)

	// Search from root should find it
	found = FindCustomRulesFile(root)
	assert.Equal(t, rulesPath, found)
}

func TestFindCustomRulesFileNotFound(t *testing.T) {
	dir := t.TempDir()
	found := FindCustomRulesFile(dir)
	assert.Equal(t, "", found)
}

func TestFindCustomRulesFilePreferNearest(t *testing.T) {
	// root has rules, root/sub also has rules -> sub's rules should win
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(sub, 0755))

	rootRules := filepath.Join(root, ".dtctl-pii.yaml")
	subRules := filepath.Join(sub, ".dtctl-pii.yaml")
	require.NoError(t, os.WriteFile(rootRules, []byte("version: v1\nrules: []\n"), 0644))
	require.NoError(t, os.WriteFile(subRules, []byte("version: v1\nrules: []\n"), 0644))

	found := FindCustomRulesFile(sub)
	assert.Equal(t, subRules, found)
}

// --- Integration: custom rules + redaction ---

func TestRedactRecordsWithCustomRules(t *testing.T) {
	r, err := NewRedactor(Config{
		Mode: ModeLite,
		CustomRules: []CustomRule{
			{Field: "data.customerRef", Category: "PERSON"},
			{Field: "data.action", Skip: true},
		},
	})
	require.NoError(t, err)

	records := []map[string]interface{}{
		{
			"data.customerRef": "John Doe",      // custom rule -> PERSON
			"data.action":      "login",         // custom skip -> preserved
			"email":            "john@test.com", // built-in pattern -> EMAIL
			"data.status":      "active",        // no match -> preserved
		},
	}

	result := r.RedactRecords(records)

	assert.Equal(t, "[PERSON]", result[0]["data.customerRef"])
	assert.Equal(t, "login", result[0]["data.action"])
	assert.Equal(t, "[EMAIL]", result[0]["email"])
	assert.Equal(t, "active", result[0]["data.status"])
}

func TestRedactRecordsCustomSkipOverridesBuiltin(t *testing.T) {
	r, err := NewRedactor(Config{
		Mode: ModeLite,
		CustomRules: []CustomRule{
			{Field: "email", Skip: true}, // override built-in EMAIL pattern
		},
	})
	require.NoError(t, err)

	records := []map[string]interface{}{
		{
			"email": "john@test.com", // skip rule overrides built-in
		},
	}

	result := r.RedactRecords(records)
	assert.Equal(t, "john@test.com", result[0]["email"]) // preserved!
}

func TestRedactRecordsCustomGlobRule(t *testing.T) {
	r, err := NewRedactor(Config{
		Mode: ModeLite,
		CustomRules: []CustomRule{
			{Field: "custom.*", Category: "CUSTOM_PII"},
		},
	})
	require.NoError(t, err)

	// DQL returns flat dotted keys
	records := []map[string]interface{}{
		{
			"custom.field1": "sensitive data",
			"custom.field2": "more data",
			"other.field":   "safe data",
		},
	}

	result := r.RedactRecords(records)

	assert.Equal(t, "[CUSTOM_PII]", result[0]["custom.field1"])
	assert.Equal(t, "[CUSTOM_PII]", result[0]["custom.field2"])
	assert.Equal(t, "safe data", result[0]["other.field"])
}

func TestRedactRecordsCustomDefaultCategory(t *testing.T) {
	r, err := NewRedactor(Config{
		Mode: ModeLite,
		CustomRules: []CustomRule{
			{Field: "data.secret", Category: "REDACTED"}, // default category
		},
	})
	require.NoError(t, err)

	records := []map[string]interface{}{
		{"data.secret": "top secret"},
	}

	result := r.RedactRecords(records)
	assert.Equal(t, "[REDACTED]", result[0]["data.secret"])
}
