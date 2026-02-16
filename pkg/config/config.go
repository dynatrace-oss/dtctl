package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"
)

// Config represents the dtctl configuration
type Config struct {
	APIVersion     string            `yaml:"apiVersion"`
	Kind           string            `yaml:"kind"`
	CurrentContext string            `yaml:"current-context"`
	Contexts       []NamedContext    `yaml:"contexts"`
	Tokens         []NamedToken      `yaml:"tokens"`
	Preferences    Preferences       `yaml:"preferences"`
	Aliases        map[string]string `yaml:"aliases,omitempty"`
}

// NamedContext holds a context with its name
type NamedContext struct {
	Name    string  `yaml:"name" table:"NAME"`
	Context Context `yaml:"context" table:"-"`
}

// SafetyLevel defines the allowed operations for a context
type SafetyLevel string

const (
	// SafetyLevelReadOnly allows only read operations
	SafetyLevelReadOnly SafetyLevel = "readonly"
	// SafetyLevelReadWriteMine allows create/update/delete of own resources only
	SafetyLevelReadWriteMine SafetyLevel = "readwrite-mine"
	// SafetyLevelReadWriteAll allows modification of all resources (no bucket deletion)
	SafetyLevelReadWriteAll SafetyLevel = "readwrite-all"
	// SafetyLevelDangerouslyUnrestricted allows all operations including data deletion
	SafetyLevelDangerouslyUnrestricted SafetyLevel = "dangerously-unrestricted"

	// DefaultSafetyLevel is used when no safety level is specified.
	// We use readwrite-all as default to avoid breaking existing workflows.
	// This allows all operations except bucket deletion, which is the most
	// common use case and matches pre-safety-level behavior.
	DefaultSafetyLevel = SafetyLevelReadWriteAll
)

// ValidSafetyLevels returns all valid safety level values
func ValidSafetyLevels() []SafetyLevel {
	return []SafetyLevel{
		SafetyLevelReadOnly,
		SafetyLevelReadWriteMine,
		SafetyLevelReadWriteAll,
		SafetyLevelDangerouslyUnrestricted,
	}
}

// IsValid checks if the safety level is valid
func (s SafetyLevel) IsValid() bool {
	switch s {
	case SafetyLevelReadOnly, SafetyLevelReadWriteMine, SafetyLevelReadWriteAll,
		SafetyLevelDangerouslyUnrestricted:
		return true
	case "":
		return true // Empty is valid (defaults to readwrite-all)
	}
	return false
}

// String returns the string representation of the safety level
func (s SafetyLevel) String() string {
	if s == "" {
		return string(DefaultSafetyLevel)
	}
	return string(s)
}

// Context holds the connection information for a Dynatrace environment
type Context struct {
	Environment string      `yaml:"environment" table:"ENVIRONMENT"`
	TokenRef    string      `yaml:"token-ref" table:"TOKEN-REF"`
	SafetyLevel SafetyLevel `yaml:"safety-level,omitempty" table:"SAFETY-LEVEL"`
	Description string      `yaml:"description,omitempty" table:"DESCRIPTION,wide"`
}

// NamedToken holds a token with its name
type NamedToken struct {
	Name  string `yaml:"name"`
	Token string `yaml:"token"`
}

// Preferences holds user preferences
type Preferences struct {
	Output string `yaml:"output,omitempty"`
	Editor string `yaml:"editor,omitempty"`
}

// DefaultConfigPath returns the default config file path following XDG Base Directory spec
// Returns: XDG_CONFIG_HOME/dtctl/config (typically ~/.config/dtctl/config)
func DefaultConfigPath() string {
	return filepath.Join(xdg.ConfigHome, "dtctl", "config")
}

// ConfigDir returns the config directory path following XDG Base Directory spec
func ConfigDir() string {
	return filepath.Join(xdg.ConfigHome, "dtctl")
}

// CacheDir returns the cache directory path following XDG Base Directory spec
func CacheDir() string {
	return filepath.Join(xdg.CacheHome, "dtctl")
}

// DataDir returns the data directory path following XDG Base Directory spec
func DataDir() string {
	return filepath.Join(xdg.DataHome, "dtctl")
}

// LocalConfigName is the name of the per-project config file
const LocalConfigName = ".dtctl.yaml"

// FindLocalConfig searches for a .dtctl.yaml file starting from the current
// directory and walking up to the root. Returns empty string if not found.
func FindLocalConfig() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	return findLocalConfigFrom(cwd)
}

// findLocalConfigFrom searches for .dtctl.yaml starting from the given directory
func findLocalConfigFrom(startDir string) string {
	dir := startDir
	for {
		configPath := filepath.Join(dir, LocalConfigName)
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			return ""
		}
		dir = parent
	}
}

// Load loads the configuration with the following precedence:
//  1. Local config (.dtctl.yaml in current directory or parent directories)
//  2. Global config (XDG_CONFIG_HOME/dtctl/config)
//
// If a local config is found, it is used exclusively (not merged with global).
func Load() (*Config, error) {
	// Check for local config first
	localConfig := FindLocalConfig()
	if localConfig != "" {
		return LoadFrom(localConfig)
	}

	// Fall back to global config
	return LoadFrom(DefaultConfigPath())
}

// LoadFrom loads the configuration from a specific path
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s. Run 'dtctl config set-context' to create one", path)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// Save saves the configuration to the default path
func (c *Config) Save() error {
	return c.SaveTo(DefaultConfigPath())
}

// SaveTo saves the configuration to a specific path
func (c *Config) SaveTo(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// CurrentContextObj returns the current context object
func (c *Config) CurrentContextObj() (*Context, error) {
	if c.CurrentContext == "" {
		return nil, fmt.Errorf("no current context set")
	}

	for _, nc := range c.Contexts {
		if nc.Name == c.CurrentContext {
			return &nc.Context, nil
		}
	}

	return nil, fmt.Errorf("current context %q not found", c.CurrentContext)
}

// GetToken retrieves a token by reference name.
// It first tries the OS keyring, then falls back to the config file.
func (c *Config) GetToken(tokenRef string) (string, error) {
	// Try keyring first
	if IsKeyringAvailable() {
		ts := NewTokenStore()
		token, err := ts.GetToken(tokenRef)
		if err == nil && token != "" {
			return token, nil
		}
	}

	// Fall back to config file
	for _, nt := range c.Tokens {
		if nt.Name == tokenRef {
			if nt.Token != "" {
				return nt.Token, nil
			}
			// Token reference exists but value is empty (migrated to keyring)
			return "", fmt.Errorf("token %q not found in keyring (may need to re-add credentials)", tokenRef)
		}
	}
	return "", fmt.Errorf("token %q not found", tokenRef)
}

// MustGetToken retrieves a token by reference name, returning empty string on error
func (c *Config) MustGetToken(tokenRef string) string {
	token, _ := c.GetToken(tokenRef)
	return token
}

// ContextOptions holds optional fields for context configuration
type ContextOptions struct {
	SafetyLevel SafetyLevel
	Description string
}

// SetContext creates or updates a context
func (c *Config) SetContext(name, environment, tokenRef string) {
	c.SetContextWithOptions(name, environment, tokenRef, nil)
}

// SetContextWithOptions creates or updates a context with optional fields
func (c *Config) SetContextWithOptions(name, environment, tokenRef string, opts *ContextOptions) {
	for i, nc := range c.Contexts {
		if nc.Name == name {
			c.Contexts[i].Context.Environment = environment
			if tokenRef != "" {
				c.Contexts[i].Context.TokenRef = tokenRef
			}
			if opts != nil {
				if opts.SafetyLevel != "" {
					c.Contexts[i].Context.SafetyLevel = opts.SafetyLevel
				}
				if opts.Description != "" {
					c.Contexts[i].Context.Description = opts.Description
				}
			}
			return
		}
	}

	ctx := Context{
		Environment: environment,
		TokenRef:    tokenRef,
	}
	if opts != nil {
		ctx.SafetyLevel = opts.SafetyLevel
		ctx.Description = opts.Description
	}

	c.Contexts = append(c.Contexts, NamedContext{
		Name:    name,
		Context: ctx,
	})
}

// GetEffectiveSafetyLevel returns the effective safety level for a context
// If no safety level is set, returns the default (readwrite-all)
func (c *Context) GetEffectiveSafetyLevel() SafetyLevel {
	if c.SafetyLevel == "" {
		return DefaultSafetyLevel
	}
	return c.SafetyLevel
}

// DeleteContext removes a context by name.
// Returns an error if the context is not found.
func (c *Config) DeleteContext(name string) error {
	for i, nc := range c.Contexts {
		if nc.Name == name {
			c.Contexts = append(c.Contexts[:i], c.Contexts[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("context %q not found", name)
}

// SetToken creates or updates a token.
// If keyring is available, the token is stored securely in the OS keyring
// and only a reference is kept in the config file.
func (c *Config) SetToken(name, token string) error {
	// Try to store in keyring first
	if IsKeyringAvailable() {
		ts := NewTokenStore()
		if err := ts.SetToken(name, token); err != nil {
			return fmt.Errorf("failed to store token in keyring: %w", err)
		}
		// Store empty token in config (reference only)
		token = ""
	}

	// Update or add token entry in config
	for i, nt := range c.Tokens {
		if nt.Name == name {
			c.Tokens[i].Token = token
			return nil
		}
	}

	c.Tokens = append(c.Tokens, NamedToken{
		Name:  name,
		Token: token,
	})
	return nil
}

// NewConfig creates a new default configuration
func NewConfig() *Config {
	return &Config{
		APIVersion: "v1",
		Kind:       "Config",
		Contexts:   []NamedContext{},
		Tokens:     []NamedToken{},
		Preferences: Preferences{
			Output: "table",
			Editor: "vim",
		},
	}
}
