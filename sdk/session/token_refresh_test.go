package session

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestEnableTokenRefresh_RetriesWith FreshToken: a 401 triggers the resolve
// callback and the request is retried with the new token.
func TestEnableTokenRefresh_RetriesWithFreshToken(t *testing.T) {
	var attempts, resolves atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		if r.Header.Get("Authorization") != "Bearer fresh-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":401,"message":"JWT token is expired"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	c, err := NewForTesting(server.URL, "expired-token")
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	// NewForTesting disables retries; the refresh hook needs one retry slot.
	c.HTTP().SetRetryCount(2)
	c.EnableTokenRefresh(func(rejected string) (string, error) {
		resolves.Add(1)
		if rejected != "expired-token" {
			t.Errorf("resolve got rejected token %q, want %q", rejected, "expired-token")
		}
		return "fresh-token", nil
	})

	resp, err := c.HTTP().R().Get("/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode(), resp.String())
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2 (401 then retry)", got)
	}
	if got := resolves.Load(); got != 1 {
		t.Errorf("resolve calls = %d, want 1", got)
	}
	if c.Token() != "fresh-token" {
		t.Errorf("client token = %q, want fresh-token", c.Token())
	}
}

// TestEnableTokenRefresh_SameTokenGivesUp: when resolve returns the token the
// server just rejected (static API tokens), the 401 surfaces without retries.
func TestEnableTokenRefresh_SameTokenGivesUp(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c, err := NewForTesting(server.URL, "static-token")
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	c.HTTP().SetRetryCount(2)
	c.EnableTokenRefresh(func(rejected string) (string, error) {
		return rejected, nil // nothing fresher exists
	})

	resp, err := c.HTTP().R().Get("/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode())
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (no pointless retry)", got)
	}
}

// TestEnableTokenRefresh_RateLimited: a token refreshed moments ago that is
// rejected again means dead credentials — no second refresh, no retry loop.
func TestEnableTokenRefresh_RateLimited(t *testing.T) {
	var attempts, resolves atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized) // rejects every token
	}))
	defer server.Close()

	c, err := NewForTesting(server.URL, "expired-token")
	if err != nil {
		t.Fatalf("NewForTesting: %v", err)
	}
	c.HTTP().SetRetryCount(3)
	c.EnableTokenRefresh(func(rejected string) (string, error) {
		resolves.Add(1)
		return "fresh-token", nil
	})

	resp, err := c.HTTP().R().Get("/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode())
	}
	if got := resolves.Load(); got != 1 {
		t.Errorf("resolve calls = %d, want 1 (rate-limited after the first)", got)
	}
	// One original attempt + one retry with the fresh token; the second 401
	// falls inside the refresh window with the same token → stop.
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}
