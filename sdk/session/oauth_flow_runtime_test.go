package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestOAuthFlowStartCancelled(t *testing.T) {
	cfg := DefaultOAuthConfig()
	cfg.Port = 0
	flow, err := NewOAuthFlow(cfg)
	if err != nil {
		t.Fatalf("NewOAuthFlow failed: %v", err)
	}
	flow.openURL = func(url string) error { return errors.New("browser unavailable") }

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err = flow.Start(ctx)
	if err == nil || !strings.Contains(err.Error(), "authentication cancelled") {
		t.Fatalf("expected cancelled error, got %v", err)
	}
}

func TestOAuthFlowRefreshTokenAndUserInfo(t *testing.T) {
	t.Run("refresh token success", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())
		flow.httpDo = func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", req.Method)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"a","refresh_token":"r2","expires_in":60}`)),
				Header:     make(http.Header),
			}, nil
		}

		tokens, err := flow.RefreshToken("r1")
		if err != nil {
			t.Fatalf("RefreshToken failed: %v", err)
		}
		if tokens.AccessToken != "a" || tokens.RefreshToken != "r2" {
			t.Fatalf("unexpected tokens: %#v", tokens)
		}
	})

	t.Run("refresh token non-200", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())
		flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Body:       io.NopCloser(strings.NewReader("bad")),
				Header:     make(http.Header),
			}, nil
		}
		_, err := flow.RefreshToken("r1")
		if err == nil {
			t.Fatalf("expected refresh error")
		}
	})

	t.Run("user info success", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())
		flow.httpDo = func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Authorization") != "Bearer tok" {
				t.Fatalf("missing auth header")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"sub":"u1","email":"a@example.invalid"}`)),
				Header:     make(http.Header),
			}, nil
		}
		user, err := flow.GetUserInfo("tok")
		if err != nil {
			t.Fatalf("GetUserInfo failed: %v", err)
		}
		if user.Sub != "u1" {
			t.Fatalf("unexpected user: %#v", user)
		}
	})

	t.Run("user info error status", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())
		flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Body:       io.NopCloser(strings.NewReader("nope")),
				Header:     make(http.Header),
			}, nil
		}
		_, err := flow.GetUserInfo("tok")
		if err == nil {
			t.Fatalf("expected user info error")
		}
	})
}

func TestOAuthFlowCallbackHandlerBranches(t *testing.T) {
	newFlow := func(t *testing.T) *OAuthFlow {
		t.Helper()
		flow, err := NewOAuthFlow(DefaultOAuthConfig())
		if err != nil {
			t.Fatalf("NewOAuthFlow failed: %v", err)
		}
		return flow
	}

	t.Run("error query parameter", func(t *testing.T) {
		flow := newFlow(t)
		req := httptest.NewRequest(http.MethodGet, callbackPath+"?error=access_denied&error_description=nope", nil)
		rr := httptest.NewRecorder()
		flow.handleCallback(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("unexpected code: %d", rr.Code)
		}
		res := <-flow.resultChan
		if res.err == nil {
			t.Fatalf("expected callback error")
		}
	})

	t.Run("invalid state", func(t *testing.T) {
		flow := newFlow(t)
		req := httptest.NewRequest(http.MethodGet, callbackPath+"?state=bad&code=abc", nil)
		rr := httptest.NewRecorder()
		flow.handleCallback(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("unexpected code: %d", rr.Code)
		}
	})

	t.Run("missing code", func(t *testing.T) {
		flow := newFlow(t)
		req := httptest.NewRequest(http.MethodGet, callbackPath+"?state="+url.QueryEscape(flow.state), nil)
		rr := httptest.NewRecorder()
		flow.handleCallback(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("unexpected code: %d", rr.Code)
		}
	})

	t.Run("exchange code success", func(t *testing.T) {
		flow := newFlow(t)
		flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"a","refresh_token":"r","expires_in":60}`)),
				Header:     make(http.Header),
			}, nil
		}
		req := httptest.NewRequest(http.MethodGet, callbackPath+"?state="+url.QueryEscape(flow.state)+"&code=abc", nil)
		rr := httptest.NewRecorder()
		flow.handleCallback(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected code: %d body=%s", rr.Code, rr.Body.String())
		}
		res := <-flow.resultChan
		if res.err != nil || res.tokens == nil || res.tokens.AccessToken != "a" {
			t.Fatalf("unexpected auth result: %#v", res)
		}
	})
}

func TestOAuthFlowStartStopCallbackServer(t *testing.T) {
	t.Run("start and stop", func(t *testing.T) {
		cfg := DefaultOAuthConfig()
		cfg.Port = 0
		flow, _ := NewOAuthFlow(cfg)
		if err := flow.startCallbackServer(); err != nil {
			t.Fatalf("startCallbackServer failed: %v", err)
		}
		flow.stopCallbackServer()
	})

	t.Run("bind error", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen failed: %v", err)
		}
		defer ln.Close()
		port := ln.Addr().(*net.TCPAddr).Port

		cfg := DefaultOAuthConfig()
		cfg.Port = port
		flow, _ := NewOAuthFlow(cfg)
		err = flow.startCallbackServer()
		if err == nil {
			t.Fatalf("expected bind error")
		}
	})
}

func TestPublishResultSingleDelivery(t *testing.T) {
	flow, _ := NewOAuthFlow(DefaultOAuthConfig())
	flow.publishResult(&authResult{err: fmt.Errorf("first")})
	flow.publishResult(&authResult{err: fmt.Errorf("second")})
	res := <-flow.resultChan
	if res == nil || res.err == nil || !strings.Contains(res.err.Error(), "first") {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestOAuthFlowExchangeCodeBranches(t *testing.T) {
	t.Run("invalid token URL request creation", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())
		flow.config.TokenURL = "://bad"
		_, err := flow.exchangeCode("abc")
		if err == nil || !strings.Contains(err.Error(), "failed to create request") {
			t.Fatalf("expected create request error, got %v", err)
		}
	})

	t.Run("http request error", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())
		flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		}
		_, err := flow.exchangeCode("abc")
		if err == nil || !strings.Contains(err.Error(), "token exchange request failed") {
			t.Fatalf("expected request failed error, got %v", err)
		}
	})

	t.Run("decode error", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())
		flow.httpDo = func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("{not-json")),
				Header:     make(http.Header),
			}, nil
		}
		_, err := flow.exchangeCode("abc")
		if err == nil || !strings.Contains(err.Error(), "failed to decode token response") {
			t.Fatalf("expected decode error, got %v", err)
		}
	})
}

func TestOAuthFlowClientCredentials(t *testing.T) {
	t.Run("posts the grant and returns the token", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())

		var got url.Values
		flow.httpDo = func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", req.Method)
			}
			if ct := req.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
				t.Fatalf("unexpected content type: %s", ct)
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("reading body: %v", err)
			}
			got, err = url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parsing body: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(
					`{"access_token":"at","token_type":"Bearer","expires_in":300,"scope":"storage:logs:read"}`)),
				Header: make(http.Header),
			}, nil
		}

		tokens, err := flow.ClientCredentials("cid", "csecret", "urn:dtaccount:abc", []string{"storage:logs:read", "storage:events:read"})
		if err != nil {
			t.Fatalf("ClientCredentials failed: %v", err)
		}

		if got.Get("grant_type") != "client_credentials" {
			t.Fatalf("unexpected grant_type: %s", got.Get("grant_type"))
		}
		if got.Get("client_id") != "cid" || got.Get("client_secret") != "csecret" {
			t.Fatalf("credentials not sent: %v", got)
		}
		if got.Get("resource") != "urn:dtaccount:abc" {
			t.Fatalf("unexpected resource: %s", got.Get("resource"))
		}
		if got.Get("scope") != "storage:logs:read storage:events:read" {
			t.Fatalf("unexpected scope: %q", got.Get("scope"))
		}
		if tokens.AccessToken != "at" {
			t.Fatalf("unexpected access token: %#v", tokens)
		}
		// RFC 6749 section 4.4.3: no refresh token is issued for this grant.
		if tokens.RefreshToken != "" {
			t.Fatalf("unexpected refresh token: %q", tokens.RefreshToken)
		}
		if tokens.ExpiresAt.IsZero() {
			t.Fatal("ExpiresAt was not derived from expires_in")
		}
	})

	t.Run("omits optional parameters when unset", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())

		var got url.Values
		flow.httpDo = func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			got, _ = url.ParseQuery(string(body))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"at","expires_in":60}`)),
				Header:     make(http.Header),
			}, nil
		}

		if _, err := flow.ClientCredentials("cid", "csecret", "", nil); err != nil {
			t.Fatalf("ClientCredentials failed: %v", err)
		}
		if _, ok := got["resource"]; ok {
			t.Fatal("resource should be omitted when empty")
		}
		if _, ok := got["scope"]; ok {
			t.Fatal("scope should be omitted when empty")
		}
	})

	t.Run("requires client id and secret", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())
		flow.httpDo = func(*http.Request) (*http.Response, error) {
			t.Fatal("no request should be made without credentials")
			return nil, nil
		}

		if _, err := flow.ClientCredentials("", "csecret", "", nil); err == nil ||
			!strings.Contains(err.Error(), "client ID") {
			t.Fatalf("expected client ID error, got %v", err)
		}
		if _, err := flow.ClientCredentials("cid", "", "", nil); err == nil ||
			!strings.Contains(err.Error(), "client secret") {
			t.Fatalf("expected client secret error, got %v", err)
		}
	})

	t.Run("redacts the client secret from error bodies", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())
		flow.httpDo = func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_client","sent":"supersecret"}`)),
				Header:     make(http.Header),
			}, nil
		}

		_, err := flow.ClientCredentials("cid", "supersecret", "", nil)
		if err == nil {
			t.Fatal("expected an error")
		}
		if strings.Contains(err.Error(), "supersecret") {
			t.Fatalf("client secret leaked into error: %v", err)
		}
		if !strings.Contains(err.Error(), "<redacted>") {
			t.Fatalf("expected redaction marker, got %v", err)
		}
	})

	t.Run("rejects a response with no access token", func(t *testing.T) {
		flow, _ := NewOAuthFlow(DefaultOAuthConfig())
		flow.httpDo = func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"expires_in":300}`)),
				Header:     make(http.Header),
			}, nil
		}

		if _, err := flow.ClientCredentials("cid", "csecret", "", nil); err == nil ||
			!strings.Contains(err.Error(), "no access token") {
			t.Fatalf("expected missing access token error, got %v", err)
		}
	})
}
