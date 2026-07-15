package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestTokenManager_FileStorageFallback verifies that when keyring is unavailable
// but file storage is available, token operations work through the file store.
func TestTokenManager_FileStorageFallback(t *testing.T) {
	dir := t.TempDir()
	fileStore := NewOAuthFileStoreWithDir(dir)

	oauthConfig := OAuthConfigForEnvironment(EnvironmentProd, DefaultSafetyLevel, nil)
	tm, err := NewTokenManager(oauthConfig)
	if err != nil {
		t.Fatalf("NewTokenManager() error: %v", err)
	}

	// Override deps: keyring unavailable, file store available
	tm.deps.keyringAvailable = func() bool { return false }
	tm.deps.fileStoreAvailable = func() bool { return true }
	tm.deps.fileGetToken = func(name string) (string, error) { return fileStore.GetToken(name) }
	tm.deps.fileSetToken = func(name, token string) error { return fileStore.SetToken(name, token) }
	tm.deps.fileDeleteToken = func(name string) error { return fileStore.DeleteToken(name) }

	tokenName := "test-file-token"
	expiresAt := time.Now().Add(1 * time.Hour).UTC()
	tokens := &TokenSet{
		AccessToken:  "file-access-token",
		RefreshToken: "file-refresh-token",
		IDToken:      "file-id-token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		Scope:        "openid",
		ExpiresAt:    expiresAt,
	}

	// Save token
	if err := tm.SaveToken(tokenName, tokens); err != nil {
		t.Fatalf("SaveToken() error: %v", err)
	}

	// Load token
	stored, err := tm.loadToken(tokenName)
	if err != nil {
		t.Fatalf("loadToken() error: %v", err)
	}
	if stored.AccessToken != "file-access-token" {
		t.Errorf("AccessToken = %q, want %q", stored.AccessToken, "file-access-token")
	}
	if stored.RefreshToken != "file-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", stored.RefreshToken, "file-refresh-token")
	}

	// GetToken should return the access token
	got, err := tm.GetToken(tokenName)
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if got != "file-access-token" {
		t.Errorf("GetToken() = %q, want %q", got, "file-access-token")
	}

	// Delete token
	if err := tm.DeleteToken(tokenName); err != nil {
		t.Fatalf("DeleteToken() error: %v", err)
	}

	// After deletion, GetToken should fail
	_, err = tm.GetToken(tokenName)
	if err == nil {
		t.Fatal("expected error after deletion, got nil")
	}
}

// TestTokenManager_NoStorageAvailable verifies that when neither keyring nor
// file storage is available, a clear error is returned.
func TestTokenManager_NoStorageAvailable(t *testing.T) {
	oauthConfig := OAuthConfigForEnvironment(EnvironmentProd, DefaultSafetyLevel, nil)
	tm, err := NewTokenManager(oauthConfig)
	if err != nil {
		t.Fatalf("NewTokenManager() error: %v", err)
	}

	// Override deps: nothing available
	tm.deps.keyringAvailable = func() bool { return false }
	tm.deps.fileStoreAvailable = func() bool { return false }

	tokens := &TokenSet{
		AccessToken:  "test",
		RefreshToken: "test",
	}

	// SaveToken should fail with a helpful error
	err = tm.SaveToken("test-token", tokens)
	if err == nil {
		t.Fatal("expected error when no storage available, got nil")
	}
	if !containsStr(err.Error(), EnvTokenStorage) {
		t.Errorf("error should mention %s, got: %v", EnvTokenStorage, err)
	}

	// loadToken should fail
	_, err = tm.loadToken("test-token")
	if err == nil {
		t.Fatal("expected error when no storage available, got nil")
	}

	// DeleteToken should fail
	err = tm.DeleteToken("test-token")
	if err == nil {
		t.Fatal("expected error when no storage available, got nil")
	}
}

// TestTokenManager_FileStorageRoundTrip tests a full save-load-refresh cycle
// using file storage to verify JSON serialization round-trips correctly.
func TestTokenManager_FileStorageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fileStore := NewOAuthFileStoreWithDir(dir)

	oauthConfig := OAuthConfigForEnvironment(EnvironmentProd, DefaultSafetyLevel, nil)
	tm, err := NewTokenManager(oauthConfig)
	if err != nil {
		t.Fatalf("NewTokenManager() error: %v", err)
	}

	tm.deps.keyringAvailable = func() bool { return false }
	tm.deps.fileStoreAvailable = func() bool { return true }
	tm.deps.fileGetToken = func(name string) (string, error) { return fileStore.GetToken(name) }
	tm.deps.fileSetToken = func(name, token string) error { return fileStore.SetToken(name, token) }
	tm.deps.fileDeleteToken = func(name string) error { return fileStore.DeleteToken(name) }

	tokenName := "roundtrip-test"
	expiresAt := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)
	original := &TokenSet{
		AccessToken:  "access-token-value",
		RefreshToken: "refresh-token-value",
		IDToken:      "id-token-value",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		Scope:        "openid profile email",
		ExpiresAt:    expiresAt,
	}

	// Save
	if err := tm.SaveToken(tokenName, original); err != nil {
		t.Fatalf("SaveToken() error: %v", err)
	}

	// Load and verify all fields
	stored, err := tm.loadToken(tokenName)
	if err != nil {
		t.Fatalf("loadToken() error: %v", err)
	}

	if stored.Name != tokenName {
		t.Errorf("Name = %q, want %q", stored.Name, tokenName)
	}
	if stored.AccessToken != original.AccessToken {
		t.Errorf("AccessToken = %q, want %q", stored.AccessToken, original.AccessToken)
	}
	if stored.RefreshToken != original.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", stored.RefreshToken, original.RefreshToken)
	}
	if stored.IDToken != original.IDToken {
		t.Errorf("IDToken = %q, want %q", stored.IDToken, original.IDToken)
	}
	if stored.TokenType != original.TokenType {
		t.Errorf("TokenType = %q, want %q", stored.TokenType, original.TokenType)
	}
	if stored.Scope != original.Scope {
		t.Errorf("Scope = %q, want %q", stored.Scope, original.Scope)
	}

	// Verify the underlying file contains valid JSON
	raw, err := fileStore.GetToken(tm.getKeyringName(tokenName))
	if err != nil {
		t.Fatalf("direct file read error: %v", err)
	}
	var parsed StoredToken
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}
	if parsed.AccessToken != original.AccessToken {
		t.Errorf("parsed AccessToken = %q, want %q", parsed.AccessToken, original.AccessToken)
	}
}

// TestTokenManager_GetTokenInfo_FileStorage verifies GetTokenInfo works
// through file storage.
func TestTokenManager_GetTokenInfo_FileStorage(t *testing.T) {
	dir := t.TempDir()
	fileStore := NewOAuthFileStoreWithDir(dir)

	oauthConfig := OAuthConfigForEnvironment(EnvironmentDev, DefaultSafetyLevel, nil)
	tm, err := NewTokenManager(oauthConfig)
	if err != nil {
		t.Fatalf("NewTokenManager() error: %v", err)
	}

	tm.deps.keyringAvailable = func() bool { return false }
	tm.deps.fileStoreAvailable = func() bool { return true }
	tm.deps.fileGetToken = func(name string) (string, error) { return fileStore.GetToken(name) }
	tm.deps.fileSetToken = func(name, token string) error { return fileStore.SetToken(name, token) }

	tokenName := "info-test"
	tokens := &TokenSet{
		AccessToken:  "info-access",
		RefreshToken: "info-refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour).UTC(),
	}

	if err := tm.SaveToken(tokenName, tokens); err != nil {
		t.Fatalf("SaveToken() error: %v", err)
	}

	info, err := tm.GetTokenInfo(tokenName)
	if err != nil {
		t.Fatalf("GetTokenInfo() error: %v", err)
	}
	if info.AccessToken != "info-access" {
		t.Errorf("AccessToken = %q, want %q", info.AccessToken, "info-access")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestTokenManager_SaveToken_KeyringTooLarge simulates the macOS scenario where
// even the minimal compact form (refresh token only) exceeds the Keychain limit.
// saveToken should fall back to file storage and loadToken should find it there.
func TestTokenManager_SaveToken_KeyringTooLarge(t *testing.T) {
	dir := t.TempDir()
	fileStore := NewOAuthFileStoreWithDir(dir)

	oauthConfig := OAuthConfigForEnvironment(EnvironmentProd, DefaultSafetyLevel, nil)
	tm, err := NewTokenManager(oauthConfig)
	if err != nil {
		t.Fatalf("NewTokenManager() error: %v", err)
	}

	tm.deps.keyringAvailable = func() bool { return true }
	tm.deps.getToken = func(_ *TokenStore, name string) (string, error) {
		return "", fmt.Errorf("token %q not found in keyring", name)
	}
	// All keyring Set calls fail — simulates macOS "data passed to Set was too big".
	tm.deps.setToken = func(_ *TokenStore, name, val string) error {
		return fmt.Errorf("failed to store token in keyring: data passed to Set was too big")
	}
	tm.deps.deleteToken = func(_ *TokenStore, name string) error { return nil }
	// fileStoreAvailable is false because keyring IS available (current system behaviour).
	tm.deps.fileStoreAvailable = func() bool { return false }
	tm.deps.fileGetToken = func(name string) (string, error) { return fileStore.GetToken(name) }
	tm.deps.fileSetToken = func(name, token string) error { return fileStore.SetToken(name, token) }
	tm.deps.fileDeleteToken = func(name string) error { return fileStore.DeleteToken(name) }

	tokens := &TokenSet{
		AccessToken:  "access-token",
		RefreshToken: strings.Repeat("x", 3000), // large enough to exceed macOS Keychain
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}

	if err := tm.SaveToken("large-token", tokens); err != nil {
		t.Fatalf("SaveToken() should fall back to file storage, got error: %v", err)
	}

	// loadToken must find the token in file storage (keyring has nothing).
	stored, err := tm.loadToken("large-token")
	if err != nil {
		t.Fatalf("loadToken() error: %v", err)
	}
	if stored.AccessToken != tokens.AccessToken {
		t.Errorf("AccessToken = %q, want %q", stored.AccessToken, tokens.AccessToken)
	}
	if stored.RefreshToken != tokens.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", stored.RefreshToken, tokens.RefreshToken)
	}
}
