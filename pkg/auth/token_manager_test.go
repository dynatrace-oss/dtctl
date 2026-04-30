package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

func TestTokenManager_getKeyringName(t *testing.T) {
	tests := []struct {
		name        string
		environment Environment
		tokenName   string
		want        string
	}{
		{
			name:        "Production environment token",
			environment: EnvironmentProd,
			tokenName:   "my-token",
			want:        "oauth:prod:my-token",
		},
		{
			name:        "Development environment token",
			environment: EnvironmentDev,
			tokenName:   "dev-token",
			want:        "oauth:dev:dev-token",
		},
		{
			name:        "Hardening environment token",
			environment: EnvironmentHard,
			tokenName:   "sprint-token",
			want:        "oauth:hard:sprint-token",
		},
		{
			name:        "Token with special characters",
			environment: EnvironmentProd,
			tokenName:   "my-env-oauth",
			want:        "oauth:prod:my-env-oauth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a token manager with the specified environment
			config := OAuthConfigForEnvironment(tt.environment, config.DefaultSafetyLevel)
			tm, err := NewTokenManager(config)
			if err != nil {
				t.Fatalf("Failed to create TokenManager: %v", err)
			}

			got := tm.getKeyringName(tt.tokenName)
			if got != tt.want {
				t.Errorf("getKeyringName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewTokenManager(t *testing.T) {
	tests := []struct {
		name    string
		config  *OAuthConfig
		wantEnv Environment
		wantErr bool
	}{
		{
			name:    "Production config",
			config:  OAuthConfigForEnvironment(EnvironmentProd, config.DefaultSafetyLevel),
			wantEnv: EnvironmentProd,
			wantErr: false,
		},
		{
			name:    "Development config",
			config:  OAuthConfigForEnvironment(EnvironmentDev, config.DefaultSafetyLevel),
			wantEnv: EnvironmentDev,
			wantErr: false,
		},
		{
			name:    "Hardening config",
			config:  OAuthConfigForEnvironment(EnvironmentHard, config.DefaultSafetyLevel),
			wantEnv: EnvironmentHard,
			wantErr: false,
		},
		{
			name:    "Nil config defaults to production",
			config:  nil,
			wantEnv: EnvironmentProd,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm, err := NewTokenManager(tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewTokenManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && tm.environment != tt.wantEnv {
				t.Errorf("TokenManager.environment = %v, want %v", tm.environment, tt.wantEnv)
			}
		})
	}
}

func TestTokenManager_EnvironmentIsolation(t *testing.T) {
	// Test that tokens from different environments have different keyring names
	tokenName := "same-token-name"

	prodConfig := OAuthConfigForEnvironment(EnvironmentProd, config.DefaultSafetyLevel)
	prodTM, err := NewTokenManager(prodConfig)
	if err != nil {
		t.Fatalf("Failed to create prod TokenManager: %v", err)
	}

	devConfig := OAuthConfigForEnvironment(EnvironmentDev, config.DefaultSafetyLevel)
	devTM, err := NewTokenManager(devConfig)
	if err != nil {
		t.Fatalf("Failed to create dev TokenManager: %v", err)
	}

	hardConfig := OAuthConfigForEnvironment(EnvironmentHard, config.DefaultSafetyLevel)
	hardTM, err := NewTokenManager(hardConfig)
	if err != nil {
		t.Fatalf("Failed to create hard TokenManager: %v", err)
	}

	prodKey := prodTM.getKeyringName(tokenName)
	devKey := devTM.getKeyringName(tokenName)
	hardKey := hardTM.getKeyringName(tokenName)

	// All three should be different
	if prodKey == devKey || prodKey == hardKey || devKey == hardKey {
		t.Errorf("Token keys should be different across environments: prod=%s, dev=%s, hard=%s",
			prodKey, devKey, hardKey)
	}

	// Verify the expected formats
	expectedProd := "oauth:prod:same-token-name"
	expectedDev := "oauth:dev:same-token-name"
	expectedHard := "oauth:hard:same-token-name"

	if prodKey != expectedProd {
		t.Errorf("Production key = %v, want %v", prodKey, expectedProd)
	}
	if devKey != expectedDev {
		t.Errorf("Development key = %v, want %v", devKey, expectedDev)
	}
	if hardKey != expectedHard {
		t.Errorf("Hardening key = %v, want %v", hardKey, expectedHard)
	}
}

func TestCompactStoredTokenForKeyring(t *testing.T) {
	expiresAt := time.Now().Add(30 * time.Minute).UTC()
	stored := &StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			AccessToken:  "access",
			RefreshToken: "refresh",
			IDToken:      "id",
			TokenType:    "Bearer",
			ExpiresIn:    1800,
			Scope:        "openid profile",
			ExpiresAt:    expiresAt,
		},
	}

	compact := compactStoredTokenForKeyring(stored)
	if compact == nil {
		t.Fatalf("compactStoredTokenForKeyring() returned nil")
	}

	if compact.Name != stored.Name {
		t.Errorf("Name = %q, want %q", compact.Name, stored.Name)
	}
	if compact.RefreshToken != stored.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", compact.RefreshToken, stored.RefreshToken)
	}
	if compact.TokenType != stored.TokenType {
		t.Errorf("TokenType = %q, want %q", compact.TokenType, stored.TokenType)
	}

	if compact.AccessToken != "" {
		t.Errorf("AccessToken = %q, want empty", compact.AccessToken)
	}
	if compact.IDToken != "" {
		t.Errorf("IDToken = %q, want empty", compact.IDToken)
	}
	if compact.Scope != "" {
		t.Errorf("Scope = %q, want empty", compact.Scope)
	}
	if compact.ExpiresIn != 0 {
		t.Errorf("ExpiresIn = %d, want 0", compact.ExpiresIn)
	}
	if !compact.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero value", compact.ExpiresAt)
	}
}

func TestMediumCompactStoredTokenForKeyring(t *testing.T) {
	expiresAt := time.Now().Add(30 * time.Minute).UTC()
	stored := &StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			AccessToken:  "access",
			RefreshToken: "refresh",
			IDToken:      "id",
			TokenType:    "Bearer",
			ExpiresIn:    1800,
			Scope:        "openid offline_access profile",
			ExpiresAt:    expiresAt,
		},
	}

	compact := mediumCompactStoredTokenForKeyring(stored)
	if compact == nil {
		t.Fatalf("mediumCompactStoredTokenForKeyring() returned nil")
	}

	if compact.Name != stored.Name {
		t.Errorf("Name = %q, want %q", compact.Name, stored.Name)
	}
	if compact.RefreshToken != stored.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", compact.RefreshToken, stored.RefreshToken)
	}
	if compact.TokenType != stored.TokenType {
		t.Errorf("TokenType = %q, want %q", compact.TokenType, stored.TokenType)
	}
	if compact.Scope != stored.Scope {
		t.Errorf("Scope = %q, want %q", compact.Scope, stored.Scope)
	}
	if !compact.ExpiresAt.Equal(expiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", compact.ExpiresAt, expiresAt)
	}

	if compact.AccessToken != "" {
		t.Errorf("AccessToken = %q, want empty", compact.AccessToken)
	}
	if compact.IDToken != "" {
		t.Errorf("IDToken = %q, want empty", compact.IDToken)
	}
	if compact.ExpiresIn != 0 {
		t.Errorf("ExpiresIn = %d, want 0", compact.ExpiresIn)
	}

	if mediumCompactStoredTokenForKeyring(nil) != nil {
		t.Error("expected nil input to return nil")
	}
}

func TestIsInvalidGrantError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "invalid_grant", err: fmt.Errorf("token refresh failed: 400 Bad Request - {\"error\":\"invalid_grant\"}"), want: true},
		{name: "wrapped invalid_grant", err: fmt.Errorf("failed to refresh token: %w", fmt.Errorf("token refresh failed: 400 Bad Request - {\"error\":\"invalid_grant\",\"error_description\":\"UNSUCCESSFUL_OAUTH_REFRESH_TOKEN_VALIDATION_FAILED\"}")), want: true},
		{name: "network error", err: fmt.Errorf("token refresh request failed: dial tcp: connection refused"), want: false},
		{name: "server error", err: fmt.Errorf("token refresh failed: 500 Internal Server Error"), want: false},
		{name: "expired access token", err: fmt.Errorf("token expired and refresh failed"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInvalidGrantError(tt.err); got != tt.want {
				t.Errorf("isInvalidGrantError() = %v, want %v", got, tt.want)
			}
		})
	}
}

// invalidGrantHTTPDo returns a fake httpDo that always responds with 400 invalid_grant.
func invalidGrantHTTPDo(_ *http.Request) (*http.Response, error) {
	body := `{"error":"invalid_grant","error_description":"UNSUCCESSFUL_OAUTH_REFRESH_TOKEN_VALIDATION_FAILED"}`
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

// newTMWithFakeKeyring builds a TokenManager whose storage is backed by an
// in-memory map, so tests never touch the OS keyring. The returned map can be
// inspected and mutated directly by test code.
func newTMWithFakeKeyring(t *testing.T) (*TokenManager, map[string]string) {
	t.Helper()
	store := make(map[string]string)

	oauthCfg := OAuthConfigForEnvironment(EnvironmentProd, config.DefaultSafetyLevel)
	tm, err := NewTokenManager(oauthCfg)
	if err != nil {
		t.Fatalf("NewTokenManager() error: %v", err)
	}

	tm.deps.keyringAvailable = func() bool { return true }
	tm.deps.getToken = func(_ *config.TokenStore, name string) (string, error) {
		v, ok := store[name]
		if !ok {
			return "", fmt.Errorf("token %q not found in keyring", name)
		}
		return v, nil
	}
	tm.deps.setToken = func(_ *config.TokenStore, name, val string) error {
		store[name] = val
		return nil
	}
	tm.deps.deleteToken = func(_ *config.TokenStore, name string) error {
		delete(store, name)
		return nil
	}
	tm.deps.fileStoreAvailable = func() bool { return false }

	return tm, store
}

func TestTokenManager_GetToken_InvalidGrant_CompactFormat(t *testing.T) {
	t.Parallel()
	tm, store := newTMWithFakeKeyring(t)
	tm.flow.httpDo = invalidGrantHTTPDo

	// Seed compact format: only refresh token, no access token, no expiry.
	key := tm.getKeyringName("my-token")
	compact, _ := json.Marshal(&StoredToken{
		Name:     "my-token",
		TokenSet: TokenSet{RefreshToken: "stale-refresh"},
	})
	store[key] = string(compact)

	_, err := tm.GetToken("my-token")

	// Error must wrap ErrOAuthSessionRevoked so the caller falls back to the platform token.
	if err == nil {
		t.Fatal("GetToken() returned nil, want error")
	}
	if !errors.Is(err, ErrOAuthSessionRevoked) {
		t.Errorf("error %q should wrap ErrOAuthSessionRevoked", err.Error())
	}

	// Stale cache entry must be gone.
	if _, ok := store[key]; ok {
		t.Error("stale OAuth cache entry still present after invalid_grant")
	}
}

func TestTokenManager_GetToken_InvalidGrant_ExpiredToken(t *testing.T) {
	t.Parallel()
	tm, store := newTMWithFakeKeyring(t)
	tm.flow.httpDo = invalidGrantHTTPDo

	key := tm.getKeyringName("my-token")
	expired, _ := json.Marshal(&StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			AccessToken:  "expired-access",
			RefreshToken: "stale-refresh",
			ExpiresAt:    time.Now().Add(-1 * time.Hour), // expired
		},
	})
	store[key] = string(expired)

	_, err := tm.GetToken("my-token")

	if err == nil {
		t.Fatal("GetToken() returned nil, want error")
	}
	if !errors.Is(err, ErrOAuthSessionRevoked) {
		t.Errorf("error %q should wrap ErrOAuthSessionRevoked", err.Error())
	}
	if _, ok := store[key]; ok {
		t.Error("stale OAuth cache entry still present after invalid_grant")
	}
}

func TestTokenManager_GetToken_InvalidGrant_TokenNearExpiry(t *testing.T) {
	// Access token within the refresh buffer but not yet expired — refresh is
	// attempted, fails with invalid_grant; cache is evicted so the next call
	// can fall back to the platform token rather than hitting the same error.
	t.Parallel()
	tm, store := newTMWithFakeKeyring(t)
	tm.flow.httpDo = invalidGrantHTTPDo

	key := tm.getKeyringName("my-token")
	nearExpiry, _ := json.Marshal(&StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			AccessToken:  "almost-expired-access",
			RefreshToken: "stale-refresh",
			ExpiresAt:    time.Now().Add(1 * time.Minute), // within 5-min buffer
		},
	})
	store[key] = string(nearExpiry)

	_, err := tm.GetToken("my-token")

	if err == nil {
		t.Fatal("GetToken() returned nil, want error")
	}
	if !errors.Is(err, ErrOAuthSessionRevoked) {
		t.Errorf("error %q should wrap ErrOAuthSessionRevoked", err.Error())
	}
	if _, ok := store[key]; ok {
		t.Error("stale OAuth cache entry still present after invalid_grant")
	}
}

func TestTokenManager_GetToken_TransientError_DoesNotEvict(t *testing.T) {
	// A network/5xx failure must NOT evict the cache — the token may still be
	// usable if the access token hasn't expired yet.
	t.Parallel()
	tm, store := newTMWithFakeKeyring(t)
	tm.flow.httpDo = func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	}

	key := tm.getKeyringName("my-token")
	stillValid, _ := json.Marshal(&StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			AccessToken:  "still-valid-access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(1 * time.Minute), // within buffer but not expired
		},
	})
	store[key] = string(stillValid)

	got, err := tm.GetToken("my-token")

	// Should return the still-valid access token, not an error.
	if err != nil {
		t.Fatalf("GetToken() error = %v, want nil (should use cached access token)", err)
	}
	if got != "still-valid-access" {
		t.Errorf("GetToken() = %q, want %q", got, "still-valid-access")
	}
	// Cache entry must NOT have been evicted.
	if _, ok := store[key]; !ok {
		t.Error("cache entry was incorrectly evicted on transient error")
	}
}
