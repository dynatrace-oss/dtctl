package config

import (
	"fmt"
	"os"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// aliasNameRegex validates alias names: letters, numbers, hyphens, underscores
var aliasNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// ValidateAliasName checks that an alias name is syntactically valid.
func ValidateAliasName(name string) error {
	if name == "" {
		return fmt.Errorf("alias name cannot be empty")
	}
	if !aliasNameRegex.MatchString(name) {
		return fmt.Errorf("alias name %q is invalid: use only letters, numbers, hyphens, and underscores", name)
	}
	return nil
}

// SetAlias adds or updates an alias. Returns an error if the name is invalid.
// builtinCheck is called to verify the name does not shadow a built-in command.
func (c *Config) SetAlias(name, expansion string, builtinCheck func(string) bool) error {
	if err := ValidateAliasName(name); err != nil {
		return err
	}
	if builtinCheck != nil && builtinCheck(name) {
		return fmt.Errorf("%q is a built-in command and cannot be used as an alias name", name)
	}
	if expansion == "" {
		return fmt.Errorf("alias expansion cannot be empty")
	}
	if c.Aliases == nil {
		c.Aliases = make(map[string]string)
	}
	c.Aliases[name] = expansion
	return nil
}

// DeleteAlias removes an alias by name. Returns an error if it does not exist.
func (c *Config) DeleteAlias(name string) error {
	if c.Aliases == nil {
		return fmt.Errorf("alias %q not found", name)
	}
	if _, ok := c.Aliases[name]; !ok {
		return fmt.Errorf("alias %q not found", name)
	}
	delete(c.Aliases, name)
	return nil
}

// GetAlias returns the expansion for an alias, or empty string if not found.
func (c *Config) GetAlias(name string) (string, bool) {
	if c.Aliases == nil {
		return "", false
	}
	exp, ok := c.Aliases[name]
	return exp, ok
}

// ListAliases returns all aliases sorted alphabetically by name.
func (c *Config) ListAliases() []AliasEntry {
	entries := make([]AliasEntry, 0, len(c.Aliases))
	for name, expansion := range c.Aliases {
		entries = append(entries, AliasEntry{Name: name, Expansion: expansion})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// AliasEntry is a single alias for display purposes.
type AliasEntry struct {
	Name      string `table:"NAME"`
	Expansion string `table:"EXPANSION"`
}

// AliasFile represents the YAML structure for import/export.
type AliasFile struct {
	Aliases map[string]string `yaml:"aliases"`
}

// ExportAliases writes aliases to a file in YAML format.
func (c *Config) ExportAliases(path string) error {
	af := AliasFile{Aliases: c.Aliases}
	data, err := yaml.Marshal(af)
	if err != nil {
		return fmt.Errorf("failed to marshal aliases: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// ImportAliases reads aliases from a YAML file and merges them into the config.
// If overwrite is false, existing aliases are not replaced and conflicts are
// returned as a list of names.
func (c *Config) ImportAliases(path string, overwrite bool, builtinCheck func(string) bool) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read alias file: %w", err)
	}

	var af AliasFile
	if err := yaml.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("failed to parse alias file: %w", err)
	}

	if c.Aliases == nil {
		c.Aliases = make(map[string]string)
	}

	var conflicts []string
	for name, expansion := range af.Aliases {
		if err := ValidateAliasName(name); err != nil {
			return nil, fmt.Errorf("invalid alias in file: %w", err)
		}
		if builtinCheck != nil && builtinCheck(name) {
			return nil, fmt.Errorf("alias %q in file shadows a built-in command", name)
		}
		if _, exists := c.Aliases[name]; exists && !overwrite {
			conflicts = append(conflicts, name)
			continue
		}
		c.Aliases[name] = expansion
	}

	// Sort conflicts for consistent output
	sort.Strings(conflicts)
	return conflicts, nil
}
