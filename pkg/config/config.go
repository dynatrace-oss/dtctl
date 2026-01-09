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
	APIVersion     string         `yaml:"apiVersion"`
	Kind           string         `yaml:"kind"`
	CurrentContext string         `yaml:"current-context"`
	Contexts       []NamedContext `yaml:"contexts"`
	Tokens         []NamedToken   `yaml:"tokens"`
	Preferences    Preferences    `yaml:"preferences"`
}

// NamedContext holds a context with its name
type NamedContext struct {
	Name    string  `yaml:"name"`
	Context Context `yaml:"context"`
}

// Context holds the connection information for a Dynatrace environment
type Context struct {
	Environment string `yaml:"environment"`
	TokenRef    string `yaml:"token-ref"`
	Namespace   string `yaml:"namespace,omitempty"`
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
// Priority order:
// 1. XDG_CONFIG_HOME/dtctl/config (typically ~/.config/dtctl/config)
// 2. Legacy path ~/.dtctl/config (for backwards compatibility, will be migrated)
func DefaultConfigPath() string {
	// XDG-compliant path
	xdgPath := filepath.Join(xdg.ConfigHome, "dtctl", "config")

	// Legacy path for backwards compatibility
	legacyPath := ""
	if home, err := os.UserHomeDir(); err == nil {
		legacyPath = filepath.Join(home, ".dtctl", "config")
	}

	// If XDG path exists, use it
	if _, err := os.Stat(xdgPath); err == nil {
		return xdgPath
	}

	// If legacy path exists and XDG path doesn't, migrate
	if legacyPath != "" {
		if _, err := os.Stat(legacyPath); err == nil {
			// Legacy config exists, attempt migration
			if err := migrateLegacyConfig(legacyPath, xdgPath); err == nil {
				return xdgPath
			}
			// Migration failed, fall back to legacy path
			return legacyPath
		}
	}

	// Default to XDG path for new installations
	return xdgPath
}

// migrateLegacyConfig migrates config from legacy path to XDG path
func migrateLegacyConfig(legacyPath, xdgPath string) error {
	// Read legacy config
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return fmt.Errorf("failed to read legacy config: %w", err)
	}

	// Create XDG config directory
	if err := os.MkdirAll(filepath.Dir(xdgPath), 0700); err != nil {
		return fmt.Errorf("failed to create XDG config directory: %w", err)
	}

	// Write to XDG path
	if err := os.WriteFile(xdgPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write XDG config: %w", err)
	}

	// Successfully migrated - optionally remove legacy config
	// For safety, we keep the legacy config as a backup
	// Users can manually remove ~/.dtctl after verifying the migration

	return nil
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

// Load loads the configuration from the default path
func Load() (*Config, error) {
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

// SetContext creates or updates a context
func (c *Config) SetContext(name, environment, tokenRef, namespace string) {
	for i, nc := range c.Contexts {
		if nc.Name == name {
			c.Contexts[i].Context.Environment = environment
			if tokenRef != "" {
				c.Contexts[i].Context.TokenRef = tokenRef
			}
			if namespace != "" {
				c.Contexts[i].Context.Namespace = namespace
			}
			return
		}
	}

	c.Contexts = append(c.Contexts, NamedContext{
		Name: name,
		Context: Context{
			Environment: environment,
			TokenRef:    tokenRef,
			Namespace:   namespace,
		},
	})
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
