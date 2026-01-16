package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()

	if cfg.APIVersion != "v1" {
		t.Errorf("APIVersion = %v, want v1", cfg.APIVersion)
	}
	if cfg.Kind != "Config" {
		t.Errorf("Kind = %v, want Config", cfg.Kind)
	}
	if len(cfg.Contexts) != 0 {
		t.Errorf("Contexts should be empty, got %d", len(cfg.Contexts))
	}
	if len(cfg.Tokens) != 0 {
		t.Errorf("Tokens should be empty, got %d", len(cfg.Tokens))
	}
	if cfg.Preferences.Output != "table" {
		t.Errorf("Preferences.Output = %v, want table", cfg.Preferences.Output)
	}
	if cfg.Preferences.Editor != "vim" {
		t.Errorf("Preferences.Editor = %v, want vim", cfg.Preferences.Editor)
	}
}

func TestConfig_SetContext(t *testing.T) {
	cfg := NewConfig()

	// Add new context
	cfg.SetContext("dev", "https://dev.dynatrace.com", "dev-token")

	if len(cfg.Contexts) != 1 {
		t.Fatalf("Expected 1 context, got %d", len(cfg.Contexts))
	}
	if cfg.Contexts[0].Name != "dev" {
		t.Errorf("Context name = %v, want dev", cfg.Contexts[0].Name)
	}
	if cfg.Contexts[0].Context.Environment != "https://dev.dynatrace.com" {
		t.Errorf("Environment = %v, want https://dev.dynatrace.com", cfg.Contexts[0].Context.Environment)
	}

	// Update existing context
	cfg.SetContext("dev", "https://dev2.dynatrace.com", "")

	if len(cfg.Contexts) != 1 {
		t.Fatalf("Expected 1 context after update, got %d", len(cfg.Contexts))
	}
	if cfg.Contexts[0].Context.Environment != "https://dev2.dynatrace.com" {
		t.Errorf("Updated environment = %v, want https://dev2.dynatrace.com", cfg.Contexts[0].Context.Environment)
	}
	// Token should remain unchanged when empty string passed
	if cfg.Contexts[0].Context.TokenRef != "dev-token" {
		t.Errorf("TokenRef should remain dev-token, got %v", cfg.Contexts[0].Context.TokenRef)
	}
}

func TestConfig_SetToken(t *testing.T) {
	cfg := NewConfig()

	// Add new token
	err := cfg.SetToken("my-token", "secret-value")
	if err != nil {
		t.Fatalf("SetToken() error = %v", err)
	}

	if len(cfg.Tokens) != 1 {
		t.Fatalf("Expected 1 token, got %d", len(cfg.Tokens))
	}
	if cfg.Tokens[0].Name != "my-token" {
		t.Errorf("Token name = %v, want my-token", cfg.Tokens[0].Name)
	}
	// Token may be empty if keyring is available (stored there instead)
	if !IsKeyringAvailable() && cfg.Tokens[0].Token != "secret-value" {
		t.Errorf("Token value = %v, want secret-value", cfg.Tokens[0].Token)
	}

	// Update existing token
	err = cfg.SetToken("my-token", "new-secret")
	if err != nil {
		t.Fatalf("SetToken() update error = %v", err)
	}

	if len(cfg.Tokens) != 1 {
		t.Fatalf("Expected 1 token after update, got %d", len(cfg.Tokens))
	}
}

func TestConfig_GetToken(t *testing.T) {
	cfg := NewConfig()
	_ = cfg.SetToken("existing", "token-value")

	tests := []struct {
		name     string
		tokenRef string
		want     string
		wantErr  bool
	}{
		{
			name:     "existing token",
			tokenRef: "existing",
			want:     "token-value",
			wantErr:  false,
		},
		{
			name:     "non-existing token",
			tokenRef: "missing",
			want:     "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cfg.GetToken(tt.tokenRef)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_MustGetToken(t *testing.T) {
	cfg := NewConfig()
	_ = cfg.SetToken("existing", "token-value")

	// Existing token
	if got := cfg.MustGetToken("existing"); got != "token-value" {
		t.Errorf("MustGetToken() = %v, want token-value", got)
	}

	// Non-existing token returns empty string
	if got := cfg.MustGetToken("missing"); got != "" {
		t.Errorf("MustGetToken() for missing = %v, want empty", got)
	}
}

func TestConfig_CurrentContextObj(t *testing.T) {
	cfg := NewConfig()
	cfg.SetContext("prod", "https://prod.dynatrace.com", "prod-token")

	// No current context set
	_, err := cfg.CurrentContextObj()
	if err == nil {
		t.Error("Expected error when no current context set")
	}

	// Set current context
	cfg.CurrentContext = "prod"
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		t.Fatalf("CurrentContextObj() error = %v", err)
	}
	if ctx.Environment != "https://prod.dynatrace.com" {
		t.Errorf("Environment = %v, want https://prod.dynatrace.com", ctx.Environment)
	}

	// Non-existing current context
	cfg.CurrentContext = "nonexistent"
	_, err = cfg.CurrentContextObj()
	if err == nil {
		t.Error("Expected error for non-existing current context")
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "dtctl-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config")

	// Create and save config
	cfg := NewConfig()
	cfg.SetContext("test", "https://test.dynatrace.com", "test-token")
	_ = cfg.SetToken("test-token", "secret123")
	cfg.CurrentContext = "test"

	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("SaveTo() error = %v", err)
	}

	// Verify file permissions (Unix-like systems only)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(configPath)
		if err != nil {
			t.Fatalf("Failed to stat config file: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("Config file permissions = %v, want 0600", info.Mode().Perm())
		}
	}

	// Load config
	loaded, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if loaded.CurrentContext != "test" {
		t.Errorf("Loaded CurrentContext = %v, want test", loaded.CurrentContext)
	}
	if len(loaded.Contexts) != 1 {
		t.Fatalf("Loaded contexts count = %d, want 1", len(loaded.Contexts))
	}
	if loaded.Contexts[0].Context.Environment != "https://test.dynatrace.com" {
		t.Errorf("Loaded environment = %v", loaded.Contexts[0].Context.Environment)
	}
}

func TestLoadFrom_NotFound(t *testing.T) {
	_, err := LoadFrom("/nonexistent/path/config")
	if err == nil {
		t.Error("Expected error for non-existent config file")
	}
}

func TestLoadFrom_InvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dtctl-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0600); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	_, err = LoadFrom(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()
	if dir == "" {
		t.Error("ConfigDir() returned empty string")
	}
}

func TestCacheDir(t *testing.T) {
	dir := CacheDir()
	if dir == "" {
		t.Error("CacheDir() returned empty string")
	}
}

func TestDataDir(t *testing.T) {
	dir := DataDir()
	if dir == "" {
		t.Error("DataDir() returned empty string")
	}
}

func TestConfig_MultipleContexts(t *testing.T) {
	cfg := NewConfig()

	cfg.SetContext("dev", "https://dev.dt.com", "dev-token")
	cfg.SetContext("staging", "https://staging.dt.com", "staging-token")
	cfg.SetContext("prod", "https://prod.dt.com", "prod-token")

	if len(cfg.Contexts) != 3 {
		t.Errorf("Expected 3 contexts, got %d", len(cfg.Contexts))
	}

	// Switch contexts
	cfg.CurrentContext = "staging"
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		t.Fatalf("CurrentContextObj() error = %v", err)
	}
	if ctx.Environment != "https://staging.dt.com" {
		t.Errorf("Wrong context environment: %v", ctx.Environment)
	}
}

func TestConfig_Save(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "dtctl-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override XDG for this test
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", origXDG)

	cfg := NewConfig()
	cfg.SetContext("test", "https://test.dt.com", "token")

	// Save should work (creates directory if needed)
	err = cfg.Save()
	if err != nil {
		t.Errorf("Save() error = %v", err)
	}
}

func TestConfig_Load(t *testing.T) {
	// Create temp directory with config
	tmpDir, err := os.MkdirTemp("", "dtctl-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config directory and file
	configDir := filepath.Join(tmpDir, "dtctl")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configContent := `apiVersion: v1
kind: Config
current-context: test
contexts:
  - name: test
    context:
      environment: https://test.dt.com
      token-ref: test-token
tokens:
  - name: test-token
    token: secret123
`
	configPath := filepath.Join(configDir, "config")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Use LoadFrom directly instead of Load() to avoid XDG caching issues
	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if cfg.CurrentContext != "test" {
		t.Errorf("CurrentContext = %v, want test", cfg.CurrentContext)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if path == "" {
		t.Error("DefaultConfigPath() returned empty string")
	}
}

func TestSaveTo_CreateDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dtctl-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Path with non-existent subdirectory
	configPath := filepath.Join(tmpDir, "subdir", "nested", "config")

	cfg := NewConfig()
	err = cfg.SaveTo(configPath)
	if err != nil {
		t.Fatalf("SaveTo() error = %v", err)
	}

	// Verify directory was created with correct permissions (Unix-like systems only)
	if runtime.GOOS != "windows" {
		dirInfo, err := os.Stat(filepath.Dir(configPath))
		if err != nil {
			t.Fatalf("Failed to stat directory: %v", err)
		}
		if dirInfo.Mode().Perm() != 0700 {
			t.Errorf("Directory permissions = %v, want 0700", dirInfo.Mode().Perm())
		}
	}
}

// Safety Level Tests

func TestSafetyLevel_IsValid(t *testing.T) {
	tests := []struct {
		level SafetyLevel
		valid bool
	}{
		{SafetyLevelReadOnly, true},
		{SafetyLevelReadWriteMine, true},
		{SafetyLevelReadWriteAll, true},
		{SafetyLevelDangerouslyUnrestricted, true},
		{"", true}, // Empty is valid (uses default)
		{"invalid", false},
		{"read-only", false},  // Old name, no longer valid
		{"read-write", false}, // Old name, no longer valid
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			if got := tt.level.IsValid(); got != tt.valid {
				t.Errorf("SafetyLevel(%q).IsValid() = %v, want %v", tt.level, got, tt.valid)
			}
		})
	}
}

func TestSafetyLevel_String(t *testing.T) {
	tests := []struct {
		level SafetyLevel
		want  string
	}{
		{SafetyLevelReadOnly, "readonly"},
		{SafetyLevelReadWriteMine, "readwrite-mine"},
		{SafetyLevelReadWriteAll, "readwrite-all"},
		{SafetyLevelDangerouslyUnrestricted, "dangerously-unrestricted"},
		{"", "readwrite-all"}, // Empty returns default
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("SafetyLevel(%q).String() = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestValidSafetyLevels(t *testing.T) {
	levels := ValidSafetyLevels()

	if len(levels) != 4 {
		t.Errorf("ValidSafetyLevels() returned %d levels, want 4", len(levels))
	}

	// Verify all returned levels are valid
	for _, level := range levels {
		if !level.IsValid() {
			t.Errorf("ValidSafetyLevels() returned invalid level: %s", level)
		}
	}

	// Verify expected levels are present
	expected := map[SafetyLevel]bool{
		SafetyLevelReadOnly:                false,
		SafetyLevelReadWriteMine:           false,
		SafetyLevelReadWriteAll:            false,
		SafetyLevelDangerouslyUnrestricted: false,
	}
	for _, level := range levels {
		expected[level] = true
	}
	for level, found := range expected {
		if !found {
			t.Errorf("ValidSafetyLevels() missing level: %s", level)
		}
	}
}

func TestContext_GetEffectiveSafetyLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    SafetyLevel
		expected SafetyLevel
	}{
		{"explicit readonly", SafetyLevelReadOnly, SafetyLevelReadOnly},
		{"explicit readwrite-mine", SafetyLevelReadWriteMine, SafetyLevelReadWriteMine},
		{"explicit readwrite-all", SafetyLevelReadWriteAll, SafetyLevelReadWriteAll},
		{"explicit unrestricted", SafetyLevelDangerouslyUnrestricted, SafetyLevelDangerouslyUnrestricted},
		{"empty defaults to readwrite-all", "", SafetyLevelReadWriteAll},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				Environment: "https://test.dt.com",
				SafetyLevel: tt.level,
			}
			if got := ctx.GetEffectiveSafetyLevel(); got != tt.expected {
				t.Errorf("GetEffectiveSafetyLevel() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_SetContextWithOptions(t *testing.T) {
	cfg := NewConfig()

	opts := &ContextOptions{
		SafetyLevel: SafetyLevelReadOnly,
		Description: "Production read-only access",
	}

	cfg.SetContextWithOptions("prod", "https://prod.dt.com", "prod-token", opts)

	if len(cfg.Contexts) != 1 {
		t.Fatalf("Expected 1 context, got %d", len(cfg.Contexts))
	}

	ctx := cfg.Contexts[0].Context
	if ctx.SafetyLevel != SafetyLevelReadOnly {
		t.Errorf("SafetyLevel = %v, want %v", ctx.SafetyLevel, SafetyLevelReadOnly)
	}
	if ctx.Description != "Production read-only access" {
		t.Errorf("Description = %v, want 'Production read-only access'", ctx.Description)
	}

	// Update with new options
	opts2 := &ContextOptions{
		SafetyLevel: SafetyLevelReadWriteAll,
	}
	cfg.SetContextWithOptions("prod", "https://prod.dt.com", "", opts2)

	if len(cfg.Contexts) != 1 {
		t.Fatalf("Expected 1 context after update, got %d", len(cfg.Contexts))
	}

	ctx = cfg.Contexts[0].Context
	if ctx.SafetyLevel != SafetyLevelReadWriteAll {
		t.Errorf("Updated SafetyLevel = %v, want %v", ctx.SafetyLevel, SafetyLevelReadWriteAll)
	}
	// Description should remain unchanged when not provided in update
	if ctx.Description != "Production read-only access" {
		t.Errorf("Description should remain unchanged, got %v", ctx.Description)
	}
}

func TestConfig_SetContextWithOptions_NilOpts(t *testing.T) {
	cfg := NewConfig()

	// SetContextWithOptions with nil opts should work like SetContext
	cfg.SetContextWithOptions("test", "https://test.dt.com", "test-token", nil)

	if len(cfg.Contexts) != 1 {
		t.Fatalf("Expected 1 context, got %d", len(cfg.Contexts))
	}

	ctx := cfg.Contexts[0].Context
	if ctx.SafetyLevel != "" {
		t.Errorf("SafetyLevel should be empty (use default), got %v", ctx.SafetyLevel)
	}
	if ctx.GetEffectiveSafetyLevel() != SafetyLevelReadWriteAll {
		t.Errorf("GetEffectiveSafetyLevel() = %v, want %v", ctx.GetEffectiveSafetyLevel(), SafetyLevelReadWriteAll)
	}
}

func TestFindLocalConfig(t *testing.T) {
	// Create a temp directory hierarchy
	tmpDir, err := os.MkdirTemp("", "dtctl-local-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create nested directory structure: tmpDir/project/subdir/nested
	projectDir := filepath.Join(tmpDir, "project")
	subDir := filepath.Join(projectDir, "subdir")
	nestedDir := filepath.Join(subDir, "nested")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dirs: %v", err)
	}

	// Test 1: No local config exists
	result := findLocalConfigFrom(nestedDir)
	if result != "" {
		t.Errorf("findLocalConfigFrom() should return empty when no config exists, got %q", result)
	}

	// Test 2: Create .dtctl.yaml in project root
	localConfigPath := filepath.Join(projectDir, LocalConfigName)
	configContent := `apiVersion: v1
kind: Config
current-context: local-test
contexts:
  - name: local-test
    context:
      environment: https://local.dt.com
      token-ref: local-token
`
	if err := os.WriteFile(localConfigPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write local config: %v", err)
	}

	// Test 3: Find config from project dir
	result = findLocalConfigFrom(projectDir)
	if result != localConfigPath {
		t.Errorf("findLocalConfigFrom(projectDir) = %q, want %q", result, localConfigPath)
	}

	// Test 4: Find config from nested subdir (walks up to project dir)
	result = findLocalConfigFrom(nestedDir)
	if result != localConfigPath {
		t.Errorf("findLocalConfigFrom(nestedDir) = %q, want %q", result, localConfigPath)
	}

	// Test 5: Config in subdir takes precedence
	subConfigPath := filepath.Join(subDir, LocalConfigName)
	if err := os.WriteFile(subConfigPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write subdir config: %v", err)
	}

	result = findLocalConfigFrom(nestedDir)
	if result != subConfigPath {
		t.Errorf("findLocalConfigFrom(nestedDir) with subdir config = %q, want %q", result, subConfigPath)
	}

	// Test 6: Starting from root should not find config
	result = findLocalConfigFrom("/")
	if result != "" {
		t.Errorf("findLocalConfigFrom('/') should return empty, got %q", result)
	}
}

func TestLocalConfigName(t *testing.T) {
	if LocalConfigName != ".dtctl.yaml" {
		t.Errorf("LocalConfigName = %q, want .dtctl.yaml", LocalConfigName)
	}
}

func TestConfig_DeleteContext(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() *Config
		contextName string
		wantErr     bool
		wantCount   int
	}{
		{
			name: "delete existing context",
			setup: func() *Config {
				cfg := NewConfig()
				cfg.SetContext("dev", "https://dev.dt.com", "dev-token")
				cfg.SetContext("prod", "https://prod.dt.com", "prod-token")
				return cfg
			},
			contextName: "dev",
			wantErr:     false,
			wantCount:   1,
		},
		{
			name: "delete non-existing context",
			setup: func() *Config {
				cfg := NewConfig()
				cfg.SetContext("dev", "https://dev.dt.com", "dev-token")
				return cfg
			},
			contextName: "nonexistent",
			wantErr:     true,
			wantCount:   1,
		},
		{
			name: "delete only context",
			setup: func() *Config {
				cfg := NewConfig()
				cfg.SetContext("only", "https://only.dt.com", "only-token")
				return cfg
			},
			contextName: "only",
			wantErr:     false,
			wantCount:   0,
		},
		{
			name: "delete from empty config",
			setup: func() *Config {
				return NewConfig()
			},
			contextName: "any",
			wantErr:     true,
			wantCount:   0,
		},
		{
			name: "delete middle context",
			setup: func() *Config {
				cfg := NewConfig()
				cfg.SetContext("first", "https://first.dt.com", "first-token")
				cfg.SetContext("middle", "https://middle.dt.com", "middle-token")
				cfg.SetContext("last", "https://last.dt.com", "last-token")
				return cfg
			},
			contextName: "middle",
			wantErr:     false,
			wantCount:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setup()
			err := cfg.DeleteContext(tt.contextName)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(cfg.Contexts) != tt.wantCount {
				t.Errorf("After DeleteContext(), context count = %d, want %d", len(cfg.Contexts), tt.wantCount)
			}

			// Verify the deleted context is actually gone
			if !tt.wantErr {
				for _, nc := range cfg.Contexts {
					if nc.Name == tt.contextName {
						t.Errorf("Context %q should have been deleted but still exists", tt.contextName)
					}
				}
			}
		})
	}
}
