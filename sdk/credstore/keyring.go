package credstore

import (
	"fmt"
	"os"
	"runtime"

	"github.com/zalando/go-keyring"
)

const (
	// DefaultService is the default keyring service name.
	DefaultService = "dtctl"

	// EnvDisableKeyring can be set to disable keyring integration.
	EnvDisableKeyring = "DTCTL_DISABLE_KEYRING"

	// ErrMsgCollectionUnlock is the error substring returned by the Secret Service
	// when a persistent keyring collection cannot be unlocked.
	ErrMsgCollectionUnlock = "failed to unlock correct collection"
)

// KeyringStore stores credentials in the OS keyring.
type KeyringStore struct {
	service string
}

// NewKeyringStore creates a keyring store with the given service name.
// If service is empty, DefaultService is used.
func NewKeyringStore(service string) *KeyringStore {
	if service == "" {
		service = DefaultService
	}
	return &KeyringStore{service: service}
}

// Get retrieves a credential from the OS keyring.
func (s *KeyringStore) Get(name string) (string, error) {
	token, err := keyring.Get(s.service, name)
	if err == keyring.ErrNotFound {
		return "", fmt.Errorf("credential %q not found in keyring", name)
	}
	if err != nil {
		return "", fmt.Errorf("failed to retrieve credential from keyring: %w", err)
	}
	return token, nil
}

// Set stores a credential in the OS keyring.
func (s *KeyringStore) Set(name, value string) error {
	if err := keyring.Set(s.service, name, value); err != nil {
		return fmt.Errorf("failed to store credential in keyring: %w", err)
	}
	return nil
}

// Delete removes a credential from the OS keyring.
func (s *KeyringStore) Delete(name string) error {
	err := keyring.Delete(s.service, name)
	if err == keyring.ErrNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to delete credential from keyring: %w", err)
	}
	return nil
}

// CheckKeyring probes the OS keyring and returns nil if it is usable,
// or a descriptive error explaining why it is not.
func CheckKeyring() error {
	return CheckKeyringForService(DefaultService)
}

// CheckKeyringForService probes the OS keyring for a specific service.
func CheckKeyringForService(service string) error {
	if os.Getenv(EnvDisableKeyring) != "" {
		return fmt.Errorf("keyring disabled via %s environment variable", EnvDisableKeyring)
	}
	_, err := keyring.Get(service, "__probe__")
	if err == nil || err == keyring.ErrNotFound {
		return nil
	}
	return fmt.Errorf("keyring probe failed: %w", err)
}

// IsKeyringAvailable reports whether the OS keyring is usable.
func IsKeyringAvailable() bool {
	return CheckKeyring() == nil
}

// KeyringBackend returns a human-readable description of the keyring backend.
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
