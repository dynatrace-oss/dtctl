package session

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// newTMWithSizedKeyring returns a TokenManager backed by an in-memory keyring
// that rejects any value longer than limitBytes with a macOS-style
// "too big" error, plus an in-memory file store used as the last-resort
// fallback. The returned maps expose what actually got persisted.
func newTMWithSizedKeyring(t *testing.T, limitBytes int) (tm *TokenManager, keyring, files map[string]string) {
	t.Helper()
	keyring = make(map[string]string)
	files = make(map[string]string)

	oauthCfg := OAuthConfigForEnvironment(EnvironmentProd, DefaultSafetyLevel, nil)
	var err error
	tm, err = NewTokenManager(oauthCfg)
	if err != nil {
		t.Fatalf("NewTokenManager() error: %v", err)
	}

	tm.deps.keyringAvailable = func() bool { return true }
	tm.deps.getToken = func(_ *TokenStore, name string) (string, error) {
		v, ok := keyring[name]
		if !ok {
			return "", fmt.Errorf("token %q not found in keyring", name)
		}
		return v, nil
	}
	tm.deps.setToken = func(_ *TokenStore, name, val string) error {
		if limitBytes >= 0 && len(val) > limitBytes {
			return fmt.Errorf("data passed to Set was too big") // matches isKeyringTooLargeErr
		}
		keyring[name] = val
		return nil
	}
	tm.deps.deleteToken = func(_ *TokenStore, name string) error {
		delete(keyring, name)
		return nil
	}
	tm.deps.fileStoreAvailable = func() bool { return false }
	tm.deps.fileSetToken = func(name, val string) error {
		files[name] = val
		return nil
	}
	tm.deps.fileGetToken = func(name string) (string, error) {
		v, ok := files[name]
		if !ok {
			return "", fmt.Errorf("token %q not found in file store", name)
		}
		return v, nil
	}
	tm.deps.fileDeleteToken = func(name string) error {
		delete(files, name)
		return nil
	}
	return tm, keyring, files
}

func sampleStoredToken() *StoredToken {
	return &StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			AccessToken:  "ACCESS-" + strings.Repeat("a", 1000),
			RefreshToken: "refresh-token",
			IDToken:      "IDT-" + strings.Repeat("i", 1000),
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			Scope:        "openid " + strings.Repeat("scope ", 200),
			ExpiresAt:    time.Now().Add(1 * time.Hour).UTC(),
		},
	}
}

func mustLen(t *testing.T, v any) int {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return len(b)
}

// TestSaveToken_PreservesAccessTokenUnderSizeLimit is the core regression test
// for the "access token stripped on macOS" bug: when the full token set is too
// large for the keyring but a smaller encoding that still contains the access
// token fits, saveToken must persist that encoding — not fall straight to the
// access-token-less form. Without the fix, GetToken's fast path never hits and
// every invocation pays an SSO refresh.
func TestSaveToken_PreservesAccessTokenUnderSizeLimit(t *testing.T) {
	t.Parallel()
	// Access token larger than the scope string, so every encoding that drops
	// a field is strictly smaller than the one before it. This exercises the
	// full tier ladder (full -> noID -> accessOnly -> medium -> minimal) in a
	// single, deterministic ordering.
	stored := sampleStoredToken()
	stored.AccessToken = "ACCESS-" + strings.Repeat("a", 4000)

	sizeFull := mustLen(t, stored)
	sizeNoID := mustLen(t, withoutIDTokenForKeyring(stored))
	sizeAccessOnly := mustLen(t, accessOnlyStoredTokenForKeyring(stored))
	sizeMedium := mustLen(t, mediumCompactStoredTokenForKeyring(stored))
	sizeMinimal := mustLen(t, compactStoredTokenForKeyring(stored))

	// With access > id ~ scope, the ladder shrinks monotonically.
	if !(sizeFull > sizeNoID && sizeNoID > sizeAccessOnly && sizeAccessOnly > sizeMedium && sizeMedium > sizeMinimal) {
		t.Fatalf("expected monotonically shrinking encodings, got full=%d noID=%d accessOnly=%d medium=%d minimal=%d",
			sizeFull, sizeNoID, sizeAccessOnly, sizeMedium, sizeMinimal)
	}

	tests := []struct {
		name          string
		limit         int
		wantAccess    bool
		wantScope     bool
		wantID        bool
		wantFileStore bool
	}{
		{name: "full fits", limit: sizeFull, wantAccess: true, wantScope: true, wantID: true},
		{name: "drop id_token only", limit: sizeNoID, wantAccess: true, wantScope: true, wantID: false},
		{name: "access-only (drop id+scope)", limit: sizeAccessOnly, wantAccess: true, wantScope: false, wantID: false},
		{name: "medium (drop access, keep scope)", limit: sizeMedium, wantAccess: false, wantScope: true, wantID: false},
		{name: "minimal (refresh only)", limit: sizeMinimal, wantAccess: false, wantScope: false, wantID: false},
		{name: "smaller than minimal -> file store", limit: sizeMinimal - 1, wantFileStore: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tm, keyring, files := newTMWithSizedKeyring(t, tc.limit)
			key := tm.getKeyringName("my-token")

			if err := tm.saveToken("my-token", stored); err != nil {
				t.Fatalf("saveToken() error = %v", err)
			}

			if tc.wantFileStore {
				if _, ok := keyring[key]; ok {
					t.Errorf("keyring entry should be absent when spilled to file store")
				}
				raw, ok := files[key]
				if !ok {
					t.Fatalf("expected token in file store, none found")
				}
				var got StoredToken
				if err := json.Unmarshal([]byte(raw), &got); err != nil {
					t.Fatalf("unmarshal file token: %v", err)
				}
				// File store holds the full token, including the access token.
				if got.AccessToken != stored.AccessToken {
					t.Errorf("file store access token = %q, want full access token", got.AccessToken)
				}
				return
			}

			raw, ok := keyring[key]
			if !ok {
				t.Fatalf("expected token in keyring, none found")
			}
			var got StoredToken
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatalf("unmarshal keyring token: %v", err)
			}

			if hasAccess := got.AccessToken != ""; hasAccess != tc.wantAccess {
				t.Errorf("access token present = %v, want %v", hasAccess, tc.wantAccess)
			}
			if hasScope := got.Scope != ""; hasScope != tc.wantScope {
				t.Errorf("scope present = %v, want %v", hasScope, tc.wantScope)
			}
			if hasID := got.IDToken != ""; hasID != tc.wantID {
				t.Errorf("id token present = %v, want %v", hasID, tc.wantID)
			}
			// The refresh token must survive in every keyring encoding.
			if got.RefreshToken != stored.RefreshToken {
				t.Errorf("refresh token = %q, want %q", got.RefreshToken, stored.RefreshToken)
			}
			// Any encoding that keeps the access token must keep its expiry so
			// GetToken's fast path (AccessToken != "" && !needsRefresh) works.
			if tc.wantAccess && got.ExpiresAt.IsZero() {
				t.Errorf("expiry must be preserved alongside a cached access token")
			}
		})
	}
}

// TestSaveToken_RoundTripsThroughGetTokenFastPath verifies that after saving a
// token whose full form is too large for the keyring but whose access token
// still fits, GetToken returns the cached access token without contacting the
// OAuth endpoint.
func TestSaveToken_RoundTripsThroughGetTokenFastPath(t *testing.T) {
	t.Parallel()
	stored := sampleStoredToken()
	limit := mustLen(t, withoutIDTokenForKeyring(stored)) // full won't fit, no-id will

	tm, _, _ := newTMWithSizedKeyring(t, limit)
	var refreshCalled bool
	tm.flow.httpDo = func(_ *http.Request) (*http.Response, error) {
		refreshCalled = true
		return nil, fmt.Errorf("OAuth endpoint must not be called on fast path")
	}

	if err := tm.saveToken("my-token", stored); err != nil {
		t.Fatalf("saveToken() error = %v", err)
	}

	got, err := tm.GetToken("my-token")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != stored.AccessToken {
		t.Errorf("GetToken() = %q, want cached access token", got)
	}
	if refreshCalled {
		t.Error("GetToken() triggered an OAuth refresh despite a cached, unexpired access token")
	}
}

// TestSaveToken_ScopeCompanion verifies that the scope string is preserved in
// a companion keyring entry exactly when the primary encoding had to drop it
// (access token kept, scope dropped), and that CachedScopes reads it back. When
// the primary encoding keeps the scope, no companion is written.
func TestSaveToken_ScopeCompanion(t *testing.T) {
	t.Parallel()
	stored := sampleStoredToken()
	stored.AccessToken = "ACCESS-" + strings.Repeat("a", 4000)
	companionKey := func(tm *TokenManager) string { return tm.getKeyringName("my-token") + scopeCompanionSuffix }
	wantScopes := strings.Fields(stored.Scope)

	t.Run("scope dropped -> companion written and readable", func(t *testing.T) {
		limit := mustLen(t, accessOnlyStoredTokenForKeyring(stored)) // keeps access, drops scope
		tm, keyring, _ := newTMWithSizedKeyring(t, limit)
		if err := tm.saveToken("my-token", stored); err != nil {
			t.Fatalf("saveToken() error = %v", err)
		}
		if got := keyring[companionKey(tm)]; got != stored.Scope {
			t.Errorf("companion scope = %q, want the full scope string", got)
		}
		got := tm.CachedScopes("my-token")
		if len(got) != len(wantScopes) {
			t.Fatalf("CachedScopes() = %d scopes, want %d", len(got), len(wantScopes))
		}
	})

	t.Run("scope kept -> no companion", func(t *testing.T) {
		limit := mustLen(t, withoutIDTokenForKeyring(stored)) // keeps access AND scope
		tm, keyring, _ := newTMWithSizedKeyring(t, limit)
		if err := tm.saveToken("my-token", stored); err != nil {
			t.Fatalf("saveToken() error = %v", err)
		}
		if _, ok := keyring[companionKey(tm)]; ok {
			t.Error("companion entry should not exist when scope is kept in the primary entry")
		}
		if got := tm.CachedScopes("my-token"); got != nil {
			t.Errorf("CachedScopes() = %v, want nil", got)
		}
	})

	t.Run("re-save with scope kept clears a stale companion", func(t *testing.T) {
		tm, keyring, _ := newTMWithSizedKeyring(t, mustLen(t, accessOnlyStoredTokenForKeyring(stored)))
		if err := tm.saveToken("my-token", stored); err != nil {
			t.Fatalf("saveToken() error = %v", err)
		}
		if _, ok := keyring[companionKey(tm)]; !ok {
			t.Fatal("expected companion to be written on first save")
		}
		// Raise the limit so the next save keeps the scope in the primary entry.
		tm.deps.setToken = func(_ *TokenStore, name, val string) error {
			keyring[name] = val
			return nil
		}
		if err := tm.saveToken("my-token", stored); err != nil {
			t.Fatalf("saveToken() error = %v", err)
		}
		if _, ok := keyring[companionKey(tm)]; ok {
			t.Error("stale companion should have been removed when scope fits in the primary entry")
		}
	})

	t.Run("DeleteToken removes the companion", func(t *testing.T) {
		tm, keyring, _ := newTMWithSizedKeyring(t, mustLen(t, accessOnlyStoredTokenForKeyring(stored)))
		if err := tm.saveToken("my-token", stored); err != nil {
			t.Fatalf("saveToken() error = %v", err)
		}
		if _, ok := keyring[companionKey(tm)]; !ok {
			t.Fatal("expected companion before delete")
		}
		if err := tm.DeleteToken("my-token"); err != nil {
			t.Fatalf("DeleteToken() error = %v", err)
		}
		if _, ok := keyring[companionKey(tm)]; ok {
			t.Error("companion entry should be removed by DeleteToken")
		}
	})
}

// TestNeedsRefresh_AdaptiveBufferForShortLivedTokens verifies that a token
// whose lifetime is at or below the fixed 5-minute refresh buffer is still
// reused for most of its life (rather than refreshed on every call), while
// long-lived tokens keep the full 5-minute buffer.
func TestNeedsRefresh_AdaptiveBufferForShortLivedTokens(t *testing.T) {
	t.Parallel()
	tm, _, _ := newTMWithSizedKeyring(t, 1<<20)

	tests := []struct {
		name        string
		expiresIn   int
		remaining   time.Duration
		wantRefresh bool
	}{
		{name: "5m token, 4m left -> reuse", expiresIn: 300, remaining: 4 * time.Minute, wantRefresh: false},
		{name: "5m token, 20s left -> refresh", expiresIn: 300, remaining: 20 * time.Second, wantRefresh: true},
		{name: "1h token, 3m left -> refresh (full 5m buffer)", expiresIn: 3600, remaining: 3 * time.Minute, wantRefresh: true},
		{name: "1h token, 10m left -> reuse", expiresIn: 3600, remaining: 10 * time.Minute, wantRefresh: false},
		{name: "unknown lifetime, 3m left -> refresh (default buffer)", expiresIn: 0, remaining: 3 * time.Minute, wantRefresh: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok := &TokenSet{
				AccessToken: "a",
				ExpiresIn:   tc.expiresIn,
				ExpiresAt:   time.Now().Add(tc.remaining),
			}
			if got := tm.needsRefresh(tok); got != tc.wantRefresh {
				t.Errorf("needsRefresh() = %v, want %v", got, tc.wantRefresh)
			}
		})
	}
}

func TestKeyringEncodingHelpers(t *testing.T) {
	t.Parallel()
	stored := sampleStoredToken()

	noID := withoutIDTokenForKeyring(stored)
	if noID.AccessToken != stored.AccessToken || noID.Scope != stored.Scope || noID.RefreshToken != stored.RefreshToken {
		t.Error("withoutIDTokenForKeyring must keep access, scope, and refresh tokens")
	}
	if noID.IDToken != "" {
		t.Errorf("withoutIDTokenForKeyring IDToken = %q, want empty", noID.IDToken)
	}
	if noID.ExpiresAt.IsZero() {
		t.Error("withoutIDTokenForKeyring must preserve ExpiresAt")
	}

	accessOnly := accessOnlyStoredTokenForKeyring(stored)
	if accessOnly.AccessToken != stored.AccessToken || accessOnly.RefreshToken != stored.RefreshToken {
		t.Error("accessOnlyStoredTokenForKeyring must keep access and refresh tokens")
	}
	if accessOnly.IDToken != "" || accessOnly.Scope != "" {
		t.Errorf("accessOnlyStoredTokenForKeyring must drop id_token and scope, got id=%q scope=%q", accessOnly.IDToken, accessOnly.Scope)
	}
	if accessOnly.ExpiresAt.IsZero() {
		t.Error("accessOnlyStoredTokenForKeyring must preserve ExpiresAt")
	}

	if withoutIDTokenForKeyring(nil) != nil || accessOnlyStoredTokenForKeyring(nil) != nil {
		t.Error("nil input must return nil")
	}

	// keyringEncodings must be ordered largest-to-smallest and start with the
	// unmodified full token.
	encs := keyringEncodings(stored)
	if len(encs) != 5 {
		t.Fatalf("keyringEncodings returned %d encodings, want 5", len(encs))
	}
	if encs[0] != stored {
		t.Error("first encoding must be the full, unmodified token")
	}
}
