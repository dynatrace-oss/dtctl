package config

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestNewTokenStore(t *testing.T) {
	ts := NewTokenStore()
	if ts == nil {
		t.Fatal("NewTokenStore() returned nil")
	}
	if !ts.fallbackToFile {
		t.Error("fallbackToFile should be true by default")
	}
}

func TestKeyringBackend(t *testing.T) {
	backend := KeyringBackend()
	if backend == "" {
		t.Error("KeyringBackend() returned empty string")
	}
	// Should return a descriptive string based on OS
	validBackends := []string{
		"macOS Keychain",
		"Secret Service (libsecret)",
		"Windows Credential Manager",
		"OS Keyring",
	}
	found := false
	for _, valid := range validBackends {
		if backend == valid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("KeyringBackend() = %v, not a recognized backend", backend)
	}
}

func TestCheckKeyring_Disabled(t *testing.T) {
	t.Setenv(EnvDisableKeyring, "1")

	err := CheckKeyring()
	if err == nil {
		t.Fatal("CheckKeyring() should return error when keyring is disabled")
	}
	if !strings.Contains(err.Error(), EnvDisableKeyring) {
		t.Errorf("error should mention %s, got: %v", EnvDisableKeyring, err)
	}
}

func TestCheckKeyring_ReturnsNilOrError(t *testing.T) {
	// Smoke test: the function should not panic regardless of environment.
	// In CI (no keyring) it returns an error; on a desktop it may return nil.
	err := CheckKeyring()
	if err != nil {
		t.Logf("CheckKeyring() returned error (expected in CI): %v", err)
	}
}

func TestIsKeyringAvailable_MatchesCheckKeyring(t *testing.T) {
	available := IsKeyringAvailable()
	err := CheckKeyring()
	if available != (err == nil) {
		t.Errorf("IsKeyringAvailable()=%v but CheckKeyring() returned %v", available, err)
	}
}

func TestEnsureKeyringCollection_Smoke(t *testing.T) {
	// EnsureKeyringCollection requires D-Bus and Secret Service.
	// In most test environments these are unavailable, so the function
	// should return an error without panicking.
	err := EnsureKeyringCollection(context.Background())
	if err == nil {
		return // D-Bus available (desktop environment) — nothing more to assert
	}
	t.Logf("EnsureKeyringCollection() error (expected in CI): %v", err)

	if runtime.GOOS == "linux" {
		// On Linux the Secret Service may be unavailable in two ways:
		//   1. D-Bus session is unreachable → "cannot connect to Secret Service"
		//   2. D-Bus is reachable but org.freedesktop.secrets is not registered
		//      (common in headless CI) → error contains "org.freedesktop.secrets"
		//      or "failed to create keyring collection"
		secretServiceErr := strings.Contains(err.Error(), "cannot connect to Secret Service") ||
			strings.Contains(err.Error(), "org.freedesktop.secrets") ||
			strings.Contains(err.Error(), "failed to create keyring collection")
		if !secretServiceErr {
			t.Errorf("expected Secret Service unavailability error, got: %v", err)
		}
	} else if !strings.Contains(err.Error(), "only supported on Linux") {
		// On non-Linux platforms the stub should report the OS.
		t.Errorf("expected 'only supported on Linux' error, got: %v", err)
	}
}

func TestEnsureKeyringCollection_RespectsContext(t *testing.T) {
	// Verify that a cancelled context is honoured (the function should
	// return promptly rather than blocking).
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := EnsureKeyringCollection(ctx)
	if err == nil {
		// If D-Bus is available and the collection already exists the
		// function returns nil before reaching the poll loop — acceptable.
		return
	}
	// The error is either the context cancellation or the usual D-Bus
	// / platform error — both are fine.
	t.Logf("EnsureKeyringCollection(cancelled) error: %v", err)
}

func TestGetTokenWithFallback(t *testing.T) {
	cfg := NewConfig()
	_ = cfg.SetToken("file-token", "file-secret")

	// Should fall back to config file when keyring unavailable or token not in keyring
	token, err := GetTokenWithFallback(cfg, "file-token")
	if err != nil {
		t.Fatalf("GetTokenWithFallback() error = %v", err)
	}
	if token != "file-secret" {
		t.Errorf("GetTokenWithFallback() = %v, want file-secret", token)
	}

	// Non-existing token should error
	_, err = GetTokenWithFallback(cfg, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existing token")
	}
}

func TestMigrateTokensToKeyring_NoKeyring(t *testing.T) {
	cfg := NewConfig()
	_ = cfg.SetToken("test-token", "secret")

	// If keyring is not available, migration should fail gracefully
	if !IsKeyringAvailable() {
		_, err := MigrateTokensToKeyring(cfg)
		if err == nil {
			t.Error("Expected error when keyring not available")
		}
	}
}

func TestTokenStore_DeleteToken_KeyringUnavailableIsNoop(t *testing.T) {
	t.Setenv(EnvDisableKeyring, "1")
	ts := NewTokenStore()
	if err := ts.DeleteToken("definitely-missing-token"); err != nil {
		t.Fatalf("DeleteToken() error = %v, want nil", err)
	}
}
