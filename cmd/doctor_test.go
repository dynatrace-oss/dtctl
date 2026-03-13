package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

func TestDoctorCheckVersionAlwaysPasses(t *testing.T) {
	results := runDoctorChecks()
	if len(results) == 0 {
		t.Fatal("expected at least one result (version check)")
	}
	if results[0].Name != "dtctl version" {
		t.Errorf("expected first check to be 'dtctl version', got %q", results[0].Name)
	}
	if results[0].Status != "ok" {
		t.Errorf("expected version check to pass, got status %q", results[0].Status)
	}
}

func TestDoctorNoConfig(t *testing.T) {
	tmpDir := t.TempDir()

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	// Point to a non-existent config
	cfgFile = filepath.Join(tmpDir, "nonexistent", "config")

	results := runDoctorChecks()

	// Should have version (ok) and config (fail), then stop
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[1].Name != "Configuration" {
		t.Errorf("expected second check to be 'Configuration', got %q", results[1].Name)
	}
	if results[1].Status != "fail" {
		t.Errorf("expected config check to fail, got %q", results[1].Status)
	}
	// Should stop after config failure
	if len(results) > 2 {
		t.Errorf("expected checks to stop after config failure, got %d results", len(results))
	}
}

func TestDoctorNoCurrentContext(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = configPath

	// Create config with no current context
	cfg := config.NewConfig()
	cfg.SetContext("dev", "https://dev.example.com", "dev-token")
	cfg.CurrentContext = "" // Explicitly no current context
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	results := runDoctorChecks()

	// Should have version, config, then context (fail)
	found := false
	for _, r := range results {
		if r.Name == "Current context" {
			found = true
			if r.Status != "fail" {
				t.Errorf("expected context check to fail, got %q", r.Status)
			}
			if !strings.Contains(r.Detail, "no current context") {
				t.Errorf("expected detail about no current context, got %q", r.Detail)
			}
		}
	}
	if !found {
		t.Error("expected 'Current context' check in results")
	}
}

func TestDoctorValidConfigNoToken(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = configPath

	// Create config with context but no token
	cfg := config.NewConfig()
	cfg.SetContext("dev", "https://dev.example.com", "dev-token")
	cfg.CurrentContext = "dev"
	// Don't set a token - it will fail at token retrieval
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	results := runDoctorChecks()

	// Should have version, config, context, then token (fail)
	found := false
	for _, r := range results {
		if r.Name == "Token" {
			found = true
			if r.Status != "fail" {
				t.Errorf("expected token check to fail, got %q", r.Status)
			}
		}
	}
	if !found {
		t.Error("expected 'Token' check in results")
	}
}

func TestDoctorFullPassingFlow(t *testing.T) {
	// Set up mock server that handles HEAD and user metadata
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/platform/metadata/v1/user":
			resp := map[string]interface{}{
				"userId":       "test-user-id",
				"emailAddress": "test@example.com",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = configPath

	cfg := config.NewConfig()
	cfg.SetContext("test", server.URL, "test-token")
	if err := cfg.SetToken("test-token", "dt0c01.ST.test-token-value.test-secret"); err != nil {
		t.Fatalf("failed to set token: %v", err)
	}
	cfg.CurrentContext = "test"
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	results := runDoctorChecks()

	// Should have at least version, config, context, token, connectivity
	if len(results) < 5 {
		t.Fatalf("expected at least 5 results, got %d", len(results))
	}

	// Check early checks pass
	for _, r := range results {
		switch r.Name {
		case "dtctl version":
			if r.Status != "ok" {
				t.Errorf("version check: expected ok, got %q: %s", r.Status, r.Detail)
			}
		case "Configuration":
			if r.Status != "ok" {
				t.Errorf("config check: expected ok, got %q: %s", r.Status, r.Detail)
			}
		case "Current context":
			if r.Status != "ok" {
				t.Errorf("context check: expected ok, got %q: %s", r.Status, r.Detail)
			}
		case "Token":
			if r.Status != "ok" {
				t.Errorf("token check: expected ok, got %q: %s", r.Status, r.Detail)
			}
		case "Connectivity":
			if r.Status != "ok" {
				t.Errorf("connectivity check: expected ok, got %q: %s", r.Status, r.Detail)
			}
		}
	}
}

func TestDoctorConnectivityFailure(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = configPath

	// Start a server then immediately close it so the port refuses connections fast
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := srv.URL
	srv.Close()

	cfg := config.NewConfig()
	cfg.SetContext("test", unreachableURL, "test-token")
	if err := cfg.SetToken("test-token", "dt0c01.ST.test-token-value.test-secret"); err != nil {
		t.Fatalf("failed to set token: %v", err)
	}
	cfg.CurrentContext = "test"
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Use a very short timeout so the test doesn't wait on any OS-level delay
	fastClient := &http.Client{Timeout: 100 * time.Millisecond}
	results := runDoctorChecksWithClient(fastClient)

	found := false
	for _, r := range results {
		if r.Name == "Connectivity" {
			found = true
			if r.Status != "fail" {
				t.Errorf("expected connectivity check to fail, got %q", r.Status)
			}
			if !strings.Contains(r.Detail, "cannot reach") {
				t.Errorf("expected detail about unreachable, got %q", r.Detail)
			}
		}
	}
	if !found {
		t.Error("expected 'Connectivity' check in results")
	}
}

func TestDoctorCmdReturnsErrorOnFailure(t *testing.T) {
	tmpDir := t.TempDir()

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = filepath.Join(tmpDir, "nonexistent", "config")

	err := doctorCmd.RunE(doctorCmd, []string{})
	if err == nil {
		t.Fatal("expected doctor command to return error when checks fail")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected error to mention failure, got %q", err.Error())
	}
}

func TestPrintDoctorResults(t *testing.T) {
	// Just verify it doesn't panic
	results := []checkResult{
		{Name: "Test", Status: "ok", Detail: "all good"},
		{Name: "Test2", Status: "warn", Detail: "warning"},
		{Name: "Test3", Status: "fail", Detail: "failure"},
	}
	printDoctorResults(results)
}
