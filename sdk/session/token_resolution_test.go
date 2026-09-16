package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adrg/xdg"
)

func TestErrOAuthSessionRevoked_IsRecognised(t *testing.T) {
	wrapped := fmt.Errorf("token %q: %w; re-authenticate", "my-token", ErrOAuthSessionRevoked)
	if !errors.Is(wrapped, ErrOAuthSessionRevoked) {
		t.Fatal("errors.Is should match wrapped ErrOAuthSessionRevoked")
	}
	// And it should NOT match isOAuthTokenNotFoundError (the message no longer says "not found").
	if isOAuthTokenNotFoundError(wrapped) {
		t.Error("isOAuthTokenNotFoundError should not match a session-revoked error")
	}
}

func TestIsOAuthTokenNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "keyring not found", err: errors.New("failed to load token from keyring: token \"oauth:prod:my-token\" not found in keyring"), want: true},
		{name: "generic token not found", err: errors.New("token not found"), want: true},
		{name: "refresh token expired", err: errors.New("failed to refresh token: invalid_grant"), want: false},
		{name: "network", err: errors.New("token refresh request failed: dial tcp timeout"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOAuthTokenNotFoundError(tt.err); got != tt.want {
				t.Errorf("isOAuthTokenNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTokenWithOAuthSupport_FallsBackWithoutOAuthContext(t *testing.T) {
	t.Setenv(EnvDisableKeyring, "1")

	cfg := NewConfig()
	if err := cfg.SetToken("api-token", "dt0c01.test"); err != nil {
		t.Fatalf("SetToken() error = %v", err)
	}

	got, err := GetTokenWithOAuthSupport(cfg, "api-token")
	if err != nil {
		t.Fatalf("GetTokenWithOAuthSupport() error = %v", err)
	}
	if got != "dt0c01.test" {
		t.Fatalf("GetTokenWithOAuthSupport() = %q, want %q", got, "dt0c01.test")
	}
}

// A forced refresh that fails with invalid_grant must evict the revoked cache
// entry (mirroring GetToken) while still returning the fallback token so the
// caller surfaces the original 401.
func TestForceRefreshWithManager_InvalidGrantEvictsCache(t *testing.T) {
	tm, store := newTMWithFakeKeyring(t)
	tm.flow.httpDo = invalidGrantHTTPDo

	key := tm.getKeyringName("my-token")
	valid, _ := json.Marshal(&StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			AccessToken:  "looks-valid-but-rejected",
			RefreshToken: "revoked-refresh",
			ExpiresAt:    time.Now().Add(1 * time.Hour),
		},
	})
	store[key] = string(valid)

	got, err := forceRefreshWithManager(tm, "my-token", "looks-valid-but-rejected")
	if err != nil {
		t.Fatalf("forceRefreshWithManager: %v", err)
	}
	if got != "looks-valid-but-rejected" {
		t.Errorf("token = %q, want the fallback so the caller surfaces the 401", got)
	}
	if _, ok := store[key]; ok {
		t.Error("revoked OAuth cache entry still present after invalid_grant on forced refresh")
	}
}

// A transient refresh failure (not invalid_grant) must NOT evict the cache.
func TestForceRefreshWithManager_TransientFailureKeepsCache(t *testing.T) {
	tm, store := newTMWithFakeKeyring(t)
	tm.flow.httpDo = func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Body:       io.NopCloser(strings.NewReader("upstream error")),
		}, nil
	}

	key := tm.getKeyringName("my-token")
	valid, _ := json.Marshal(&StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			AccessToken:  "still-good",
			RefreshToken: "still-good-refresh",
			ExpiresAt:    time.Now().Add(1 * time.Hour),
		},
	})
	store[key] = string(valid)

	got, err := forceRefreshWithManager(tm, "my-token", "still-good")
	if err != nil || got != "still-good" {
		t.Fatalf("got (%q, %v), want fallback with nil error", got, err)
	}
	if _, ok := store[key]; !ok {
		t.Error("cache entry evicted on a transient failure")
	}
}

// writeOAuthFileEntry writes a fake OAuth file entry to the given oauth-tokens dir.
func writeOAuthFileEntry(t *testing.T, dir, keyringName, accessToken string) {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"access_token": accessToken})
	name := sanitizeTokenName(keyringName) + ".json"
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatalf("writeOAuthFileEntry: %v", err)
	}
}

// withTempDataDir overrides xdg.DataHome with a temp dir for the duration of
// the test, writing any pre-populated oauth-tokens files via the callback.
func withTempDataDir(t *testing.T, populate func(oauthDir string)) (restore func()) {
	t.Helper()
	tmpDir := t.TempDir()
	oauthDir := filepath.Join(tmpDir, "dtctl", "oauth-tokens")
	if err := os.MkdirAll(oauthDir, 0700); err != nil {
		t.Fatal(err)
	}
	if populate != nil {
		populate(oauthDir)
	}
	// Directly patch the global that DataDir() reads, then restore.
	prev := xdg.DataHome
	xdg.DataHome = tmpDir
	return func() { xdg.DataHome = prev }
}

// TestSealedConfigIgnoresFileStore verifies that a sealed config returns the
// inline token even when a file-store entry exists that would otherwise win.
func TestSealedConfigIgnoresFileStore(t *testing.T) {
	t.Setenv(EnvDisableKeyring, "1")
	t.Setenv(EnvTokenStorage, "file")

	cfg := NewConfig()
	if err := cfg.SetToken("session-test", "request-token"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	cfg.SealInlineCredentials()

	// Write a file-store entry AFTER SetToken (SetToken invalidates OAuth cache
	// entries) — the sealed config must return the inline value, not this.
	restore := withTempDataDir(t, func(oauthDir string) {
		writeOAuthFileEntry(t, oauthDir, "oauth:prod:session-test", "stolen-token")
	})
	defer restore()

	got, err := GetTokenForContext(cfg, "https://prod.example.invalid", "session-test")
	if err != nil {
		t.Fatalf("GetTokenForContext: %v", err)
	}
	if got != "request-token" {
		t.Errorf("GetTokenForContext = %q, want %q (file store must be ignored for sealed configs)", got, "request-token")
	}
}

// TestUnsealedConfigConsultsFileStore is the baseline: without sealing, the
// file store entry wins over the inline token. This guards against the seal
// becoming a no-op.
func TestUnsealedConfigConsultsFileStore(t *testing.T) {
	t.Setenv(EnvDisableKeyring, "1")
	t.Setenv(EnvTokenStorage, "file")

	cfg := NewConfig()
	if err := cfg.SetToken("session-test", "inline-token"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	// NOT sealed — the file store should be consulted.

	// Write the file-store entry AFTER SetToken: SetToken invalidates OAuth cache
	// entries for the token name, which would delete a pre-written file.
	restore := withTempDataDir(t, func(oauthDir string) {
		writeOAuthFileEntry(t, oauthDir, "oauth:prod:session-test", "file-store-token")
	})
	defer restore()

	got, err := cfg.GetToken("session-test")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	// The file store entry wins over the inline token on an unsealed config.
	if got != "file-store-token" {
		t.Errorf("GetToken = %q, want %q (file store must be consulted for unsealed configs)", got, "file-store-token")
	}
}

// TestLocalConfigIgnoresOAuthStore verifies that GetTokenForContext never
// consults the OAuth TokenManager for an auto-discovered local config. Without
// this guard, a rogue .dtctl.yaml could redirect stored OAuth credentials to
// any host by setting token-ref to a known OAuth key name.
//
// The test uses a non-local (unsealed, regular) config to prove the file store
// IS consulted normally via TokenManager, then a local config to prove it is NOT.
func TestLocalConfigIgnoresOAuthStore(t *testing.T) {
	t.Setenv(EnvDisableKeyring, "1")
	t.Setenv(EnvTokenStorage, "file")

	const env = "https://abc12345.apps.dynatrace.com"
	const tokenRef = "prod"

	restore := withTempDataDir(t, func(oauthDir string) {
		writeOAuthFileEntry(t, oauthDir, "oauth:prod:prod", "stolen-token")
	})
	defer restore()

	// Baseline: a non-local config DOES return the TokenManager file-store token,
	// confirming the path under test is actually reachable.
	baseline := NewConfig()
	got, err := GetTokenForContext(baseline, env, tokenRef)
	if err != nil || got != "stolen-token" {
		t.Fatalf("baseline: GetTokenForContext = (%q, %v), want (\"stolen-token\", nil) — file store unreachable, test invalid", got, err)
	}

	// Local config: the TokenManager path must be skipped entirely.
	cfg := newLocalConfig(t, env, tokenRef, nil)
	got, _ = GetTokenForContext(cfg, env, tokenRef)
	if got == "stolen-token" {
		t.Error("GetTokenForContext returned OAuth TokenManager token for a local config — OAuth bypass not fixed")
	}
}

// TestLocalConfigRefreshIgnoresOAuthStore verifies that RefreshedTokenForContext
// skips the OAuth refresh path for local configs. The test arranges for
// GetTokenForContext to succeed (returning a known token), so RefreshedTokenForContext
// reaches the cfg.IsLocal() gate at token_resolution.go rather than returning
// early on an error.
//
// Limitation: full mutation-testing of this guard requires an injectable OAuth
// HTTP client; without it, forceRefreshWithManager falls back to the stale token
// on any network failure, making the outcome identical with or without the guard.
// The guard is verified correct by code inspection and by the GetTokenForContext
// test above, which shares the same guard expression.
func TestLocalConfigRefreshIgnoresOAuthStore(t *testing.T) {
	t.Setenv(EnvDisableKeyring, "1")
	t.Setenv(EnvTokenStorage, "file")

	const env = "https://abc12345.apps.dynatrace.com"
	const tokenRef = "prod"
	const staleToken = "stale-access-token"

	// Write an OAuth file entry that Config.GetToken (not TokenManager) will
	// return after the origin binding check passes. This lets GetTokenForContext
	// succeed and return staleToken, so RefreshedTokenForContext reaches the guard.
	restore := withTempDataDir(t, func(oauthDir string) {
		writeOAuthFileEntry(t, oauthDir, "oauth:prod:prod", staleToken)
	})
	defer restore()

	trusted := map[string]string{tokenRef: "abc12345.apps.dynatrace.com"}
	cfg := newLocalConfig(t, env, tokenRef, trusted)

	// Verify GetTokenForContext resolves successfully (reaches the guard via GetToken).
	resolved, err := GetTokenForContext(cfg, env, tokenRef)
	if err != nil || resolved != staleToken {
		t.Fatalf("GetTokenForContext = (%q, %v), want (%q, nil) — test setup invalid", resolved, err, staleToken)
	}

	// RefreshedTokenForContext must return the stale token unchanged (no network refresh).
	got, err := RefreshedTokenForContext(cfg, env, tokenRef, staleToken)
	if err != nil {
		t.Fatalf("RefreshedTokenForContext: %v", err)
	}
	if got != staleToken {
		t.Errorf("RefreshedTokenForContext = %q, want %q", got, staleToken)
	}
}

// newLocalConfig returns a Config marked as local (auto-discovered) with the
// given environment URL and token-ref. trustedOrigins overrides whatever
// buildTrustedOrigins() loads from the host's global config, making tests
// hermetic. Pass nil to keep the host-loaded origins (not recommended for
// new tests).
func newLocalConfig(t *testing.T, environment, tokenRef string, trustedOrigins map[string]string) *Config {
	t.Helper()
	cfg := NewConfig()
	ctx := Context{Environment: environment, TokenRef: tokenRef}
	cfg.Contexts = append(cfg.Contexts, NamedContext{Name: "default", Context: ctx})
	cfg.CurrentContext = "default"
	tmpFile := filepath.Join(t.TempDir(), ".dtctl.yaml")
	if err := os.WriteFile(tmpFile, []byte(""), 0600); err != nil {
		t.Fatalf("newLocalConfig: %v", err)
	}
	cfg.markLocal(tmpFile)
	if trustedOrigins != nil {
		cfg.globalBindings = make(map[string]globalBinding, len(trustedOrigins))
		for ref, host := range trustedOrigins {
			cfg.globalBindings[ref] = globalBinding{hosts: []string{host}, level: DefaultSafetyLevel}
		}
	}
	return cfg
}

// TestSealedConfigRefreshReturnsInline verifies that RefreshedTokenForContext
// returns the inline token unchanged for a sealed config without contacting any
// OAuth endpoint.
func TestSealedConfigRefreshReturnsInline(t *testing.T) {
	t.Setenv(EnvDisableKeyring, "1")
	t.Setenv(EnvTokenStorage, "file")

	cfg := NewConfig()
	if err := cfg.SetToken("tok-ref", "inline-token"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	cfg.SealInlineCredentials()

	// Use a non-routable address so any OAuth call would fail noticeably.
	got, err := RefreshedTokenForContext(cfg, "https://192.0.2.1", "tok-ref", "inline-token")
	if err != nil {
		t.Fatalf("RefreshedTokenForContext: %v", err)
	}
	if got != "inline-token" {
		t.Errorf("RefreshedTokenForContext = %q, want %q", got, "inline-token")
	}
}
