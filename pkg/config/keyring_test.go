package config

import (
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

func TestContains(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		substrs []string
		want    bool
	}{
		{
			name:    "contains one",
			s:       "hello world",
			substrs: []string{"world"},
			want:    true,
		},
		{
			name:    "contains multiple check",
			s:       "secret service error",
			substrs: []string{"keychain", "secret service"},
			want:    true,
		},
		{
			name:    "contains none",
			s:       "some error",
			substrs: []string{"keychain", "dbus"},
			want:    false,
		},
		{
			name:    "empty string",
			s:       "",
			substrs: []string{"test"},
			want:    false,
		},
		{
			name:    "empty substrs",
			s:       "hello",
			substrs: []string{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.s, tt.substrs...); got != tt.want {
				t.Errorf("contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTokenWithFallback(t *testing.T) {
	cfg := NewConfig()
	cfg.SetToken("file-token", "file-secret")

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
	cfg.SetToken("test-token", "secret")

	// If keyring is not available, migration should fail gracefully
	if !IsKeyringAvailable() {
		_, err := MigrateTokensToKeyring(cfg)
		if err == nil {
			t.Error("Expected error when keyring not available")
		}
	}
}
