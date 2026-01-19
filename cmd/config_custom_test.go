package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/spf13/viper"
)

func TestConfigFlagRespected(t *testing.T) {
	// 1. Setup separate directories for "default" config and "custom" config
	tmpDir := t.TempDir()
	defaultConfigDir := filepath.Join(tmpDir, "default")
	os.MkdirAll(defaultConfigDir, 0700)

	customConfigFile := filepath.Join(tmpDir, "custom", "custom-config.yaml")
	os.MkdirAll(filepath.Dir(customConfigFile), 0700)

	// Mock XDG_CONFIG_HOME to point to our temp default dir
	// This ensures valid Load() calls would go here if --config is ignored
	t.Setenv("XDG_CONFIG_HOME", defaultConfigDir)

	// Save original cfgFile value and restore after test
	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()

	// 2. Set the global cfgFile variable (simulating --config flag)
	cfgFile = customConfigFile

	// 3. Run config set-context command
	// validation: should create file at customConfigFile
	args := []string{"test-ctx"}
	cmd := configSetContextCmd

	// Reset flags to avoid interference
	cmd.Flags().Set("environment", "https://example.com")
	cmd.Flags().Set("token-ref", "my-token")
	cmd.Flags().Set("safety-level", "readonly")

	err := cmd.RunE(cmd, args)
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// 4. Verify custom config file was created
	if _, err := os.Stat(customConfigFile); os.IsNotExist(err) {
		t.Errorf("Custom config file was NOT created at %s", customConfigFile)
	}

	// 5. Verify default config was NOT created/touched
	defaultConfigPath := filepath.Join(defaultConfigDir, "dtctl", "config")
	if _, err := os.Stat(defaultConfigPath); err == nil {
		t.Errorf("Default config file SHOULD NOT exist at %s", defaultConfigPath)
	}

	// 6. Verify content of custom config
	cfg, err := config.LoadFrom(customConfigFile)
	if err != nil {
		t.Fatalf("Failed to load custom config: %v", err)
	}

	if cfg.CurrentContext != "test-ctx" {
		t.Errorf("Expected current-context 'test-ctx', got '%s'", cfg.CurrentContext)
	}

	// 7. Verify we can read it back using view command
	// Reset Viper to ensure it doesn't hold old state
	viper.Reset()

	// Capture stdout
	// (Simulated by just running the command and ensuring no error_
	viewErr := configViewCmd.RunE(configViewCmd, []string{})
	if viewErr != nil {
		t.Errorf("View command failed with custom config: %v", viewErr)
	}
}
