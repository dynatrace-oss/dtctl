package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAliasName(t *testing.T) {
	tests := []struct {
		name    string
		alias   string
		wantErr bool
	}{
		{"valid simple", "wf", false},
		{"valid with hyphen", "prod-wf", false},
		{"valid with underscore", "prod_wf", false},
		{"valid mixed", "prod-wf_v2", false},
		{"valid numbers", "wf123", false},
		{"empty name", "", true},
		{"starts with hyphen", "-invalid", true},
		{"starts with underscore", "_invalid", true},
		{"contains space", "my alias", true},
		{"contains special char", "my-alias!", true},
		{"contains dot", "my.alias", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAliasName(tt.alias)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSetAlias(t *testing.T) {
	tests := []struct {
		name        string
		aliasName   string
		expansion   string
		builtinFunc func(string) bool
		wantErr     string
	}{
		{
			name:      "simple alias",
			aliasName: "wf",
			expansion: "get workflows",
		},
		{
			name:      "alias with hyphens and underscores",
			aliasName: "prod-wf_v2",
			expansion: "get workflows --context=production",
		},
		{
			name:      "parameterized alias",
			aliasName: "pw",
			expansion: "get workflow $1",
		},
		{
			name:      "shell alias",
			aliasName: "count",
			expansion: "!dtctl get workflows -o json | jq length",
		},
		{
			name:      "rejects empty name",
			aliasName: "",
			expansion: "get workflows",
			wantErr:   "alias name cannot be empty",
		},
		{
			name:      "rejects invalid characters",
			aliasName: "my alias!",
			expansion: "get workflows",
			wantErr:   "invalid",
		},
		{
			name:      "rejects builtin shadow",
			aliasName: "get",
			expansion: "describe workflows",
			builtinFunc: func(s string) bool {
				return s == "get"
			},
			wantErr: "built-in command",
		},
		{
			name:      "allows non-builtin name",
			aliasName: "wf",
			expansion: "get workflows",
			builtinFunc: func(s string) bool {
				return s == "get"
			},
		},
		{
			name:      "rejects empty expansion",
			aliasName: "wf",
			expansion: "",
			wantErr:   "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig()
			err := cfg.SetAlias(tt.aliasName, tt.expansion, tt.builtinFunc)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				got, ok := cfg.GetAlias(tt.aliasName)
				require.True(t, ok)
				require.Equal(t, tt.expansion, got)
			}
		})
	}
}

func TestSetAlias_UpdateExisting(t *testing.T) {
	cfg := NewConfig()

	// Set initial alias
	err := cfg.SetAlias("wf", "get workflows", nil)
	require.NoError(t, err)

	// Update it
	err = cfg.SetAlias("wf", "get workflows --context=production", nil)
	require.NoError(t, err)

	// Verify updated value
	got, ok := cfg.GetAlias("wf")
	require.True(t, ok)
	require.Equal(t, "get workflows --context=production", got)
}

func TestDeleteAlias(t *testing.T) {
	cfg := NewConfig()

	// Delete from empty config
	err := cfg.DeleteAlias("wf")
	require.ErrorContains(t, err, "not found")

	// Add an alias
	err = cfg.SetAlias("wf", "get workflows", nil)
	require.NoError(t, err)

	// Delete it
	err = cfg.DeleteAlias("wf")
	require.NoError(t, err)

	// Verify it's gone
	_, ok := cfg.GetAlias("wf")
	require.False(t, ok)

	// Try to delete again
	err = cfg.DeleteAlias("wf")
	require.ErrorContains(t, err, "not found")
}

func TestGetAlias(t *testing.T) {
	cfg := NewConfig()

	// Get from empty config
	_, ok := cfg.GetAlias("wf")
	require.False(t, ok)

	// Add an alias
	err := cfg.SetAlias("wf", "get workflows", nil)
	require.NoError(t, err)

	// Get it
	exp, ok := cfg.GetAlias("wf")
	require.True(t, ok)
	require.Equal(t, "get workflows", exp)
}

func TestListAliases(t *testing.T) {
	cfg := NewConfig()

	// List empty
	entries := cfg.ListAliases()
	require.Empty(t, entries)

	// Add several aliases
	require.NoError(t, cfg.SetAlias("wf", "get workflows", nil))
	require.NoError(t, cfg.SetAlias("prod-wf", "get workflows --context=production", nil))
	require.NoError(t, cfg.SetAlias("deploy", "apply -f $1 --context=$2", nil))

	// List them
	entries = cfg.ListAliases()
	require.Len(t, entries, 3)

	// Verify sorted alphabetically
	require.Equal(t, "deploy", entries[0].Name)
	require.Equal(t, "prod-wf", entries[1].Name)
	require.Equal(t, "wf", entries[2].Name)

	// Verify expansions
	require.Equal(t, "apply -f $1 --context=$2", entries[0].Expansion)
	require.Equal(t, "get workflows --context=production", entries[1].Expansion)
	require.Equal(t, "get workflows", entries[2].Expansion)
}

func TestExportAliases(t *testing.T) {
	cfg := NewConfig()
	require.NoError(t, cfg.SetAlias("wf", "get workflows", nil))
	require.NoError(t, cfg.SetAlias("prod-wf", "get workflows --context=production", nil))

	tmpDir := t.TempDir()
	exportPath := filepath.Join(tmpDir, "aliases.yaml")

	err := cfg.ExportAliases(exportPath)
	require.NoError(t, err)

	// Verify file exists
	data, err := os.ReadFile(exportPath)
	require.NoError(t, err)

	// Verify content
	require.Contains(t, string(data), "aliases:")
	require.Contains(t, string(data), "wf: get workflows")
	require.Contains(t, string(data), "prod-wf: get workflows --context=production")
}

func TestImportAliases(t *testing.T) {
	t.Run("successful import", func(t *testing.T) {
		tmpDir := t.TempDir()
		importPath := filepath.Join(tmpDir, "aliases.yaml")

		// Create import file
		content := `aliases:
  wf: get workflows
  prod-wf: get workflows --context=production
  deploy: apply -f $1 --context=$2
`
		err := os.WriteFile(importPath, []byte(content), 0600)
		require.NoError(t, err)

		cfg := NewConfig()
		conflicts, err := cfg.ImportAliases(importPath, false, nil)
		require.NoError(t, err)
		require.Empty(t, conflicts)

		// Verify imported
		exp, ok := cfg.GetAlias("wf")
		require.True(t, ok)
		require.Equal(t, "get workflows", exp)

		exp, ok = cfg.GetAlias("prod-wf")
		require.True(t, ok)
		require.Equal(t, "get workflows --context=production", exp)
	})

	t.Run("conflict without overwrite", func(t *testing.T) {
		tmpDir := t.TempDir()
		importPath := filepath.Join(tmpDir, "aliases.yaml")

		// Create import file
		content := `aliases:
  wf: get workflows --new
  prod-wf: get workflows --context=production
`
		err := os.WriteFile(importPath, []byte(content), 0600)
		require.NoError(t, err)

		cfg := NewConfig()
		require.NoError(t, cfg.SetAlias("wf", "get workflows --old", nil))

		conflicts, err := cfg.ImportAliases(importPath, false, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"wf"}, conflicts)

		// Verify old value is preserved
		exp, ok := cfg.GetAlias("wf")
		require.True(t, ok)
		require.Equal(t, "get workflows --old", exp)

		// Verify new alias was still added
		exp, ok = cfg.GetAlias("prod-wf")
		require.True(t, ok)
		require.Equal(t, "get workflows --context=production", exp)
	})

	t.Run("conflict with overwrite", func(t *testing.T) {
		tmpDir := t.TempDir()
		importPath := filepath.Join(tmpDir, "aliases.yaml")

		// Create import file
		content := `aliases:
  wf: get workflows --new
`
		err := os.WriteFile(importPath, []byte(content), 0600)
		require.NoError(t, err)

		cfg := NewConfig()
		require.NoError(t, cfg.SetAlias("wf", "get workflows --old", nil))

		conflicts, err := cfg.ImportAliases(importPath, true, nil)
		require.NoError(t, err)
		require.Empty(t, conflicts)

		// Verify new value overwrote old
		exp, ok := cfg.GetAlias("wf")
		require.True(t, ok)
		require.Equal(t, "get workflows --new", exp)
	})

	t.Run("rejects invalid alias name", func(t *testing.T) {
		tmpDir := t.TempDir()
		importPath := filepath.Join(tmpDir, "aliases.yaml")

		// Create import file with invalid name
		content := `aliases:
  "my alias!": get workflows
`
		err := os.WriteFile(importPath, []byte(content), 0600)
		require.NoError(t, err)

		cfg := NewConfig()
		_, err = cfg.ImportAliases(importPath, false, nil)
		require.ErrorContains(t, err, "invalid alias")
	})

	t.Run("rejects builtin shadow", func(t *testing.T) {
		tmpDir := t.TempDir()
		importPath := filepath.Join(tmpDir, "aliases.yaml")

		// Create import file
		content := `aliases:
  get: describe workflows
`
		err := os.WriteFile(importPath, []byte(content), 0600)
		require.NoError(t, err)

		cfg := NewConfig()
		_, err = cfg.ImportAliases(importPath, false, func(name string) bool {
			return name == "get"
		})
		require.ErrorContains(t, err, "shadows a built-in command")
	})

	t.Run("invalid yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		importPath := filepath.Join(tmpDir, "aliases.yaml")

		// Create invalid YAML
		content := `aliases: [invalid`
		err := os.WriteFile(importPath, []byte(content), 0600)
		require.NoError(t, err)

		cfg := NewConfig()
		_, err = cfg.ImportAliases(importPath, false, nil)
		require.ErrorContains(t, err, "failed to parse")
	})

	t.Run("file not found", func(t *testing.T) {
		cfg := NewConfig()
		_, err := cfg.ImportAliases("/nonexistent/path.yaml", false, nil)
		require.ErrorContains(t, err, "failed to read")
	})
}

func TestExportImportRoundTrip(t *testing.T) {
	// Create config with aliases
	cfg1 := NewConfig()
	require.NoError(t, cfg1.SetAlias("wf", "get workflows", nil))
	require.NoError(t, cfg1.SetAlias("prod-wf", "get workflows --context=production", nil))
	require.NoError(t, cfg1.SetAlias("deploy", "apply -f $1 --context=$2", nil))
	require.NoError(t, cfg1.SetAlias("count", "!dtctl get wf -o json | jq length", nil))

	tmpDir := t.TempDir()
	exportPath := filepath.Join(tmpDir, "aliases.yaml")

	// Export
	err := cfg1.ExportAliases(exportPath)
	require.NoError(t, err)

	// Import into new config
	cfg2 := NewConfig()
	conflicts, err := cfg2.ImportAliases(exportPath, false, nil)
	require.NoError(t, err)
	require.Empty(t, conflicts)

	// Verify all aliases match
	entries1 := cfg1.ListAliases()
	entries2 := cfg2.ListAliases()
	require.Equal(t, entries1, entries2)
}
