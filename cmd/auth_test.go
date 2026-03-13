package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// setupAuthTestConfig creates a temporary config with the given context and returns the path.
func setupAuthTestConfig(t *testing.T, contextName, environment, tokenRef string) string {
	t.Helper()
	t.Setenv("DTCTL_DISABLE_KEYRING", "1")
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := config.NewConfig()
	cfg.SetContext(contextName, environment, tokenRef)
	cfg.CurrentContext = contextName

	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}
	return configPath
}

// TestAuthLogin_FlagValidation checks that the login command fails correctly when
// neither flags nor a current context are available.
func TestAuthLogin_FlagValidation(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		setupConfig bool // whether to write a config with a current context
		wantErrSub  string
	}{
		{
			name:        "no flags no config errors helpfully",
			args:        []string{"auth", "login"},
			setupConfig: false,
			wantErrSub:  "--context and --environment are required",
		},
		{
			name:        "no flags empty current context errors helpfully",
			args:        []string{"auth", "login"},
			setupConfig: true, // config exists but CurrentContext is empty (handled below)
			wantErrSub:  "--context and --environment are required when no current context is set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()

			if tt.name == "no flags no config errors helpfully" {
				// Point to a non-existent config file
				tmpDir := t.TempDir()
				cfgFile = filepath.Join(tmpDir, "nonexistent.yaml")
			} else {
				// Config with no current context set
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "config.yaml")
				cfg := config.NewConfig()
				// Deliberately leave cfg.CurrentContext empty
				if err := cfg.SaveTo(configPath); err != nil {
					t.Fatalf("failed to save config: %v", err)
				}
				cfgFile = configPath
			}

			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if err == nil {
				t.Fatal("expected error but got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErrSub, err)
			}

			// Reset
			cfgFile = ""
		})
	}
}

// TestAuthLogin_CurrentContextFallback verifies that the login command derives
// context name, environment URL and token name from the active context when no
// flags are provided.  The test stops before the actual OAuth flow (keyring
// unavailable) but checks the context resolution logic produces the expected
// error – i.e. it gets past flag validation.
func TestAuthLogin_CurrentContextFallback(t *testing.T) {
	viper.Reset()

	const (
		ctxName  = "my-context"
		envURL   = "https://abc12345.apps.dynatrace.com"
		tokenRef = "my-context-oauth"
	)

	configPath := setupAuthTestConfig(t, ctxName, envURL, tokenRef)
	cfgFile = configPath
	defer func() { cfgFile = "" }()

	// Execute with no flags – should pass flag validation and reach the
	// keyring check (which fails in a test environment).
	rootCmd.SetArgs([]string{"auth", "login"})
	err := rootCmd.Execute()

	// We expect the command to fail, but NOT because of missing flags.
	// It should fail later (keyring unavailable or similar infrastructure error).
	if err == nil {
		// Unlikely in a unit test environment without a real keyring/browser,
		// but not a failure of the logic we are testing.
		return
	}

	if strings.Contains(err.Error(), "--context and --environment are required") {
		t.Errorf("expected current-context fallback to work, but got flag validation error: %v", err)
	}
}

// TestAuthLogin_PartialFlags verifies that supplying only --context (without
// --environment) causes the missing value to be filled from the current context.
func TestAuthLogin_PartialFlags_EnvironmentFromContext(t *testing.T) {
	viper.Reset()

	const (
		ctxName  = "my-context"
		envURL   = "https://abc12345.apps.dynatrace.com"
		tokenRef = "my-context-oauth"
	)

	configPath := setupAuthTestConfig(t, ctxName, envURL, tokenRef)
	cfgFile = configPath
	defer func() { cfgFile = "" }()

	rootCmd.SetArgs([]string{"auth", "login", "--context", ctxName})
	err := rootCmd.Execute()

	if err != nil && strings.Contains(err.Error(), "--context and --environment are required") {
		t.Errorf("expected environment to be filled from current context, but got: %v", err)
	}
}
