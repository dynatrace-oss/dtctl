package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	// Spill holds the global result-spill settings (D15). Per-context overrides
	// live on Context.Spill.
	Spill SpillConfig `yaml:"spill,omitempty"`

	// localPath is the path of the auto-discovered local .dtctl.yaml this
	// config was loaded from, if any. Empty when loaded from the global config
	// or from an explicit --config file. Unexported so it is never serialized.
	localPath string
	// ignoredExecKeys is true when code-execution keys (aliases and/or apply
	// hooks) are present in an auto-discovered local config. Such keys are
	// loaded into the struct (so they round-trip safely through save/edit) but
	// are never honored at runtime — alias resolution and hook execution check
	// IsLocal() and skip them. See markLocal, GetPreApplyHook, resolveAlias.
	ignoredExecKeys bool
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

// Hooks holds hook commands for lifecycle events
type Hooks struct {
	PreApply  string `yaml:"pre-apply,omitempty"`
	PostApply string `yaml:"post-apply,omitempty"`
}

// Context holds the connection information for a Dynatrace environment
type Context struct {
	Environment string      `yaml:"environment" table:"ENVIRONMENT"`
	TokenRef    string      `yaml:"token-ref" table:"TOKEN-REF"`
	SafetyLevel SafetyLevel `yaml:"safety-level,omitempty" table:"SAFETY-LEVEL"`
	Description string      `yaml:"description,omitempty" table:"DESCRIPTION,wide"`
	Hooks       Hooks       `yaml:"hooks,omitempty"`
	// Spill overrides the global spill settings for this context (D15). Nil
	// fields inherit the global spill config.
	Spill *SpillConfig `yaml:"spill,omitempty"`
}

// SpillConfig holds the result-spill settings (D15). Threshold and TTL are kept
// as human-friendly strings in the file (e.g. "50KB", "24h") and parsed when
// resolving the effective settings. All fields are optional; an unset field
// inherits from the next layer in the precedence chain
// (flag → env → context-config → global-config → built-in default).
type SpillConfig struct {
	Mode      string `yaml:"mode,omitempty"`      // auto|always|never
	Dir       string `yaml:"dir,omitempty"`       // base directory for spilled files
	Format    string `yaml:"format,omitempty"`    // jsonl|json|csv|parquet (default jsonl)
	Threshold string `yaml:"threshold,omitempty"` // e.g. "50KB"
	TTL       string `yaml:"ttl,omitempty"`       // e.g. "24h"
}

// EffectiveSpillConfig merges the global spill config with the current context's
// override (context wins per field, D15). Env and flag layers are applied by the
// caller on top of this base.
func (c *Config) EffectiveSpillConfig() SpillConfig {
	merged := c.Spill
	if ctx, err := c.CurrentContextObj(); err == nil && ctx.Spill != nil {
		ov := ctx.Spill
		if ov.Mode != "" {
			merged.Mode = ov.Mode
		}
		if ov.Dir != "" {
			merged.Dir = ov.Dir
		}
		if ov.Format != "" {
			merged.Format = ov.Format
		}
		if ov.Threshold != "" {
			merged.Threshold = ov.Threshold
		}
		if ov.TTL != "" {
			merged.TTL = ov.TTL
		}
	}
	return merged
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
	Hooks  Hooks  `yaml:"hooks,omitempty"`
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
//
// Security: an auto-discovered local .dtctl.yaml is treated as untrusted (the
// classic "untrusted working directory / checked-out repo / shared dir"
// scenario). Code-execution keys — shell aliases and apply hooks — defined in
// such a config are never honored: alias resolution and hook execution check
// IsLocal() and skip them. These keys are honored only from the global config
// or an explicit --config file (loaded via LoadFrom), which carry stronger
// ownership expectations. The keys are still loaded into the struct (and never
// mutated here) so that config-management commands round-trip the file without
// silently destroying a user's own aliases or hooks.
func Load() (*Config, error) {
	// Check for local config first
	localConfig := FindLocalConfig()
	if localConfig != "" {
		cfg, err := LoadFrom(localConfig)
		if err != nil {
			return nil, err
		}
		cfg.markLocal(localConfig)
		return cfg, nil
	}

	// Fall back to global config
	return LoadFrom(DefaultConfigPath())
}

// markLocal records that the config was loaded from an auto-discovered local
// .dtctl.yaml and notes whether it carries any code-execution keys (top-level
// aliases, global apply hooks, or per-context apply hooks). It does NOT mutate
// those keys: the values are needed for safe round-tripping by edit/save
// commands, and they are ignored at the point of use (resolveAlias,
// GetPreApplyHook/GetPostApplyHook) rather than being stripped at load. The
// recorded flag lets callers surface a one-line warning naming the local config
// in effect.
func (c *Config) markLocal(path string) {
	c.localPath = path
	c.ignoredExecKeys = c.hasExecKeys()
}

// hasExecKeys reports whether the config defines any code-execution key:
// a top-level alias, a global (preferences) apply hook, or a per-context hook.
func (c *Config) hasExecKeys() bool {
	if len(c.Aliases) > 0 {
		return true
	}
	if c.Preferences.Hooks.PreApply != "" || c.Preferences.Hooks.PostApply != "" {
		return true
	}
	for i := range c.Contexts {
		h := c.Contexts[i].Context.Hooks
		if h.PreApply != "" || h.PostApply != "" {
			return true
		}
	}
	return false
}

// IsLocal reports whether the config was loaded from an auto-discovered local
// .dtctl.yaml (as opposed to the global config or an explicit --config file).
// Code-execution keys (aliases, apply hooks) are ignored when this is true.
func (c *Config) IsLocal() bool { return c.localPath != "" }

// LocalConfigPath returns the path of the auto-discovered local .dtctl.yaml the
// config was loaded from, or "" if it was not loaded from a local config.
func (c *Config) LocalConfigPath() string { return c.localPath }

// IgnoredExecKeys reports whether code-execution keys (aliases, apply hooks)
// are present in the auto-discovered local config and are therefore ignored at
// runtime. See markLocal.
func (c *Config) IgnoredExecKeys() bool { return c.ignoredExecKeys }

// LoadFrom loads the configuration from a specific path
func LoadFrom(path string) (*Config, error) {
	return loadFrom(path, true)
}

// LoadFromWithoutExpansion loads the configuration from a specific path without
// expanding environment variables. Use this to inspect raw template values.
func LoadFromWithoutExpansion(path string) (*Config, error) {
	return loadFrom(path, false)
}

// LoadWithoutExpansion loads the configuration without expanding environment variables,
// using the same search order as Load (local config, then global config).
func LoadWithoutExpansion() (*Config, error) {
	if local := FindLocalConfig(); local != "" {
		cfg, err := LoadFromWithoutExpansion(local)
		if err != nil {
			return nil, err
		}
		// Tag the local config so callers ignore its code-execution keys at the
		// point of use, without mutating the loaded values (this path also
		// backs config-management commands that load-modify-save the file). See
		// Load.
		cfg.markLocal(local)
		return cfg, nil
	}
	return LoadFromWithoutExpansion(DefaultConfigPath())
}

func loadFrom(path string, expandEnv bool) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s. Run 'dtctl config set-context' to create one", path)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if expandEnv {
		// Expand environment variables in the config file.
		//
		// We deliberately do NOT use os.ExpandEnv: it expands every $-prefixed
		// token, including shell positional parameters like $1/$2/$@ that can
		// legitimately appear in opaque config values such as hook commands.
		// expandEnvPreservingShellParams leaves those alone and substitutes only
		// real environment variable names.
		data = []byte(expandEnvPreservingShellParams(string(data)))
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// expandEnvPreservingShellParams expands $VAR and ${VAR} references using
// the process environment, but leaves shell positional parameters and
// special parameters intact (e.g. $1, ${10}, $@, $*, $#, $?, $!, $$, $0).
// This lets users embed those tokens in opaque config values such as hook
// commands without having them silently rewritten to the empty string at
// config load.
//
// Lookups for ordinary names that are not set in the environment fall back
// to "" (matching os.ExpandEnv); use "${VAR}" in the config when an
// unexpanded literal is desired (and `VAR` is in scope to be unset).
//
// We implement the scan ourselves rather than calling os.Expand so the
// brace form (`${10}`) is preserved exactly for shell positionals — the
// stdlib helper passes the bare name to the mapper, losing the braces.
func expandEnvPreservingShellParams(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if c != '$' || i+1 >= len(s) {
			b.WriteByte(c)
			i++
			continue
		}

		// Brace form: ${...}
		if s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				// No closing brace — treat as literal.
				b.WriteByte(c)
				i++
				continue
			}
			name := s[i+2 : i+2+end]
			if isShellPositionalOrSpecial(name) {
				b.WriteString(s[i : i+2+end+1]) // preserve `${name}` verbatim
			} else {
				b.WriteString(os.Getenv(name))
			}
			i += 2 + end + 1
			continue
		}

		// Bare form: $NAME or $1 or $@ etc.
		nameLen := bareEnvNameLen(s[i+1:])
		if nameLen == 0 {
			// `$` followed by something that is not a valid name char and
			// not a special parameter — write `$` literally.
			b.WriteByte(c)
			i++
			continue
		}
		name := s[i+1 : i+1+nameLen]
		if isShellPositionalOrSpecial(name) {
			b.WriteString(s[i : i+1+nameLen]) // preserve `$name` verbatim
		} else {
			b.WriteString(os.Getenv(name))
		}
		i += 1 + nameLen
	}
	return b.String()
}

// bareEnvNameLen returns how many bytes at the start of s form a bare
// (unbraced) shell variable reference, not counting the leading `$` (which
// must already be stripped by the caller). Returns 0 if s does not start
// with a valid name character or single-char special parameter.
//
// Matches POSIX bare-form references: `$NAME` (alpha/underscore-led name),
// `$1` (positional digit, single-char only without braces), `$@`, `$*`,
// `$#`, `$?`, `$!`, `$$`, `$-`.
func bareEnvNameLen(s string) int {
	if len(s) == 0 {
		return 0
	}
	c := s[0]
	// Single-digit positional ($0..$9 — multi-digit needs braces in POSIX).
	if c >= '0' && c <= '9' {
		return 1
	}
	// Single-char special parameters.
	switch c {
	case '@', '*', '#', '?', '!', '$', '-':
		return 1
	}
	// Identifier: [A-Za-z_][A-Za-z0-9_]*
	if !isNameStart(c) {
		return 0
	}
	n := 1
	for n < len(s) && isNameCont(s[n]) {
		n++
	}
	return n
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isNameCont(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}

// isShellPositionalOrSpecial reports whether name refers to a shell
// positional parameter ($0, $1, ${10}, ...) or special parameter
// ($@, $*, $#, $?, $!, $$, $-) and should therefore be preserved verbatim
// rather than expanded against the process environment.
func isShellPositionalOrSpecial(name string) bool {
	if name == "" {
		return false
	}
	// Purely numeric → positional parameter.
	allDigits := true
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return true
	}
	if len(name) == 1 {
		switch name[0] {
		case '@', '*', '#', '?', '!', '$', '-':
			return true
		}
	}
	return false
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

// GetContext returns a named context by name
func (c *Config) GetContext(name string) (*NamedContext, error) {
	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			return &c.Contexts[i], nil
		}
	}
	return nil, fmt.Errorf("context %q not found", name)
}

// GetToken retrieves a token by reference name.
// It first tries the OS keyring (checking both regular and OAuth tokens),
// then file-based OAuth token storage, then falls back to the config file.
func (c *Config) GetToken(tokenRef string) (string, error) {
	// Try keyring first
	if IsKeyringAvailable() {
		ts := NewTokenStore()

		// First check for OAuth token.
		// Current format: oauth:<env>:<tokenRef>
		// Legacy format:  oauth:<tokenRef>
		for _, keyringName := range c.oauthKeyringNames(tokenRef) {
			oauthToken, err := ts.GetToken(keyringName)
			if err != nil || oauthToken == "" {
				continue
			}

			var tokenData struct {
				AccessToken string `json:"access_token"`
			}
			if err := json.Unmarshal([]byte(oauthToken), &tokenData); err == nil && tokenData.AccessToken != "" {
				return tokenData.AccessToken, nil
			}
		}

		// Fall back to regular token
		token, err := ts.GetToken(tokenRef)
		if err == nil && token != "" {
			return token, nil
		}
	}

	// Try file-based OAuth token storage (for headless/WSL environments)
	if !IsKeyringAvailable() || IsFileTokenStorage() {
		fileStore := NewOAuthFileStore()
		for _, keyringName := range c.oauthKeyringNames(tokenRef) {
			oauthToken, err := fileStore.GetToken(keyringName)
			if err != nil || oauthToken == "" {
				continue
			}

			var tokenData struct {
				AccessToken string `json:"access_token"`
			}
			if err := json.Unmarshal([]byte(oauthToken), &tokenData); err == nil && tokenData.AccessToken != "" {
				return tokenData.AccessToken, nil
			}
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

func (c *Config) oauthKeyringNames(tokenRef string) []string {
	addCandidate := func(list []string, seen map[string]struct{}, key string) []string {
		if key == "" {
			return list
		}
		if _, exists := seen[key]; exists {
			return list
		}
		seen[key] = struct{}{}
		return append(list, key)
	}

	seen := make(map[string]struct{})
	var candidates []string

	// Prefer environment-specific entries from matching contexts.
	for _, nc := range c.Contexts {
		if nc.Context.TokenRef != tokenRef {
			continue
		}
		env := oauthEnvironmentFromURL(nc.Context.Environment)
		if env != "" {
			candidates = addCandidate(candidates, seen, fmt.Sprintf("oauth:%s:%s", env, tokenRef))
		}
	}

	// Also check all known environment prefixes to support shared token refs.
	for _, env := range []string{"prod", "dev", "hard"} {
		candidates = addCandidate(candidates, seen, fmt.Sprintf("oauth:%s:%s", env, tokenRef))
	}

	return candidates
}

func oauthEnvironmentFromURL(environmentURL string) string {
	url := strings.ToLower(environmentURL)

	if strings.Contains(url, "dev.apps.dynatracelabs.com") {
		return "dev"
	}
	if strings.Contains(url, "sprint.apps.dynatracelabs.com") {
		return "hard"
	}
	if strings.Contains(url, "apps.dynatrace.com") {
		return "prod"
	}

	return ""
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

// GetPreApplyHook returns the effective pre-apply hook command.
// Per-context hooks take precedence over global (preferences) hooks.
// The special value "none" explicitly disables the global hook for a context.
func (c *Config) GetPreApplyHook() string {
	// Hooks from an auto-discovered local config are untrusted and never run.
	if c.IsLocal() {
		return ""
	}
	// Per-context hook wins
	if ctx, err := c.CurrentContextObj(); err == nil {
		if ctx.Hooks.PreApply != "" {
			if ctx.Hooks.PreApply == "none" {
				return "" // explicitly disabled
			}
			return ctx.Hooks.PreApply
		}
	}
	// Fall back to global
	return c.Preferences.Hooks.PreApply
}

// GetPostApplyHook returns the effective post-apply hook command.
// Per-context hooks take precedence over global (preferences) hooks.
// The special value "none" explicitly disables the global hook for a context.
func (c *Config) GetPostApplyHook() string {
	// Hooks from an auto-discovered local config are untrusted and never run.
	if c.IsLocal() {
		return ""
	}
	if ctx, err := c.CurrentContextObj(); err == nil {
		if ctx.Hooks.PostApply != "" {
			if ctx.Hooks.PostApply == "none" {
				return ""
			}
			return ctx.Hooks.PostApply
		}
	}
	return c.Preferences.Hooks.PostApply
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

// PruneEmptyEnvironments removes contexts whose names are in placeholderNames,
// except the named keepContext. Pass the context names from the raw (unexpanded)
// config file to avoid pruning contexts backed by currently-unset env vars.
func (c *Config) PruneEmptyEnvironments(keepContext string, placeholderNames map[string]bool) {
	kept := c.Contexts[:0]
	for _, nc := range c.Contexts {
		if placeholderNames[nc.Name] && nc.Name != keepContext {
			continue
		}
		kept = append(kept, nc)
	}
	c.Contexts = kept
}

// SetToken creates or updates a token.
// If keyring is available, the token is stored securely in the OS keyring
// and only a reference is kept in the config file.
// Any cached OAuth tokens for this credential name are invalidated so that
// a rotated platform token does not keep using a stale refresh token.
func (c *Config) SetToken(name, token string) error {
	return c.setTokenWithKeyring(name, token, nil, nil)
}

// setTokenWithKeyring is the testable core of SetToken; accepts an explicit
// keyringBackend and OAuthFileStore so tests avoid the OS keyring.
func (c *Config) setTokenWithKeyring(name, token string, kr keyringBackend, fileStore *OAuthFileStore) error {
	if kr == nil {
		kr = newOSKeyring()
	}
	if fileStore == nil {
		fileStore = NewOAuthFileStore()
	}

	keyringAvailable := kr.Available()
	if keyringAvailable {
		if err := kr.Set(name, token); err != nil {
			return fmt.Errorf("failed to store token in keyring: %w", err)
		}
		// Store empty token in config (reference only)
		token = ""
	}

	// Invalidate cached OAuth tokens for all known environments.
	// When a platform token is rotated, the old OAuth refresh token
	// is no longer valid and must not be reused.
	// Both keyring and file-based caches are cleared, because GetToken
	// checks both backends.
	// Deletion is best-effort: a failure here means the user will get
	// an invalid_grant error on the next request and must re-authenticate,
	// which is acceptable.
	for _, key := range c.oauthKeyringNames(name) {
		if keyringAvailable {
			_ = kr.Delete(key)
		}
		_ = fileStore.DeleteToken(key)
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
