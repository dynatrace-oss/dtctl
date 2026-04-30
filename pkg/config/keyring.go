package config

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	// KeyringService is the service name used for keyring storage
	KeyringService = "dtctl"

	// EnvDisableKeyring can be set to disable keyring integration
	EnvDisableKeyring = "DTCTL_DISABLE_KEYRING"

	// EnvTokenStorage controls the OAuth token storage backend.
	// Set to "file" to use file-based storage instead of the OS keyring.
	// This is useful for headless Linux, WSL, CI/CD, and container environments
	// where a system keyring is not available.
	//
	// Valid values: "keyring" (default), "file"
	EnvTokenStorage = "DTCTL_TOKEN_STORAGE"

	// ErrMsgCollectionUnlock is the error substring returned by the Secret Service
	// backend when a persistent keyring collection does not exist or cannot be
	// unlocked. Centralised here so callers match on a single constant instead
	// of a fragile raw string.
	ErrMsgCollectionUnlock = "failed to unlock correct collection"
)

// keyringBackend abstracts secure credential storage so callers can be tested
// without a live OS keyring.
type keyringBackend interface {
	Available() bool
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
}

// osKeyring is the production implementation backed by the OS keyring.
type osKeyring struct{ store *TokenStore }

func newOSKeyring() *osKeyring                       { return &osKeyring{store: NewTokenStore()} }
func (k *osKeyring) Available() bool                 { return IsKeyringAvailable() }
func (k *osKeyring) Get(name string) (string, error) { return k.store.GetToken(name) }
func (k *osKeyring) Set(name, value string) error    { return k.store.SetToken(name, value) }
func (k *osKeyring) Delete(name string) error        { return k.store.DeleteToken(name) }

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

// isKeyringDisabled reports whether the keyring has been intentionally
// disabled via the DTCTL_DISABLE_KEYRING environment variable.
func isKeyringDisabled() bool {
	return os.Getenv(EnvDisableKeyring) != ""
}

// CheckKeyring probes the OS keyring and returns nil if it is usable,
// or a descriptive error explaining why it is not.
func CheckKeyring() error {
	if isKeyringDisabled() {
		return fmt.Errorf("keyring disabled via %s environment variable", EnvDisableKeyring)
	}

	_, err := keyring.Get(KeyringService, "__test__")
	if err == nil || err == keyring.ErrNotFound {
		return nil // keyring is reachable
	}
	return fmt.Errorf("keyring probe failed: %w", err)
}

// IsKeyringAvailable checks if keyring storage is available on this system
func IsKeyringAvailable() bool {
	return CheckKeyring() == nil
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

// IsFileTokenStorage reports whether the user has explicitly opted into
// file-based OAuth token storage via DTCTL_TOKEN_STORAGE=file.
func IsFileTokenStorage() bool {
	return strings.EqualFold(os.Getenv(EnvTokenStorage), "file")
}

// IsOAuthStorageAvailable reports whether OAuth tokens can be stored
// and retrieved — either via the OS keyring or file-based storage.
func IsOAuthStorageAvailable() bool {
	return IsKeyringAvailable() || IsFileTokenStorage()
}

// OAuthStorageBackend returns a human-readable label describing
// where OAuth tokens are (or will be) stored.
func OAuthStorageBackend() string {
	if IsFileTokenStorage() {
		return fmt.Sprintf("file (%s)", oauthTokensDir())
	}
	if IsKeyringAvailable() {
		return KeyringBackend()
	}
	// Fallback: file storage is used implicitly when keyring is unavailable
	// and the token manager falls back to file.
	return fmt.Sprintf("file (%s)", oauthTokensDir())
}
