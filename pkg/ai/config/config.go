package aiconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AIConfig holds AI-related configuration stored in ~/.dtctl/ai.yml.
// All fields can be overridden by environment variables named DTCTL_AI_<FIELD_UPPERCASE>.
type AIConfig struct {
	OpenAIBaseURL       string `yaml:"openai_baseurl"`
	OpenAIToken         string `yaml:"openai_token"`
	OpenRouterBaseURL   string `yaml:"openrouter_baseurl"`
	OpenRouterToken     string `yaml:"openrouter_token"`
	DeepSeekBaseURL     string `yaml:"deepseek_baseurl"`
	DeepSeekToken       string `yaml:"deepseek_token"`
	AnthropicBaseURL    string `yaml:"anthropic_baseurl"`
	AnthropicToken      string `yaml:"anthropic_token"`
	GoogleBaseURL       string `yaml:"google_baseurl"`
	GoogleToken         string `yaml:"google_token"`
	MistralBaseURL      string `yaml:"mistral_baseurl"`
	MistralToken        string `yaml:"mistral_token"`
	GitLabBaseURL       string `yaml:"gitlab_baseurl"`
	GitLabToken         string `yaml:"gitlab_token"`
	GitLabWebhookSecret string `yaml:"gitlab_webhook_secret"`
	AgentName           string `yaml:"agent_name"`
	DefaultProvider     string `yaml:"default_provider"`
	DefaultModel        string `yaml:"default_model"`
}

// configPath returns the path to the AI config file: ~/.dtctl/ai.yml
func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".dtctl", "ai.yml")
}

// Load reads the AI config file and applies environment variable overrides.
// If the file does not exist, an empty config is returned with env overrides applied.
func Load() (*AIConfig, error) {
	cfg := &AIConfig{}

	data, err := os.ReadFile(configPath())
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading AI config: %w", err)
	}

	if len(data) > 0 {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing AI config: %w", err)
		}
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

// loadRaw reads the config file without applying env overrides (used by set/get/ls commands).
func loadRaw() (*AIConfig, error) {
	cfg := &AIConfig{}

	data, err := os.ReadFile(configPath())
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading AI config: %w", err)
	}

	if len(data) > 0 {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing AI config: %w", err)
		}
	}

	applyDefaultURLs(cfg)
	return cfg, nil
}

// Save writes the config to ~/.dtctl/ai.yml, creating the directory if needed.
func Save(cfg *AIConfig) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serializing AI config: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

// Set sets a single key in the config file.
func Set(key, value string) error {
	cfg, err := loadRaw()
	if err != nil {
		return err
	}

	if err := setField(cfg, key, value); err != nil {
		return err
	}

	return Save(cfg)
}

// Get returns the value for a key, applying env overrides.
func Get(key string) (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}

	val, err := getField(cfg, key)
	if err != nil {
		return "", err
	}

	return val, nil
}

// ListAll returns all key=value pairs from the config (with env overrides applied).
// Token values are masked.
func ListAll() (map[string]string, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(fields))
	for _, f := range fields {
		val := *f.ptr(cfg)
		if f.IsToken {
			val = maskToken(val)
		}
		result[f.Key] = val
	}
	return result, nil
}

// ValidKeys returns all valid configuration key names.
func ValidKeys() []string {
	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = f.Key
	}
	return keys
}

// ConfigFilePath returns the path to the config file.
func ConfigFilePath() string {
	return configPath()
}

// Default values for provider base URLs.
const (
	DefaultOpenAIBaseURL     = "https://api.openai.com/v1"
	DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"
	DefaultDeepSeekBaseURL   = "https://api.deepseek.com/v1"
	DefaultAnthropicBaseURL  = "https://api.anthropic.com/v1"
	DefaultGoogleBaseURL     = "https://generativelanguage.googleapis.com/v1beta"
	DefaultMistralBaseURL    = "https://api.mistral.ai/v1"
	DefaultGitLabBaseURL     = "https://gitlab.com"
)

// fieldSpec is the single source of truth for every config key.
// Adding a new provider or field requires only one entry here.
type fieldSpec struct {
	Key        string                  // yaml key name (also used as CLI flag name)
	ptr        func(*AIConfig) *string // pointer into AIConfig for get/set
	DefaultURL string                  // non-empty for baseurl fields; used as default when blank
	IsToken    bool                    // true → value is masked in ls output
}

// fields drives ValidKeys, setField, getField, applyEnvOverrides, applyDefaultURLs,
// and ListAll — no key name appears more than once.
var fields = []fieldSpec{
	{Key: "openai_baseurl", ptr: func(c *AIConfig) *string { return &c.OpenAIBaseURL }, DefaultURL: DefaultOpenAIBaseURL},
	{Key: "openai_token", ptr: func(c *AIConfig) *string { return &c.OpenAIToken }, IsToken: true},
	{Key: "openrouter_baseurl", ptr: func(c *AIConfig) *string { return &c.OpenRouterBaseURL }, DefaultURL: DefaultOpenRouterBaseURL},
	{Key: "openrouter_token", ptr: func(c *AIConfig) *string { return &c.OpenRouterToken }, IsToken: true},
	{Key: "deepseek_baseurl", ptr: func(c *AIConfig) *string { return &c.DeepSeekBaseURL }, DefaultURL: DefaultDeepSeekBaseURL},
	{Key: "deepseek_token", ptr: func(c *AIConfig) *string { return &c.DeepSeekToken }, IsToken: true},
	{Key: "anthropic_baseurl", ptr: func(c *AIConfig) *string { return &c.AnthropicBaseURL }, DefaultURL: DefaultAnthropicBaseURL},
	{Key: "anthropic_token", ptr: func(c *AIConfig) *string { return &c.AnthropicToken }, IsToken: true},
	{Key: "google_baseurl", ptr: func(c *AIConfig) *string { return &c.GoogleBaseURL }, DefaultURL: DefaultGoogleBaseURL},
	{Key: "google_token", ptr: func(c *AIConfig) *string { return &c.GoogleToken }, IsToken: true},
	{Key: "mistral_baseurl", ptr: func(c *AIConfig) *string { return &c.MistralBaseURL }, DefaultURL: DefaultMistralBaseURL},
	{Key: "mistral_token", ptr: func(c *AIConfig) *string { return &c.MistralToken }, IsToken: true},
	{Key: "gitlab_baseurl", ptr: func(c *AIConfig) *string { return &c.GitLabBaseURL }, DefaultURL: DefaultGitLabBaseURL},
	{Key: "gitlab_token", ptr: func(c *AIConfig) *string { return &c.GitLabToken }, IsToken: true},
	{Key: "gitlab_webhook_secret", ptr: func(c *AIConfig) *string { return &c.GitLabWebhookSecret }, IsToken: true},
	{Key: "agent_name", ptr: func(c *AIConfig) *string { return &c.AgentName }},
	{Key: "default_provider", ptr: func(c *AIConfig) *string { return &c.DefaultProvider }},
	{Key: "default_model", ptr: func(c *AIConfig) *string { return &c.DefaultModel }},
}

// findField looks up a spec by key (case-insensitive). Returns nil if not found.
func findField(key string) *fieldSpec {
	key = strings.ToLower(strings.TrimSpace(key))
	for i := range fields {
		if fields[i].Key == key {
			return &fields[i]
		}
	}
	return nil
}

// Accessors — return the effective value (file config + env override) with defaults applied.

func GetOpenAIBaseURL() string { return effectiveURL("openai_baseurl", DefaultOpenAIBaseURL) }
func GetOpenAIAPIKey() string  { return effectiveToken("openai_token") }
func GetOpenRouterBaseURL() string {
	return effectiveURL("openrouter_baseurl", DefaultOpenRouterBaseURL)
}
func GetOpenRouterAPIKey() string    { return effectiveToken("openrouter_token") }
func GetDeepSeekBaseURL() string     { return effectiveURL("deepseek_baseurl", DefaultDeepSeekBaseURL) }
func GetDeepSeekAPIKey() string      { return effectiveToken("deepseek_token") }
func GetAnthropicBaseURL() string    { return effectiveURL("anthropic_baseurl", DefaultAnthropicBaseURL) }
func GetAnthropicAPIKey() string     { return effectiveToken("anthropic_token") }
func GetGeminiBaseURL() string       { return effectiveURL("google_baseurl", DefaultGoogleBaseURL) }
func GetGeminiAPIKey() string        { return effectiveToken("google_token") }
func GetMistralBaseURL() string      { return effectiveURL("mistral_baseurl", DefaultMistralBaseURL) }
func GetMistralAPIKey() string       { return effectiveToken("mistral_token") }
func GetGitLabBaseURL() string       { return effectiveURL("gitlab_baseurl", DefaultGitLabBaseURL) }
func GetGitLabToken() string         { return effectiveToken("gitlab_token") }
func GetGitLabWebhookSecret() string { return effectiveToken("gitlab_webhook_secret") }

func GetAgentName() string {
	cfg, _ := Load()
	if cfg != nil && strings.TrimSpace(cfg.AgentName) != "" {
		return cfg.AgentName
	}
	return "dtctl-agent"
}

func GetDefaultAiProvider() string {
	cfg, _ := Load()
	if cfg != nil && strings.TrimSpace(cfg.DefaultProvider) != "" {
		return cfg.DefaultProvider
	}
	return "openai"
}

func GetDefaultAiModel() string {
	cfg, _ := Load()
	if cfg != nil {
		return cfg.DefaultModel
	}
	return ""
}

// ─── internal helpers ────────────────────────────────────────────────────────

// envKey converts a config key to its DTCTL_AI_ environment variable name.
func envKey(key string) string {
	return "DTCTL_AI_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
}

// applyDefaultURLs fills in empty baseurl fields with their canonical defaults.
// Called by loadRaw so that defaults are written to the file on the next save.
func applyDefaultURLs(cfg *AIConfig) {
	for _, f := range fields {
		if f.DefaultURL != "" && *f.ptr(cfg) == "" {
			*f.ptr(cfg) = f.DefaultURL
		}
	}
}

// applyEnvOverrides overwrites config fields when DTCTL_AI_<KEY> env vars are set.
func applyEnvOverrides(cfg *AIConfig) {
	for _, f := range fields {
		if v := os.Getenv(envKey(f.Key)); v != "" {
			*f.ptr(cfg) = v
		}
	}
}

// effectiveURL returns the config value for a URL key; falls back to defaultVal if empty.
func effectiveURL(key, defaultVal string) string {
	cfg, _ := Load()
	val, _ := getField(cfg, key)
	if strings.TrimSpace(val) == "" {
		return defaultVal
	}
	return val
}

// effectiveToken returns the config value for a token key (empty string if not set).
func effectiveToken(key string) string {
	cfg, _ := Load()
	val, _ := getField(cfg, key)
	return val
}

// setField sets a field on the AIConfig by key name.
func setField(cfg *AIConfig, key, value string) error {
	f := findField(key)
	if f == nil {
		return fmt.Errorf("unknown AI config key %q — valid keys: %s", key, strings.Join(ValidKeys(), ", "))
	}
	*f.ptr(cfg) = value
	return nil
}

// getField returns a field value from the AIConfig by key name.
func getField(cfg *AIConfig, key string) (string, error) {
	if cfg == nil {
		return "", nil
	}
	f := findField(key)
	if f == nil {
		return "", fmt.Errorf("unknown AI config key %q — valid keys: %s", key, strings.Join(ValidKeys(), ", "))
	}
	return *f.ptr(cfg), nil
}

// maskToken masks all but the last 4 characters of a token for display.
func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(token)-4) + token[len(token)-4:]
}
