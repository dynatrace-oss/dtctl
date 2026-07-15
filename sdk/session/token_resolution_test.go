package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
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
