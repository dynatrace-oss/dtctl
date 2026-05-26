package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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

// TestTokenManager_GetToken_ConcurrentCompact verifies the core fix: when N
// goroutines simultaneously call GetToken on a compact token (refresh_token
// only, no access_token, no expiry), the OAuth token endpoint is contacted
// exactly once. All other goroutines should reuse the access_token written by
// the winner.
//
// This test exercises the cross-process advisory lock indirectly — within a
// single process goroutines share address space, so Go's sync primitives are
// also involved, but the re-read-after-lock logic in GetToken is exercised by
// the in-process race as well. The cross-process lock is separately validated
// by the flock semantics on the operating system.
//
// A unique token name is used so that the underlying lock file path is
// distinct from any other concurrent test run on the same machine.
func TestTokenManager_GetToken_ConcurrentCompact(t *testing.T) {
	t.Parallel()

	const goroutines = 10

	// Unique token name prevents lock file collisions with concurrent test runs
	// (e.g. go test -count=2 or parallel CI jobs on the same host).
	tokenName := "concurrent-test-token-" + t.Name()

	// Count how many times the fake OAuth endpoint is reached.
	var refreshCalls atomic.Int32

	// Shared in-memory keyring protected by a mutex so concurrent setToken /
	// getToken calls are safe without data races.
	var storeMu sync.Mutex
	store := make(map[string]string)

	// Build one TokenManager per goroutine — each gets its own struct but they
	// share the same underlying store map, simulating separate processes that
	// all talk to the same OS keychain.
	newTM := func() *TokenManager {
		oauthCfg := OAuthConfigForEnvironment(EnvironmentProd, config.DefaultSafetyLevel)
		tm, err := NewTokenManager(oauthCfg)
		if err != nil {
			// Cannot use t.Fatalf here: calling t.Fatalf from a goroutine other
			// than the test goroutine calls runtime.Goexit on the spawned goroutine,
			// not on the test goroutine, so the test would not be marked as failed.
			// Return nil and let the caller handle it via the error channel.
			return nil
		}
		tm.deps.keyringAvailable = func() bool { return true }
		tm.deps.getToken = func(_ *config.TokenStore, name string) (string, error) {
			storeMu.Lock()
			defer storeMu.Unlock()
			v, ok := store[name]
			if !ok {
				return "", fmt.Errorf("token %q not found", name)
			}
			return v, nil
		}
		tm.deps.setToken = func(_ *config.TokenStore, name, val string) error {
			storeMu.Lock()
			defer storeMu.Unlock()
			store[name] = val
			return nil
		}
		tm.deps.deleteToken = func(_ *config.TokenStore, name string) error {
			storeMu.Lock()
			defer storeMu.Unlock()
			delete(store, name)
			return nil
		}
		tm.deps.fileStoreAvailable = func() bool { return false }

		// Fake OAuth endpoint: increment the counter and return a fresh token.
		// Includes a small sleep to increase the chance of concurrent callers
		// overlapping inside the "needs refresh" window.
		tm.flow.httpDo = func(_ *http.Request) (*http.Response, error) {
			refreshCalls.Add(1)
			time.Sleep(10 * time.Millisecond)
			body := `{
				"access_token":  "new-access-token",
				"refresh_token": "new-refresh-token",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         "openid"
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}

		return tm
	}

	// Seed a compact token into the shared store using the first manager's
	// key derivation (all managers share the same environment so keys match).
	seed := newTM()
	if seed == nil {
		t.Fatal("failed to create seed TokenManager")
	}
	key := seed.getKeyringName(tokenName)
	compact, _ := json.Marshal(&StoredToken{
		Name:     tokenName,
		TokenSet: TokenSet{RefreshToken: "original-refresh"},
	})
	store[key] = string(compact)

	// Launch N goroutines, each with its own TokenManager, all calling GetToken
	// at the same time. Errors are collected via a buffered channel to avoid
	// calling t.Fatalf from a non-test goroutine (which would only Goexit the
	// spawned goroutine, not mark the test as failed).
	type result struct {
		token string
		err   error
	}
	results := make(chan result, goroutines)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tm := newTM()
			if tm == nil {
				results <- result{err: fmt.Errorf("NewTokenManager returned nil")}
				return
			}
			tok, err := tm.GetToken(tokenName)
			results <- result{token: tok, err: err}
		}()
	}
	wg.Wait()
	close(results)

	for r := range results {
		if r.err != nil {
			t.Errorf("GetToken() error = %v", r.err)
			continue
		}
		if r.token != "new-access-token" {
			t.Errorf("GetToken() = %q, want %q", r.token, "new-access-token")
		}
	}

	// The OAuth endpoint must have been called exactly once. If it was called
	// more than once the lock (or the re-read-after-lock) is not working.
	if n := refreshCalls.Load(); n != 1 {
		t.Errorf("OAuth refresh endpoint called %d times, want exactly 1", n)
	}
}

func TestTokenManager_GetToken_MediumCompact_ReturnsToken(t *testing.T) {
	// Regression test for CR-5/SEC-1: when the keyring holds a medium-compact
	// token (AccessToken stripped, ExpiresAt preserved), GetToken must still
	// call RefreshToken and return a real access token — not silently return "".
	//
	// Before the fix, the ExpiresAt.IsZero() guard on the compact branch meant
	// medium-compact tokens bypassed the refresh path and returned "" with nil error.
	t.Parallel()
	tm, store := newTMWithFakeKeyring(t)
	tm.flow.httpDo = func(_ *http.Request) (*http.Response, error) {
		body := `{
			"access_token":  "refreshed-access",
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"scope":         "openid"
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}

	key := tm.getKeyringName("my-token")
	// Medium-compact: no access token, but ExpiresAt is set (not zero).
	mediumCompact, _ := json.Marshal(&StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			RefreshToken: "medium-refresh",
			ExpiresAt:    time.Now().Add(1 * time.Hour), // non-zero: would fool old code
		},
	})
	store[key] = string(mediumCompact)

	got, err := tm.GetToken("my-token")

	if err != nil {
		t.Fatalf("GetToken() error = %v, want nil", err)
	}
	if got != "refreshed-access" {
		t.Errorf("GetToken() = %q, want %q (medium-compact must trigger refresh)", got, "refreshed-access")
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

func TestTokenManager_GetToken_LockFailure_FallsThrough(t *testing.T) {
	// The refresh lock is best-effort: when acquireRefreshLock returns an
	// error (e.g. /tmp is read-only, filesystem is full, signal interrupts
	// the open) GetToken must log a warning and still proceed with the
	// refresh rather than aborting. This protects single-process callers
	// from being broken by a filesystem-level lock failure.
	//
	// Cannot run in parallel: we swap the package-level acquireRefreshLock.

	originalLock := acquireRefreshLock
	t.Cleanup(func() { acquireRefreshLock = originalLock })

	var lockCalls int
	acquireRefreshLock = func(_, _ string) (func(), error) {
		lockCalls++
		return func() {}, errors.New("simulated lock failure")
	}

	// Redirect stderr so the warning does not pollute test output, and so we
	// can assert the operator-visible message is present.
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	originalStderr := os.Stderr
	os.Stderr = stderrW
	t.Cleanup(func() { os.Stderr = originalStderr })

	tm, store := newTMWithFakeKeyring(t)
	tm.flow.httpDo = func(_ *http.Request) (*http.Response, error) {
		body := `{
			"access_token":  "refreshed-access",
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"scope":         "openid"
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}

	key := tm.getKeyringName("my-token")
	compact, _ := json.Marshal(&StoredToken{
		Name:     "my-token",
		TokenSet: TokenSet{RefreshToken: "stale-refresh"},
	})
	store[key] = string(compact)

	got, err := tm.GetToken("my-token")

	// Close the write end so the read does not block, then restore stderr
	// before any t.Errorf so failure messages are visible to the operator.
	_ = stderrW.Close()
	stderrBytes, _ := io.ReadAll(stderrR)
	os.Stderr = originalStderr

	if err != nil {
		t.Fatalf("GetToken() error = %v, want nil (lock failure must not abort)", err)
	}
	if got != "refreshed-access" {
		t.Errorf("GetToken() = %q, want %q (refresh must still happen)", got, "refreshed-access")
	}
	if lockCalls != 1 {
		t.Errorf("acquireRefreshLock call count = %d, want 1", lockCalls)
	}
	if !strings.Contains(string(stderrBytes), "could not acquire token refresh lock") {
		t.Errorf("stderr = %q, want to contain warning about lock acquisition", string(stderrBytes))
	}
}
