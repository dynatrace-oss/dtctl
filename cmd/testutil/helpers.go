package testutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// SetupTestConfig creates a temporary config file for testing and returns the config path and cleanup function
func SetupTestConfig(t *testing.T, serverURL string) (configPath string, cleanup func()) {
	t.Helper()

	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "config")

	cfg := config.NewConfig()
	cfg.SetContext("test", serverURL, "test-token")
	if err := cfg.SetToken("test-token", "dt0c01.ST.test-token-value.test-secret"); err != nil {
		t.Fatalf("failed to set token: %v", err)
	}
	cfg.CurrentContext = "test"

	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	cleanup = func() {
		_ = os.RemoveAll(tmpDir)
	}

	return configPath, cleanup
}

// ExecuteCommand executes a cobra command with args and returns output/error
func ExecuteCommand(t *testing.T, cmd *cobra.Command, args ...string) (output string, err error) {
	t.Helper()

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err = cmd.Execute()
	output = buf.String()
	return
}

// CreateTempFile creates a temporary file with content and returns the file path
func CreateTempFile(t *testing.T, content string, pattern string) string {
	t.Helper()

	if pattern == "" {
		pattern = "test-*.yaml"
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), pattern)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() {
		_ = tmpFile.Close()
	}()

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	return tmpFile.Name()
}

// ResetCommandFlags resets a command's flags to allow reuse in tests
func ResetCommandFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		flag.Changed = false
		_ = flag.Value.Set(flag.DefValue)
	})
}
