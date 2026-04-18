package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

func withStubbedSessionStatus(t *testing.T, status *SessionStatus) {
	t.Helper()
	original := buildSessionStatusFunc
	buildSessionStatusFunc = func(contextName string, ctx *config.Context, tokenName string) (*SessionStatus, error) {
		return status, nil
	}
	t.Cleanup(func() { buildSessionStatusFunc = original })
}

func TestOAuthSessionCheckResult_OK(t *testing.T) {
	exp := time.Now().Add(30 * time.Minute)
	status := &SessionStatus{
		IsOAuth:              true,
		AccessTokenPresent:   true,
		AccessTokenExpiresAt: &exp,
		RefreshTokenPresent:  true,
	}

	r := oauthSessionCheckResult(status)
	if r.Status != "ok" {
		t.Errorf("expected ok, got %q (detail: %s)", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "refresh token present") {
		t.Errorf("expected detail to mention refresh token, got %q", r.Detail)
	}
}

func TestOAuthSessionCheckResult_NoRefreshToken(t *testing.T) {
	exp := time.Now().Add(30 * time.Minute)
	status := &SessionStatus{
		IsOAuth:              true,
		AccessTokenPresent:   true,
		AccessTokenExpiresAt: &exp,
		RefreshTokenPresent:  false,
	}

	r := oauthSessionCheckResult(status)
	if r.Status != "warn" {
		t.Errorf("expected warn, got %q", r.Status)
	}
	if !strings.Contains(r.Detail, "no refresh token") {
		t.Errorf("expected detail to mention no refresh token, got %q", r.Detail)
	}
	if !strings.Contains(r.Detail, "dtctl auth login") {
		t.Errorf("expected detail to recommend login, got %q", r.Detail)
	}
}

func TestOAuthSessionCheckResult_AccessExpired(t *testing.T) {
	exp := time.Now().Add(-5 * time.Minute)
	status := &SessionStatus{
		IsOAuth:              true,
		AccessTokenPresent:   true,
		AccessTokenExpiresAt: &exp,
		RefreshTokenPresent:  true,
	}

	r := oauthSessionCheckResult(status)
	if r.Status != "warn" {
		t.Errorf("expected warn, got %q", r.Status)
	}
	if !strings.Contains(r.Detail, "access token expired") {
		t.Errorf("expected detail to mention expired access token, got %q", r.Detail)
	}
}

func TestDoctor_OAuthSessionRow_OK(t *testing.T) {
	exp := time.Now().Add(30 * time.Minute)
	withStubbedSessionStatus(t, &SessionStatus{
		IsOAuth:              true,
		AccessTokenPresent:   true,
		AccessTokenExpiresAt: &exp,
		RefreshTokenPresent:  true,
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/platform/metadata/v1/user":
			resp := map[string]interface{}{
				"userId":       "test-user-id",
				"emailAddress": "test@example.com",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = configPath

	cfg := config.NewConfig()
	cfg.SetContext("test", server.URL, "test-oauth")
	if err := cfg.SetToken("test-oauth", "dt0c01.ST.test-token-value.test-secret"); err != nil {
		t.Fatalf("failed to set token: %v", err)
	}
	cfg.CurrentContext = "test"
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	results := runDoctorChecks()

	found := false
	for _, r := range results {
		if r.Name == "OAuth session" {
			found = true
			if r.Status != "ok" {
				t.Errorf("expected OAuth session ok, got %q (detail: %s)", r.Status, r.Detail)
			}
		}
	}
	if !found {
		t.Error("expected 'OAuth session' row in doctor output")
	}
}

func TestDoctor_OAuthSessionRow_SkippedForPlatformToken(t *testing.T) {
	withStubbedSessionStatus(t, &SessionStatus{IsOAuth: false})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	originalCfgFile := cfgFile
	defer func() { cfgFile = originalCfgFile }()
	cfgFile = configPath

	cfg := config.NewConfig()
	cfg.SetContext("test", server.URL, "test-platform")
	if err := cfg.SetToken("test-platform", "dt0c01.ST.platform-token.secret"); err != nil {
		t.Fatalf("failed to set token: %v", err)
	}
	cfg.CurrentContext = "test"
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	results := runDoctorChecks()

	for _, r := range results {
		if r.Name == "OAuth session" {
			t.Errorf("did not expect 'OAuth session' row for platform token, got %q: %s", r.Status, r.Detail)
		}
	}
}

func TestPrintSessionStatusTable_DoesNotPanic(t *testing.T) {
	cases := []*SessionStatus{
		{Context: "c", Environment: "https://e", IsOAuth: false},
		func() *SessionStatus {
			exp := time.Now().Add(42 * time.Minute)
			return &SessionStatus{
				Context:              "c",
				Environment:          "https://e",
				IsOAuth:              true,
				Storage:              "macOS Keychain",
				AccessTokenPresent:   true,
				AccessTokenExpiresAt: &exp,
				RefreshTokenPresent:  true,
				GrantedScopes:        []string{"openid", "offline_access", "storage:logs:read"},
			}
		}(),
		{
			Context:             "c",
			Environment:         "https://e",
			IsOAuth:             true,
			AccessTokenPresent:  true,
			RefreshTokenPresent: false,
		},
	}

	for i, s := range cases {
		func(i int, s *SessionStatus) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("case %d panicked: %v", i, r)
				}
			}()
			printSessionStatusTable(s)
		}(i, s)
	}
}
