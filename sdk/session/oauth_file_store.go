package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// oauthTokenDir is the subdirectory under DataDir where OAuth tokens are stored.
	oauthTokenDir = "oauth-tokens"

	// oauthTokenFileMode is the file permission for OAuth token files (owner-only read/write).
	oauthTokenFileMode = 0600

	// oauthTokenDirMode is the directory permission for the OAuth tokens directory.
	oauthTokenDirMode = 0700
)

// OAuthFileStore provides file-based storage for OAuth tokens.
// Tokens are stored as individual JSON files under $XDG_DATA_HOME/dtctl/oauth-tokens/,
// with 0600 permissions (owner-only read/write).
//
// This is used as a fallback when the OS keyring is unavailable (e.g. headless Linux,
// WSL, CI/CD environments, containers).
type OAuthFileStore struct {
	// dir overrides the default directory for testing.
	// When empty, oauthTokensDir() is used.
	dir string
}

// NewOAuthFileStore creates a new file-based OAuth token store.
func NewOAuthFileStore() *OAuthFileStore {
	return &OAuthFileStore{}
}

// NewOAuthFileStoreWithDir creates a file store using a specific directory (for testing).
func NewOAuthFileStoreWithDir(dir string) *OAuthFileStore {
	return &OAuthFileStore{dir: dir}
}

// SetToken writes a token to a file.
func (fs *OAuthFileStore) SetToken(name, token string) error {
	dir := fs.tokenDir()

	if err := os.MkdirAll(dir, oauthTokenDirMode); err != nil {
		return fmt.Errorf("failed to create OAuth token directory: %w", err)
	}

	path := filepath.Join(dir, sanitizeTokenName(name)+".json")
	if err := os.WriteFile(path, []byte(token), oauthTokenFileMode); err != nil {
		return fmt.Errorf("failed to write OAuth token file: %w", err)
	}
	return nil
}

// GetToken reads a token from a file.
func (fs *OAuthFileStore) GetToken(name string) (string, error) {
	path := filepath.Join(fs.tokenDir(), sanitizeTokenName(name)+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("token %q not found in file store", name)
		}
		return "", fmt.Errorf("failed to read OAuth token file: %w", err)
	}
	return string(data), nil
}

// DeleteToken removes a token file.
func (fs *OAuthFileStore) DeleteToken(name string) error {
	path := filepath.Join(fs.tokenDir(), sanitizeTokenName(name)+".json")

	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete OAuth token file: %w", err)
	}
	return nil
}

// tokenDir returns the directory where tokens are stored.
func (fs *OAuthFileStore) tokenDir() string {
	if fs.dir != "" {
		return fs.dir
	}
	return oauthTokensDir()
}

// oauthTokensDir returns the default directory for file-based OAuth token storage.
func oauthTokensDir() string {
	return filepath.Join(DataDir(), oauthTokenDir)
}

// sanitizeTokenName converts a keyring-style token name (e.g. "oauth:prod:my-token")
// into a safe filename by replacing colons with double-underscores.
// filepath.Base is applied to prevent path traversal (e.g. "../" in the name).
func sanitizeTokenName(name string) string {
	safe := strings.ReplaceAll(name, ":", "__")
	return filepath.Base(safe)
}
