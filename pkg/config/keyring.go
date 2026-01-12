package config

import (
	"fmt"
	"os"
	"runtime"

	"github.com/zalando/go-keyring"
)

const (
	// KeyringService is the service name used for keyring storage
	KeyringService = "dtctl"

	// EnvDisableKeyring can be set to disable keyring integration
	EnvDisableKeyring = "DTCTL_DISABLE_KEYRING"
)

// TokenStore provides secure token storage using the OS keyring
type TokenStore struct {
	// fallbackToFile indicates whether to fall back to file-based storage
	// when keyring is unavailable
	fallbackToFile bool
}

// NewTokenStore creates a new token store
func NewTokenStore() *TokenStore {
	return &TokenStore{
		fallbackToFile: true,
	}
}

// IsKeyringAvailable checks if keyring storage is available on this system
func IsKeyringAvailable() bool {
	// Check if explicitly disabled
	if os.Getenv(EnvDisableKeyring) != "" {
		return false
	}

	// Test keyring availability by attempting a get operation
	// This will fail gracefully if keyring is not available
	_, err := keyring.Get(KeyringService, "__test__")
	// If error is "not found", keyring is available but key doesn't exist
	// If error is something else (like "keyring not available"), it's not available
	if err == keyring.ErrNotFound {
		return true
	}
	// Try to detect if it's a "keyring unavailable" type error
	if err != nil {
		errStr := err.Error()
		// Common error messages when keyring is unavailable
		if contains(errStr, "secret service", "dbus", "keychain", "credential") {
			return false
		}
	}
	return err == nil
}

// contains checks if s contains any of the substrings
func contains(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// SetToken stores a token securely in the OS keyring
func (ts *TokenStore) SetToken(name, token string) error {
	if !IsKeyringAvailable() {
		if ts.fallbackToFile {
			return nil // Will be handled by file-based storage
		}
		return fmt.Errorf("keyring not available and fallback disabled")
	}

	err := keyring.Set(KeyringService, name, token)
	if err != nil {
		return fmt.Errorf("failed to store token in keyring: %w", err)
	}
	return nil
}

// GetToken retrieves a token from the OS keyring
func (ts *TokenStore) GetToken(name string) (string, error) {
	if !IsKeyringAvailable() {
		return "", fmt.Errorf("keyring not available")
	}

	token, err := keyring.Get(KeyringService, name)
	if err == keyring.ErrNotFound {
		return "", fmt.Errorf("token %q not found in keyring", name)
	}
	if err != nil {
		return "", fmt.Errorf("failed to retrieve token from keyring: %w", err)
	}
	return token, nil
}

// DeleteToken removes a token from the OS keyring
func (ts *TokenStore) DeleteToken(name string) error {
	if !IsKeyringAvailable() {
		return nil // Nothing to delete
	}

	err := keyring.Delete(KeyringService, name)
	if err == keyring.ErrNotFound {
		return nil // Already deleted
	}
	if err != nil {
		return fmt.Errorf("failed to delete token from keyring: %w", err)
	}
	return nil
}

// MigrateTokensToKeyring migrates tokens from config file to keyring
// Returns the number of tokens migrated and any error
func MigrateTokensToKeyring(cfg *Config) (int, error) {
	if !IsKeyringAvailable() {
		return 0, fmt.Errorf("keyring not available")
	}

	ts := NewTokenStore()
	migrated := 0

	for i, nt := range cfg.Tokens {
		if nt.Token == "" {
			continue // Already migrated or empty
		}

		// Store in keyring
		if err := ts.SetToken(nt.Name, nt.Token); err != nil {
			return migrated, fmt.Errorf("failed to migrate token %q: %w", nt.Name, err)
		}

		// Clear from config (mark as migrated)
		cfg.Tokens[i].Token = ""
		migrated++
	}

	return migrated, nil
}

// GetTokenWithFallback tries to get a token from keyring first, then falls back to config
func GetTokenWithFallback(cfg *Config, tokenRef string) (string, error) {
	// Try keyring first
	if IsKeyringAvailable() {
		ts := NewTokenStore()
		token, err := ts.GetToken(tokenRef)
		if err == nil && token != "" {
			return token, nil
		}
	}

	// Fall back to config file
	return cfg.GetToken(tokenRef)
}

// KeyringBackend returns a string describing the keyring backend in use
func KeyringBackend() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS Keychain"
	case "linux":
		return "Secret Service (libsecret)"
	case "windows":
		return "Windows Credential Manager"
	default:
		return "OS Keyring"
	}
}
