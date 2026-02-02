package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

func TestConfigSetCmd(t *testing.T) {
	// Create a temporary directory for the config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	// Set the config path
	if err := os.Setenv("XDG_CONFIG_HOME", tmpDir); err != nil {
		t.Fatalf("failed to set XDG_CONFIG_HOME: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("XDG_CONFIG_HOME")
	}()

	tests := []struct {
		name      string
		key       string
		value     string
		wantError bool
		validate  func(t *testing.T, cfg *config.Config)
	}{
		{
			name:      "set editor preference",
			key:       "preferences.editor",
			value:     "vim",
			wantError: false,
			validate: func(t *testing.T, cfg *config.Config) {
				if cfg.Preferences.Editor != "vim" {
					t.Errorf("expected editor to be 'vim', got %q", cfg.Preferences.Editor)
				}
			},
		},
		{
			name:      "set editor to micro",
			key:       "preferences.editor",
			value:     "micro",
			wantError: false,
			validate: func(t *testing.T, cfg *config.Config) {
				if cfg.Preferences.Editor != "micro" {
					t.Errorf("expected editor to be 'micro', got %q", cfg.Preferences.Editor)
				}
			},
		},
		{
			name:      "unknown key",
			key:       "unknown.key",
			value:     "value",
			wantError: true,
			validate:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up for fresh test
			_ = os.Remove(configPath)

			// Execute the RunE function directly with args
			err := configSetCmd.RunE(configSetCmd, []string{tt.key, tt.value})

			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Validate the result
			if tt.validate != nil {
				cfg, err := config.Load()
				if err != nil {
					t.Fatalf("failed to load config: %v", err)
				}
				tt.validate(t, cfg)
			}
		})
	}
}

func TestConfigDeleteContextCmd(t *testing.T) {
	// Create a temporary directory for the config file
	tmpDir := t.TempDir()

	// Set the config path
	if err := os.Setenv("XDG_CONFIG_HOME", tmpDir); err != nil {
		t.Fatalf("failed to set XDG_CONFIG_HOME: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("XDG_CONFIG_HOME")
	}()

	tests := []struct {
		name         string
		setupConfig  func() *config.Config
		contextName  string
		wantError    bool
		errorContain string
		validate     func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "delete existing context",
			setupConfig: func() *config.Config {
				cfg := config.NewConfig()
				cfg.SetContext("dev", "https://dev.dynatrace.com", "dev-token")
				cfg.SetContext("prod", "https://prod.dynatrace.com", "prod-token")
				cfg.CurrentContext = "dev"
				return cfg
			},
			contextName: "prod",
			wantError:   false,
			validate: func(t *testing.T, cfg *config.Config) {
				// Should only have one context left
				if len(cfg.Contexts) != 1 {
					t.Errorf("expected 1 context, got %d", len(cfg.Contexts))
				}
				// The remaining context should be dev
				if cfg.Contexts[0].Name != "dev" {
					t.Errorf("expected remaining context to be 'dev', got %q", cfg.Contexts[0].Name)
				}
				// Current context should still be dev
				if cfg.CurrentContext != "dev" {
					t.Errorf("expected current context to be 'dev', got %q", cfg.CurrentContext)
				}
			},
		},
		{
			name: "delete current context clears current-context",
			setupConfig: func() *config.Config {
				cfg := config.NewConfig()
				cfg.SetContext("dev", "https://dev.dynatrace.com", "dev-token")
				cfg.SetContext("prod", "https://prod.dynatrace.com", "prod-token")
				cfg.CurrentContext = "dev"
				return cfg
			},
			contextName: "dev",
			wantError:   false,
			validate: func(t *testing.T, cfg *config.Config) {
				// Should only have one context left
				if len(cfg.Contexts) != 1 {
					t.Errorf("expected 1 context, got %d", len(cfg.Contexts))
				}
				// Current context should be cleared
				if cfg.CurrentContext != "" {
					t.Errorf("expected current context to be cleared, got %q", cfg.CurrentContext)
				}
			},
		},
		{
			name: "delete non-existent context",
			setupConfig: func() *config.Config {
				cfg := config.NewConfig()
				cfg.SetContext("dev", "https://dev.dynatrace.com", "dev-token")
				cfg.CurrentContext = "dev"
				return cfg
			},
			contextName:  "nonexistent",
			wantError:    true,
			errorContain: "not found",
		},
		{
			name: "delete only context",
			setupConfig: func() *config.Config {
				cfg := config.NewConfig()
				cfg.SetContext("only", "https://only.dynatrace.com", "only-token")
				cfg.CurrentContext = "only"
				return cfg
			},
			contextName: "only",
			wantError:   false,
			validate: func(t *testing.T, cfg *config.Config) {
				// Should have no contexts left
				if len(cfg.Contexts) != 0 {
					t.Errorf("expected 0 contexts, got %d", len(cfg.Contexts))
				}
				// Current context should be cleared
				if cfg.CurrentContext != "" {
					t.Errorf("expected current context to be cleared, got %q", cfg.CurrentContext)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup config
			cfg := tt.setupConfig()
			if err := cfg.Save(); err != nil {
				t.Fatalf("failed to save config: %v", err)
			}

			// Execute the RunE function directly with args
			err := configDeleteContextCmd.RunE(configDeleteContextCmd, []string{tt.contextName})

			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				}
				if tt.errorContain != "" && !strings.Contains(err.Error(), tt.errorContain) {
					t.Errorf("expected error to contain %q, got %q", tt.errorContain, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Validate the result
			if tt.validate != nil {
				cfg, err := config.Load()
				if err != nil {
					t.Fatalf("failed to load config: %v", err)
				}
				tt.validate(t, cfg)
			}
		})
	}
}
