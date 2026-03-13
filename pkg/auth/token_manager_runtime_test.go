package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

func storedJSON(t *testing.T, s StoredToken) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal stored token failed: %v", err)
	}
	return string(b)
}

func TestIsOAuthToken(t *testing.T) {
	if !IsOAuthToken("oauth:prod:abc") {
		t.Fatalf("expected oauth token")
	}
	if IsOAuthToken("plain-token") {
		t.Fatalf("did not expect oauth token")
	}
}

func TestTokenManagerDeleteToken(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())

	t.Run("keyring unavailable", func(t *testing.T) {
		tm.deps.keyringAvailable = func() bool { return false }
		if err := tm.DeleteToken("abc"); err == nil {
			t.Fatalf("expected delete error")
		}
	})

	t.Run("keyring available", func(t *testing.T) {
		called := false
		tm.deps.keyringAvailable = func() bool { return true }
		tm.deps.deleteToken = func(ts *config.TokenStore, name string) error {
			called = true
			return nil
		}
		if err := tm.DeleteToken("abc"); err != nil {
			t.Fatalf("unexpected delete error: %v", err)
		}
		if !called {
			t.Fatalf("expected delete call")
		}
	})
}

func TestTokenManagerLoadAndSaveTokenBranches(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())
	tm.deps.keyringAvailable = func() bool { return true }

	t.Run("load parse error", func(t *testing.T) {
		tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
			return "{invalid", nil
		}
		if _, err := tm.loadToken("abc"); err == nil {
			t.Fatalf("expected parse error")
		}
	})

	t.Run("save compact fallback success", func(t *testing.T) {
		calls := 0
		tm.deps.setToken = func(ts *config.TokenStore, name, token string) error {
			calls++
			if calls == 1 {
				return errors.New("too large")
			}
			return nil
		}
		err := tm.saveToken("abc", &StoredToken{Name: "abc", TokenSet: TokenSet{RefreshToken: "r"}})
		if err != nil {
			t.Fatalf("unexpected save error: %v", err)
		}
		if calls != 2 {
			t.Fatalf("expected two set attempts, got %d", calls)
		}
	})

	t.Run("save compact fallback fail", func(t *testing.T) {
		tm.deps.setToken = func(ts *config.TokenStore, name, token string) error {
			return errors.New("still failing")
		}
		err := tm.saveToken("abc", &StoredToken{Name: "abc", TokenSet: TokenSet{RefreshToken: "r"}})
		if err == nil {
			t.Fatalf("expected save error")
		}
	})
}

func TestTokenManagerGetToken_LoadError(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())
	tm.deps.keyringAvailable = func() bool { return true }
	tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
		return "", errors.New("missing")
	}

	_, err := tm.GetToken("abc")
	if err == nil {
		t.Fatalf("expected load error")
	}
}

func TestTokenManagerGetTokenAndRefreshPaths(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())
	tm.deps.keyringAvailable = func() bool { return true }
	tm.deps.setToken = func(ts *config.TokenStore, name, token string) error { return nil }

	t.Run("compact storage forces refresh", func(t *testing.T) {
		tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
			return storedJSON(t, StoredToken{Name: name, TokenSet: TokenSet{RefreshToken: "r1"}}), nil
		}
		tm.flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"new-a","expires_in":60}`)), Header: make(http.Header)}, nil
		}
		access, err := tm.GetToken("abc")
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}
		if access != "new-a" {
			t.Fatalf("unexpected access token: %q", access)
		}
	})

	t.Run("refresh fail but token not expired returns old", func(t *testing.T) {
		tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
			return storedJSON(t, StoredToken{Name: name, TokenSet: TokenSet{AccessToken: "old", RefreshToken: "r1", ExpiresAt: time.Now().Add(2 * time.Hour)}}), nil
		}
		tm.flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network")
		}
		access, err := tm.GetToken("abc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if access != "old" {
			t.Fatalf("unexpected fallback access token: %q", access)
		}
	})

	t.Run("refresh fail and expired returns error", func(t *testing.T) {
		tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
			return storedJSON(t, StoredToken{Name: name, TokenSet: TokenSet{AccessToken: "old", RefreshToken: "r1", ExpiresAt: time.Now().Add(-1 * time.Minute)}}), nil
		}
		tm.flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network")
		}
		_, err := tm.GetToken("abc")
		if err == nil {
			t.Fatalf("expected expired+refresh failure error")
		}
	})
}

func TestTokenManagerRefreshTokenNoRefreshToken(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())
	tm.deps.keyringAvailable = func() bool { return true }
	tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
		return storedJSON(t, StoredToken{Name: name, TokenSet: TokenSet{AccessToken: "a"}}), nil
	}

	_, err := tm.RefreshToken("abc")
	if err == nil {
		t.Fatalf("expected no refresh token error")
	}
}

func TestTokenManagerRefreshTokenAdditionalBranches(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())
	tm.deps.keyringAvailable = func() bool { return true }

	t.Run("load token error", func(t *testing.T) {
		tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
			return "", errors.New("cannot load")
		}
		_, err := tm.RefreshToken("abc")
		if err == nil {
			t.Fatalf("expected load error")
		}
	})

	t.Run("refresh request failure", func(t *testing.T) {
		tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
			return storedJSON(t, StoredToken{Name: name, TokenSet: TokenSet{RefreshToken: "r1"}}), nil
		}
		tm.flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("http down")
		}
		_, err := tm.RefreshToken("abc")
		if err == nil {
			t.Fatalf("expected refresh failure")
		}
	})

	t.Run("preserve old refresh token when provider omits it", func(t *testing.T) {
		tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
			return storedJSON(t, StoredToken{Name: name, TokenSet: TokenSet{RefreshToken: "old-refresh"}}), nil
		}
		tm.deps.setToken = func(ts *config.TokenStore, name, token string) error { return nil }
		tm.flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"a2","expires_in":60}`)), Header: make(http.Header)}, nil
		}
		tokens, err := tm.RefreshToken("abc")
		if err != nil {
			t.Fatalf("unexpected refresh error: %v", err)
		}
		if tokens.RefreshToken != "old-refresh" {
			t.Fatalf("expected preserved refresh token, got %q", tokens.RefreshToken)
		}
	})

	t.Run("save refreshed token failure", func(t *testing.T) {
		tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
			return storedJSON(t, StoredToken{Name: name, TokenSet: TokenSet{RefreshToken: "r1"}}), nil
		}
		tm.deps.setToken = func(ts *config.TokenStore, name, token string) error { return errors.New("cannot save") }
		tm.flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"a2","refresh_token":"r2","expires_in":60}`)), Header: make(http.Header)}, nil
		}
		_, err := tm.RefreshToken("abc")
		if err == nil {
			t.Fatalf("expected save failure")
		}
	})
}

func TestTokenManagerRefreshToken_NilHTTPDoFallback(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())
	tm.deps.keyringAvailable = func() bool { return true }
	tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
		return storedJSON(t, StoredToken{Name: name, TokenSet: TokenSet{RefreshToken: "r1"}}), nil
	}
	tm.deps.setToken = func(ts *config.TokenStore, name, token string) error { return nil }

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a2","refresh_token":"r2","expires_in":60}`))
	}))
	defer server.Close()

	tm.flow.config.TokenURL = server.URL
	tm.flow.httpDo = nil

	tokens, err := tm.RefreshToken("abc")
	if err != nil {
		t.Fatalf("expected nil-httpDo fallback refresh to succeed, got: %v", err)
	}
	if tokens.AccessToken != "a2" || tokens.RefreshToken != "r2" {
		t.Fatalf("unexpected refreshed tokens: %#v", tokens)
	}
}

func TestTokenManagerGetTokenInfo(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())
	tm.deps.keyringAvailable = func() bool { return true }
	tm.deps.getToken = func(ts *config.TokenStore, name string) (string, error) {
		return storedJSON(t, StoredToken{Name: name, TokenSet: TokenSet{AccessToken: "a", RefreshToken: "r"}}), nil
	}

	info, err := tm.GetTokenInfo("abc")
	if err != nil {
		t.Fatalf("GetTokenInfo failed: %v", err)
	}
	if !strings.HasSuffix(info.Name, ":abc") || info.AccessToken != "a" {
		t.Fatalf("unexpected token info: %#v", info)
	}
}

func TestTokenManagerSaveToken(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())
	tm.deps.keyringAvailable = func() bool { return true }
	tm.deps.setToken = func(ts *config.TokenStore, name, token string) error { return nil }

	err := tm.SaveToken("abc", &TokenSet{AccessToken: "a", RefreshToken: "r", ExpiresIn: 60})
	if err != nil {
		t.Fatalf("SaveToken failed: %v", err)
	}
}

func TestTokenManagerSaveToken_KeyringUnavailable(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())
	tm.deps.keyringAvailable = func() bool { return false }

	err := tm.SaveToken("abc", &TokenSet{AccessToken: "a", RefreshToken: "r", ExpiresIn: 60})
	if err == nil {
		t.Fatalf("expected keyring unavailable error")
	}
}

func TestTokenManagerLoadToken_KeyringUnavailable(t *testing.T) {
	tm, _ := NewTokenManager(DefaultOAuthConfig())
	tm.deps.keyringAvailable = func() bool { return false }

	_, err := tm.loadToken("abc")
	if err == nil {
		t.Fatalf("expected keyring unavailable error")
	}
}
