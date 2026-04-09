package config

import (
	"fmt"
	"os"
	"runtime"
	"time"

	dbus "github.com/godbus/dbus/v5"
	"github.com/zalando/go-keyring"
	ss "github.com/zalando/go-keyring/secret_service"
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

// CheckKeyring probes the OS keyring and returns nil if it is usable,
// or a descriptive error explaining why it is not.
func CheckKeyring() error {
	if os.Getenv(EnvDisableKeyring) != "" {
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

// EnsureKeyringCollection checks whether a usable Secret Service collection
// exists and, if not, creates a persistent "login" collection.
// On Linux/WSL gnome-keyring may start with only a transient "session"
// collection; this function creates the permanent one, which may trigger
// an OS password prompt.
func EnsureKeyringCollection() error {
	svc, err := ss.NewSecretService()
	if err != nil {
		return fmt.Errorf("cannot connect to Secret Service: %w", err)
	}

	// If the "login" collection already exists, nothing to do.
	loginPath := dbus.ObjectPath("/org/freedesktop/secrets/collection/login")
	if svc.CheckCollectionPath(loginPath) == nil {
		return nil
	}

	// Create a persistent collection via D-Bus with alias "default".
	// gnome-keyring only accepts the "default" alias. Using this alias
	// ensures GetLoginCollection() can discover the collection via its
	// fallback to /org/freedesktop/secrets/aliases/default.
	props := map[string]dbus.Variant{
		"org.freedesktop.Secret.Collection.Label": dbus.MakeVariant("Login"),
	}
	var collectionPath, promptPath dbus.ObjectPath
	obj := svc.Object("org.freedesktop.secrets", "/org/freedesktop/secrets")
	err = obj.Call("org.freedesktop.Secret.Service.CreateCollection", 0, props, "default").
		Store(&collectionPath, &promptPath)
	if err != nil {
		return fmt.Errorf("failed to create keyring collection: %w", err)
	}

	// If no prompt was returned, the collection was created immediately.
	if promptPath == dbus.ObjectPath("/") {
		return nil
	}

	// A prompt was returned — trigger it so the OS displays a password dialog.
	promptObj := svc.Object("org.freedesktop.secrets", promptPath)
	if err := promptObj.Call("org.freedesktop.Secret.Prompt.Prompt", 0, "").Err; err != nil {
		return fmt.Errorf("failed to trigger keyring prompt: %w", err)
	}

	// Poll until the default alias points to a real collection, indicating
	// the user completed the password prompt. D-Bus signal delivery is
	// unreliable in some environments (notably WSL), so polling is more
	// robust than waiting for the Prompt.Completed signal.
	deadline := time.After(2 * time.Minute)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("timed out waiting for keyring password prompt to complete")
		case <-ticker.C:
			var alias dbus.ObjectPath
			call := obj.Call("org.freedesktop.Secret.Service.ReadAlias", 0, "default")
			if call.Err == nil {
				_ = call.Store(&alias)
				if alias != "/" && alias != "" {
					return nil
				}
			}
		}
	}
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
