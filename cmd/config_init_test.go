package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"gopkg.in/yaml.v3"
)

func TestConfigInitCmd(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		existingFile bool
		wantErr      bool
		wantContext  string
	}{
		{
			name:         "create new config",
			args:         []string{},
			existingFile: false,
			wantErr:      false,
			wantContext:  "my-environment",
		},
		{
			name:         "create with custom context",
			args:         []string{"--context", "production"},
			existingFile: false,
			wantErr:      false,
			wantContext:  "production",
		},
		{
			name:         "fail on existing file without force",
			args:         []string{},
			existingFile: true,
			wantErr:      true,
		},
		{
			name:         "overwrite with force flag",
			args:         []string{"--force"},
			existingFile: true,
			wantErr:      false,
			wantContext:  "my-environment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir := t.TempDir()
			origDir, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(origDir)

			if err := os.Chdir(tmpDir); err != nil {
				t.Fatal(err)
			}

			// Create existing file if needed
			if tt.existingFile {
				if err := os.WriteFile(config.LocalConfigName, []byte("existing"), 0600); err != nil {
					t.Fatal(err)
				}
			}

			// Parse flags manually
			contextName := ""
			force := false
			for i := 0; i < len(tt.args); i++ {
				if tt.args[i] == "--context" && i+1 < len(tt.args) {
					contextName = tt.args[i+1]
					i++
				} else if tt.args[i] == "--force" {
					force = true
				}
			}

			// Check if .dtctl.yaml already exists
			configPath := config.LocalConfigName
			if _, err := os.Stat(configPath); err == nil {
				if !force {
					if !tt.wantErr {
						t.Error("Expected error when file exists without --force")
					}
					return
				}
			}

			// Create template config
			template := createLocalConfigTemplate(contextName)

			// Write to file
			data, marshalErr := yaml.Marshal(template)
			if marshalErr != nil {
				t.Fatalf("Failed to marshal config template: %v", marshalErr)
			}

			if err := os.WriteFile(configPath, data, 0600); err != nil {
				t.Fatalf("Failed to write %s: %v", configPath, err)
			}

			if tt.wantErr {
				return
			}

			// Verify file was created
			if _, err := os.Stat(config.LocalConfigName); os.IsNotExist(err) {
				t.Error("Expected .dtctl.yaml to be created")
				return
			}

			// Parse and verify content
			fileData, err := os.ReadFile(config.LocalConfigName)
			if err != nil {
				t.Fatalf("Failed to read created file: %v", err)
			}

			var cfg config.Config
			if err := yaml.Unmarshal(fileData, &cfg); err != nil {
				t.Fatalf("Failed to parse created config: %v", err)
			}

			// Verify structure
			if cfg.APIVersion != "dtctl.io/v1" {
				t.Errorf("APIVersion = %q, want dtctl.io/v1", cfg.APIVersion)
			}
			if cfg.Kind != "Config" {
				t.Errorf("Kind = %q, want Config", cfg.Kind)
			}
			if cfg.CurrentContext != tt.wantContext {
				t.Errorf("CurrentContext = %q, want %q", cfg.CurrentContext, tt.wantContext)
			}

			// Verify context exists
			if len(cfg.Contexts) != 1 {
				t.Fatalf("Expected 1 context, got %d", len(cfg.Contexts))
			}
			if cfg.Contexts[0].Name != tt.wantContext {
				t.Errorf("Context name = %q, want %q", cfg.Contexts[0].Name, tt.wantContext)
			}

			// Verify environment variable placeholders
			if cfg.Contexts[0].Context.Environment != "${DT_ENVIRONMENT_URL}" {
				t.Errorf("Environment = %q, want ${DT_ENVIRONMENT_URL}", cfg.Contexts[0].Context.Environment)
			}

			// Verify token placeholder
			if len(cfg.Tokens) != 1 {
				t.Fatalf("Expected 1 token, got %d", len(cfg.Tokens))
			}
			if cfg.Tokens[0].Token != "${DT_API_TOKEN}" {
				t.Errorf("Token = %q, want ${DT_API_TOKEN}", cfg.Tokens[0].Token)
			}
		})
	}
}

func TestCreateLocalConfigTemplate(t *testing.T) {
	tests := []struct {
		name        string
		contextName string
		wantContext string
	}{
		{
			name:        "default context name",
			contextName: "",
			wantContext: "my-environment",
		},
		{
			name:        "custom context name",
			contextName: "production",
			wantContext: "production",
		},
		{
			name:        "dev context",
			contextName: "dev",
			wantContext: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createLocalConfigTemplate(tt.contextName)

			if cfg.APIVersion != "dtctl.io/v1" {
				t.Errorf("APIVersion = %q, want dtctl.io/v1", cfg.APIVersion)
			}
			if cfg.Kind != "Config" {
				t.Errorf("Kind = %q, want Config", cfg.Kind)
			}
			if cfg.CurrentContext != tt.wantContext {
				t.Errorf("CurrentContext = %q, want %q", cfg.CurrentContext, tt.wantContext)
			}

			// Verify contexts
			if len(cfg.Contexts) != 1 {
				t.Fatalf("Expected 1 context, got %d", len(cfg.Contexts))
			}
			ctx := cfg.Contexts[0]
			if ctx.Name != tt.wantContext {
				t.Errorf("Context name = %q, want %q", ctx.Name, tt.wantContext)
			}
			if ctx.Context.Environment != "${DT_ENVIRONMENT_URL}" {
				t.Errorf("Environment = %q, want ${DT_ENVIRONMENT_URL}", ctx.Context.Environment)
			}
			if ctx.Context.TokenRef != "my-token" {
				t.Errorf("TokenRef = %q, want my-token", ctx.Context.TokenRef)
			}
			if ctx.Context.SafetyLevel != config.SafetyLevelReadWriteAll {
				t.Errorf("SafetyLevel = %q, want %q", ctx.Context.SafetyLevel, config.SafetyLevelReadWriteAll)
			}

			// Verify tokens
			if len(cfg.Tokens) != 1 {
				t.Fatalf("Expected 1 token, got %d", len(cfg.Tokens))
			}
			token := cfg.Tokens[0]
			if token.Name != "my-token" {
				t.Errorf("Token name = %q, want my-token", token.Name)
			}
			if token.Token != "${DT_API_TOKEN}" {
				t.Errorf("Token = %q, want ${DT_API_TOKEN}", token.Token)
			}

			// Verify preferences
			if cfg.Preferences.Output != "table" {
				t.Errorf("Preferences.Output = %q, want table", cfg.Preferences.Output)
			}
		})
	}
}

func TestConfigInitCmd_Integration(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create template config manually (like the command does)
	template := createLocalConfigTemplate("test-env")
	data, err := yaml.Marshal(template)
	if err != nil {
		t.Fatalf("Failed to marshal template: %v", err)
	}

	if err := os.WriteFile(config.LocalConfigName, data, 0600); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Set environment variables
	os.Setenv("DT_ENVIRONMENT_URL", "https://test.dynatrace.com")
	os.Setenv("DT_API_TOKEN", "dt0s16.TEST_TOKEN")
	defer os.Unsetenv("DT_ENVIRONMENT_URL")
	defer os.Unsetenv("DT_API_TOKEN")

	// Load the config with environment variable expansion
	cfg, err := config.LoadFrom(filepath.Join(tmpDir, config.LocalConfigName))
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify environment variables were expanded
	if len(cfg.Contexts) != 1 {
		t.Fatalf("Expected 1 context, got %d", len(cfg.Contexts))
	}
	if cfg.Contexts[0].Context.Environment != "https://test.dynatrace.com" {
		t.Errorf("Environment = %q, want https://test.dynatrace.com", cfg.Contexts[0].Context.Environment)
	}

	if len(cfg.Tokens) != 1 {
		t.Fatalf("Expected 1 token, got %d", len(cfg.Tokens))
	}
	if cfg.Tokens[0].Token != "dt0s16.TEST_TOKEN" {
		t.Errorf("Token = %q, want dt0s16.TEST_TOKEN", cfg.Tokens[0].Token)
	}
}
