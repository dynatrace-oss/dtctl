package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// setupCtxTestConfig creates a temp config with contexts for testing.
func setupCtxTestConfig(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	xdg.Reload()
	t.Cleanup(func() { xdg.Reload() })

	cfg := config.NewConfig()
	cfg.SetContext("dev", "https://dev.example.com", "dev-token")
	cfg.SetContext("prod", "https://prod.example.com", "prod-token")
	cfg.CurrentContext = "dev"

	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	return tmpDir
}

func TestCtxListContexts(t *testing.T) {
	setupCtxTestConfig(t)

	// Save and restore cfgFile
	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = ""

	err := ctxCmd.RunE(ctxCmd, []string{})
	if err != nil {
		t.Fatalf("ctx (list) failed: %v", err)
	}
}

func TestCtxSwitchContext(t *testing.T) {
	setupCtxTestConfig(t)

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = ""

	// Switch to prod
	err := ctxCmd.RunE(ctxCmd, []string{"prod"})
	if err != nil {
		t.Fatalf("ctx prod failed: %v", err)
	}

	// Verify switch happened
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.CurrentContext != "prod" {
		t.Errorf("expected current context 'prod', got %q", cfg.CurrentContext)
	}
}

func TestCtxSwitchNonExistent(t *testing.T) {
	setupCtxTestConfig(t)

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = ""

	err := ctxCmd.RunE(ctxCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for non-existent context, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to contain 'not found', got %q", err.Error())
	}
}

func TestCtxCurrentCmd(t *testing.T) {
	setupCtxTestConfig(t)

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = ""

	err := ctxCurrentCmd.RunE(ctxCurrentCmd, []string{})
	if err != nil {
		t.Fatalf("ctx current failed: %v", err)
	}
}

func TestCtxDescribeCmd(t *testing.T) {
	setupCtxTestConfig(t)

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = ""

	t.Run("describe existing context", func(t *testing.T) {
		err := ctxDescribeCmd.RunE(ctxDescribeCmd, []string{"dev"})
		if err != nil {
			t.Fatalf("ctx describe dev failed: %v", err)
		}
	})

	t.Run("describe non-existent context", func(t *testing.T) {
		err := ctxDescribeCmd.RunE(ctxDescribeCmd, []string{"nonexistent"})
		if err == nil {
			t.Fatal("expected error for non-existent context, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected error to contain 'not found', got %q", err.Error())
		}
	})
}

func TestCtxSetCmd(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = configPath

	t.Run("create new context", func(t *testing.T) {
		_ = ctxSetCmd.Flags().Set("environment", "https://staging.example.com")
		_ = ctxSetCmd.Flags().Set("token-ref", "staging-token")
		_ = ctxSetCmd.Flags().Set("safety-level", "readonly")
		_ = ctxSetCmd.Flags().Set("description", "Staging environment")
		defer func() {
			_ = ctxSetCmd.Flags().Set("environment", "")
			_ = ctxSetCmd.Flags().Set("token-ref", "")
			_ = ctxSetCmd.Flags().Set("safety-level", "")
			_ = ctxSetCmd.Flags().Set("description", "")
		}()

		err := ctxSetCmd.RunE(ctxSetCmd, []string{"staging"})
		if err != nil {
			t.Fatalf("ctx set staging failed: %v", err)
		}

		// Verify context was created
		cfg, err := config.LoadFrom(configPath)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if len(cfg.Contexts) != 1 {
			t.Fatalf("expected 1 context, got %d", len(cfg.Contexts))
		}
		if cfg.Contexts[0].Name != "staging" {
			t.Errorf("expected context 'staging', got %q", cfg.Contexts[0].Name)
		}
		if cfg.Contexts[0].Context.Environment != "https://staging.example.com" {
			t.Errorf("expected environment 'https://staging.example.com', got %q", cfg.Contexts[0].Context.Environment)
		}
		if cfg.Contexts[0].Context.SafetyLevel != config.SafetyLevelReadOnly {
			t.Errorf("expected safety level 'readonly', got %q", cfg.Contexts[0].Context.SafetyLevel)
		}
		// First context should be set as current
		if cfg.CurrentContext != "staging" {
			t.Errorf("expected current context 'staging', got %q", cfg.CurrentContext)
		}
	})

	t.Run("create without environment fails", func(t *testing.T) {
		_ = ctxSetCmd.Flags().Set("environment", "")
		_ = ctxSetCmd.Flags().Set("token-ref", "")
		_ = ctxSetCmd.Flags().Set("safety-level", "")
		_ = ctxSetCmd.Flags().Set("description", "")

		// Clean config file to ensure no existing context
		cleanCfg := config.NewConfig()
		if err := cleanCfg.SaveTo(configPath); err != nil {
			t.Fatalf("failed to save clean config: %v", err)
		}

		err := ctxSetCmd.RunE(ctxSetCmd, []string{"new-ctx"})
		if err == nil {
			t.Fatal("expected error when creating context without environment")
		}
		if !strings.Contains(err.Error(), "--environment") {
			t.Errorf("expected error to mention --environment, got %q", err.Error())
		}
	})

	t.Run("invalid safety level", func(t *testing.T) {
		_ = ctxSetCmd.Flags().Set("environment", "https://test.example.com")
		_ = ctxSetCmd.Flags().Set("safety-level", "invalid-level")
		defer func() {
			_ = ctxSetCmd.Flags().Set("environment", "")
			_ = ctxSetCmd.Flags().Set("safety-level", "")
		}()

		err := ctxSetCmd.RunE(ctxSetCmd, []string{"bad-ctx"})
		if err == nil {
			t.Fatal("expected error for invalid safety level")
		}
		if !strings.Contains(err.Error(), "invalid safety level") {
			t.Errorf("expected error about invalid safety level, got %q", err.Error())
		}
	})
}

func TestCtxDeleteCmd(t *testing.T) {
	setupCtxTestConfig(t)

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = ""

	t.Run("delete non-current context", func(t *testing.T) {
		err := ctxDeleteCmd.RunE(ctxDeleteCmd, []string{"prod"})
		if err != nil {
			t.Fatalf("ctx delete prod failed: %v", err)
		}

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if len(cfg.Contexts) != 1 {
			t.Errorf("expected 1 context, got %d", len(cfg.Contexts))
		}
		// Current should still be dev
		if cfg.CurrentContext != "dev" {
			t.Errorf("expected current context 'dev', got %q", cfg.CurrentContext)
		}
	})

	t.Run("delete current context clears current-context", func(t *testing.T) {
		err := ctxDeleteCmd.RunE(ctxDeleteCmd, []string{"dev"})
		if err != nil {
			t.Fatalf("ctx delete dev failed: %v", err)
		}

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if cfg.CurrentContext != "" {
			t.Errorf("expected current context to be cleared, got %q", cfg.CurrentContext)
		}
	})

	t.Run("delete non-existent context", func(t *testing.T) {
		err := ctxDeleteCmd.RunE(ctxDeleteCmd, []string{"nonexistent"})
		if err == nil {
			t.Fatal("expected error for non-existent context")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected error to contain 'not found', got %q", err.Error())
		}
	})
}

func TestCtxWithCustomConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "custom-config.yaml")

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = configPath

	// Create config using ctx set
	_ = ctxSetCmd.Flags().Set("environment", "https://test.example.com")
	_ = ctxSetCmd.Flags().Set("token-ref", "test-token")
	defer func() {
		_ = ctxSetCmd.Flags().Set("environment", "")
		_ = ctxSetCmd.Flags().Set("token-ref", "")
	}()

	if err := ctxSetCmd.RunE(ctxSetCmd, []string{"test"}); err != nil {
		t.Fatalf("ctx set failed: %v", err)
	}

	// Verify it was written to custom path
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("config file was NOT created at %s", configPath)
	}

	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("failed to load config from custom path: %v", err)
	}
	if len(cfg.Contexts) != 1 || cfg.Contexts[0].Name != "test" {
		t.Errorf("expected context 'test' in custom config, got %+v", cfg.Contexts)
	}

	// Switch using ctx
	_ = ctxSetCmd.Flags().Set("environment", "https://other.example.com")
	if err := ctxSetCmd.RunE(ctxSetCmd, []string{"other"}); err != nil {
		t.Fatalf("ctx set other failed: %v", err)
	}

	if err := ctxCmd.RunE(ctxCmd, []string{"test"}); err != nil {
		t.Fatalf("ctx switch failed: %v", err)
	}

	cfg, err = config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if cfg.CurrentContext != "test" {
		t.Errorf("expected current context 'test', got %q", cfg.CurrentContext)
	}
}
