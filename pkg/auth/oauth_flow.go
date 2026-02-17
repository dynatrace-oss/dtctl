package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/pkg/browser"
)

const (
	// Production environment
	prodAuthURL      = "https://sso.dynatrace.com/oauth2/authorize"
	prodTokenURL     = "https://token.dynatrace.com/sso/oauth2/token"
	prodUserInfoURL  = "https://sso.dynatrace.com/sso/oauth2/userinfo"
	prodClientID     = "dt0s12.live-debugging-prod"
	
	// Development environment
	devAuthURL       = "https://sso-dev.dynatracelabs.com/oauth2/authorize"
	devTokenURL      = "https://dev.token.dynatracelabs.com/sso/oauth2/token"
	devUserInfoURL   = "https://sso-dev.dynatracelabs.com/sso/oauth2/userinfo"
	devClientID      = "dt0s12.live-debugging-dev"
	
	// Hardening/Sprint environment
	hardAuthURL      = "https://sso-sprint.dynatracelabs.com/oauth2/authorize"
	hardTokenURL     = "https://hard.token.dynatracelabs.com/sso/oauth2/token"
	hardUserInfoURL  = "https://sso-sprint.dynatracelabs.com/sso/oauth2/userinfo"
	hardClientID     = "dt0s12.live-debugging-sprint"
	
	callbackPort     = 3232
	// Must match the registered redirect URI for the OAuth client
	callbackPath     = "/auth/login"
)

// Environment represents a Dynatrace environment type
type Environment string

const (
	EnvironmentProd Environment = "prod"
	EnvironmentDev  Environment = "dev"
	EnvironmentHard Environment = "hard"
)

// GetScopesForSafetyLevel returns the OAuth scopes required for a given safety level
func GetScopesForSafetyLevel(level config.SafetyLevel) []string {
	// Normalize empty string to default
	if level == "" {
		level = config.DefaultSafetyLevel
	}
	// temporarly returning the scopes that the ide plugin client uses until we have the clients for dtctl
	return []string{"storage:application.snapshots:read", "storage:logs:read", "storage:buckets:read", "dev-obs:breakpoints:set", "openid", "app-engine:apps:run"}

	// switch level {
	// case config.SafetyLevelReadOnly:
	// 	return []string{
	// 		"openid",
	// 		"document:documents:read",
	// 		"automation:workflows:read",
	// 		"slo:read",
	// 		"settings:schemas:read",
	// 		"settings:objects:read",
	// 		"storage:logs:read",
	// 		"storage:events:read",
	// 		"storage:metrics:read",
	// 		"storage:spans:read",
	// 		"storage:bizevents:read",
	// 		"storage:entities:read",
	// 		"storage:smartscape:read",
	// 		"storage:system:read",
	// 		"storage:security.events:read",
	// 		"storage:application.snapshots:read",
	// 		"storage:user.events:read",
	// 		"storage:user.sessions:read",
	// 		"storage:user.replays:read",
	// 		"storage:buckets:read",
	// 		"storage:bucket-definitions:read",
	// 		"storage:fieldsets:read",
	// 		"storage:fieldset-definitions:read",
	// 		"storage:files:read",
	// 		"storage:filter-segments:read",
	// 		"iam:users:read",
	// 		"iam:groups:read",
	// 		"notifications:read",
	// 		"vulnerabilities:read",
	// 		"davis:analyzers:read",
	// 		"app-engine:apps:run",
	// 	}
	
	// case config.SafetyLevelReadWriteMine:
	// 	return []string{
	// 		"openid",
	// 		"document:documents:read",
	// 		"document:documents:write",
	// 		"automation:workflows:read",
	// 		"automation:workflows:write",
	// 		"automation:workflows:execute",
	// 		"slo:read",
	// 		"slo:write",
	// 		"settings:schemas:read",
	// 		"settings:objects:read",
	// 		"settings:objects:write",
	// 		"storage:logs:read",
	// 		"storage:events:read",
	// 		"storage:metrics:read",
	// 		"storage:spans:read",
	// 		"storage:bizevents:read",
	// 		"storage:entities:read",
	// 		"storage:smartscape:read",
	// 		"storage:system:read",
	// 		"storage:security.events:read",
	// 		"storage:buckets:read",
	// 		"storage:bucket-definitions:read",
	// 		"storage:files:read",
	// 		"storage:files:write",
	// 		"storage:filter-segments:read",
	// 		"storage:filter-segments:write",
	// 		"davis:analyzers:read",
	// 		"davis:analyzers:execute",
	// 		"davis-copilot:conversations:execute",
	// 		"app-engine:apps:run",
	// 		"app-engine:functions:run",
	// 	}
	
	// case config.SafetyLevelReadWriteAll:
	// 	return []string{
	// 		"openid",
	// 		"document:documents:read",
	// 		"document:documents:write",
	// 		"automation:workflows:read",
	// 		"automation:workflows:write",
	// 		"automation:workflows:execute",
	// 		"slo:read",
	// 		"slo:write",
	// 		"settings:schemas:read",
	// 		"settings:objects:read",
	// 		"settings:objects:write",
	// 		"storage:logs:read",
	// 		"storage:logs:write",
	// 		"storage:events:read",
	// 		"storage:events:write",
	// 		"storage:metrics:read",
	// 		"storage:metrics:write",
	// 		"storage:spans:read",
	// 		"storage:bizevents:read",
	// 		"storage:entities:read",
	// 		"storage:smartscape:read",
	// 		"storage:system:read",
	// 		"storage:security.events:read",
	// 		"storage:application.snapshots:read",
	// 		"storage:user.events:read",
	// 		"storage:user.sessions:read",
	// 		"storage:user.replays:read",
	// 		"storage:buckets:read",
	// 		"storage:buckets:write",
	// 		"storage:bucket-definitions:read",
	// 		"storage:fieldsets:read",
	// 		"storage:fieldset-definitions:read",
	// 		"storage:files:read",
	// 		"storage:files:write",
	// 		"storage:filter-segments:read",
	// 		"storage:filter-segments:write",
	// 		"iam:users:read",
	// 		"iam:groups:read",
	// 		"notifications:read",
	// 		"vulnerabilities:read",
	// 		"davis:analyzers:read",
	// 		"davis:analyzers:execute",
	// 		"davis-copilot:conversations:execute",
	// 		"davis-copilot:nl2dql:execute",
	// 		"davis-copilot:dql2nl:execute",
	// 		"davis-copilot:document-search:execute",
	// 		"app-engine:apps:install",
	// 		"app-engine:apps:run",
	// 		"app-engine:apps:delete",
	// 		"app-engine:functions:run",
	// 		"app-engine:edge-connects:read",
	// 		"app-engine:edge-connects:write",
	// 	}
	
	// case config.SafetyLevelDangerouslyUnrestricted:
	// 	return []string{
	// 		"openid",
	// 		"document:documents:read",
	// 		"document:documents:write",
	// 		"automation:workflows:read",
	// 		"automation:workflows:write",
	// 		"automation:workflows:execute",
	// 		"slo:read",
	// 		"slo:write",
	// 		"settings:schemas:read",
	// 		"settings:objects:read",
	// 		"settings:objects:write",
	// 		"settings:objects:admin",
	// 		"storage:logs:read",
	// 		"storage:logs:write",
	// 		"storage:events:read",
	// 		"storage:events:write",
	// 		"storage:metrics:read",
	// 		"storage:metrics:write",
	// 		"storage:spans:read",
	// 		"storage:bizevents:read",
	// 		"storage:entities:read",
	// 		"storage:smartscape:read",
	// 		"storage:system:read",
	// 		"storage:security.events:read",
	// 		"storage:application.snapshots:read",
	// 		"storage:user.events:read",
	// 		"storage:user.sessions:read",
	// 		"storage:user.replays:read",
	// 		"storage:buckets:read",
	// 		"storage:buckets:write",
	// 		"storage:bucket-definitions:read",
	// 		"storage:bucket-definitions:write",
	// 		"storage:bucket-definitions:delete",
	// 		"storage:bucket-definitions:truncate",
	// 		"storage:fieldsets:read",
	// 		"storage:fieldset-definitions:read",
	// 		"storage:fieldset-definitions:write",
	// 		"storage:files:read",
	// 		"storage:files:write",
	// 		"storage:files:delete",
	// 		"storage:filter-segments:read",
	// 		"storage:filter-segments:write",
	// 		"storage:filter-segments:share",
	// 		"storage:filter-segments:delete",
	// 		"storage:filter-segments:admin",
	// 		"storage:records:delete",
	// 		"iam:users:read",
	// 		"iam:groups:read",
	// 		"iam:policies:read",
	// 		"notifications:read",
	// 		"notifications:write",
	// 		"vulnerabilities:read",
	// 		"davis:analyzers:read",
	// 		"davis:analyzers:execute",
	// 		"davis-copilot:conversations:execute",
	// 		"davis-copilot:nl2dql:execute",
	// 		"davis-copilot:dql2nl:execute",
	// 		"davis-copilot:document-search:execute",
	// 		"app-engine:apps:install",
	// 		"app-engine:apps:run",
	// 		"app-engine:apps:delete",
	// 		"app-engine:functions:run",
	// 		"app-engine:edge-connects:read",
	// 		"app-engine:edge-connects:write",
	// 	}
	
	// default:
	// 	// Default to readwrite-all
	// 	return GetScopesForSafetyLevel(config.SafetyLevelReadWriteAll)
	// }
}

type OAuthConfig struct {
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	ClientID    string
	Scopes      []string
	Port        int
	Environment Environment
	SafetyLevel config.SafetyLevel
}

// DetectEnvironment determines the environment type from a Dynatrace URL
func DetectEnvironment(environmentURL string) Environment {
	if strings.Contains(environmentURL, "apps.dynatrace.com") {
		return EnvironmentProd
	} else if strings.Contains(environmentURL, "dev.apps.dynatracelabs.com") {
		return EnvironmentDev
	} else if strings.Contains(environmentURL, "sprint.apps.dynatracelabs.com") {
		return EnvironmentHard
	}
	// Default to prod if unable to detect
	return EnvironmentProd
}

// DefaultOAuthConfig returns the default OAuth configuration for production with readwrite-all safety level
func DefaultOAuthConfig() *OAuthConfig {
	return OAuthConfigForEnvironment(EnvironmentProd, config.DefaultSafetyLevel)
}

// OAuthConfigForEnvironment creates an OAuth configuration for the specified environment and safety level
func OAuthConfigForEnvironment(env Environment, safetyLevel config.SafetyLevel) *OAuthConfig {
	var authURL, tokenURL, userInfoURL, clientID string
	
	// Normalize empty safety level to default
	if safetyLevel == "" {
		safetyLevel = config.DefaultSafetyLevel
	}
	
	switch env {
	case EnvironmentDev:
		authURL = devAuthURL
		tokenURL = devTokenURL
		userInfoURL = devUserInfoURL
		clientID = devClientID
	case EnvironmentHard:
		authURL = hardAuthURL
		tokenURL = hardTokenURL
		userInfoURL = hardUserInfoURL
		clientID = hardClientID
	default: // EnvironmentProd
		authURL = prodAuthURL
		tokenURL = prodTokenURL
		userInfoURL = prodUserInfoURL
		clientID = prodClientID
	}
	
	return &OAuthConfig{
		AuthURL:     authURL,
		TokenURL:    tokenURL,
		UserInfoURL: userInfoURL,
		ClientID:    clientID,
		Scopes:      GetScopesForSafetyLevel(safetyLevel),
		Port:        callbackPort,
		Environment: env,
		SafetyLevel: safetyLevel,
	}
}

// OAuthConfigFromEnvironmentURL creates an OAuth configuration by detecting the environment from a URL
// Uses the default safety level (readwrite-all)
func OAuthConfigFromEnvironmentURL(environmentURL string) *OAuthConfig {
	env := DetectEnvironment(environmentURL)
	return OAuthConfigForEnvironment(env, config.DefaultSafetyLevel)
}

// OAuthConfigFromEnvironmentURLWithSafety creates an OAuth configuration with specific safety level
func OAuthConfigFromEnvironmentURLWithSafety(environmentURL string, safetyLevel config.SafetyLevel) *OAuthConfig {
	env := DetectEnvironment(environmentURL)
	return OAuthConfigForEnvironment(env, safetyLevel)
}

type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

type UserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

type OAuthFlow struct {
	config         *OAuthConfig
	codeVerifier   string
	codeChallenge  string
	state          string
	server         *http.Server
	resultChan     chan *authResult
}

type authResult struct {
	tokens *TokenSet
	err    error
}

func NewOAuthFlow(config *OAuthConfig) (*OAuthFlow, error) {
	if config == nil {
		config = DefaultOAuthConfig()
	}
	
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PKCE: %w", err)
	}
	
	state, err := generateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}
	
	return &OAuthFlow{
		config:        config,
		codeVerifier:  verifier,
		codeChallenge: challenge,
		state:         state,
		resultChan:    make(chan *authResult, 1),
	}, nil
}

func (f *OAuthFlow) Start(ctx context.Context) (*TokenSet, error) {
	if err := f.startCallbackServer(); err != nil {
		return nil, fmt.Errorf("failed to start callback server: %w", err)
	}
	defer f.stopCallbackServer()
	
	authURL := f.buildAuthURL()
	
	fmt.Println("Opening browser for authentication...")
	fmt.Println("If the browser doesn't open automatically, please visit:")
	fmt.Println(authURL)
	
	if err := browser.OpenURL(authURL); err != nil {
		fmt.Printf("Failed to open browser automatically: %v\n", err)
		fmt.Println("Please open the URL above manually.")
	}
	
	select {
	case result := <-f.resultChan:
		if result.err != nil {
			return nil, result.err
		}
		return result.tokens, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("authentication cancelled: %w", ctx.Err())
	}
}

func (f *OAuthFlow) RefreshToken(refreshToken string) (*TokenSet, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {f.config.ClientID},
	}
	
	req, err := http.NewRequest("POST", f.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed: %s - %s", resp.Status, string(body))
	}
	
	var tokens TokenSet
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	
	tokens.ExpiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	
	return &tokens, nil
}

func (f *OAuthFlow) GetUserInfo(accessToken string) (*UserInfo, error) {
	req, err := http.NewRequest("GET", f.config.UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Authorization", "Bearer "+accessToken)
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user info request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user info: %s - %s", resp.Status, string(body))
	}
	
	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}
	
	return &userInfo, nil
}

func (f *OAuthFlow) buildAuthURL() string {
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {f.config.ClientID},
		"redirect_uri":          {f.getRedirectURI()},
		"scope":                 {strings.Join(f.config.Scopes, " ")},
		"state":                 {f.state},
		"code_challenge":        {f.codeChallenge},
		"code_challenge_method": {"S256"},
	}
	
	return f.config.AuthURL + "?" + params.Encode()
}

func (f *OAuthFlow) getRedirectURI() string {
	return fmt.Sprintf("http://localhost:%d%s", f.config.Port, callbackPath)
}

func (f *OAuthFlow) startCallbackServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, f.handleCallback)
	
	f.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", f.config.Port),
		Handler: mux,
	}
	
	go func() {
		if err := f.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			f.resultChan <- &authResult{err: fmt.Errorf("callback server error: %w", err)}
		}
	}()
	
	time.Sleep(100 * time.Millisecond)
	
	return nil
}

func (f *OAuthFlow) stopCallbackServer() {
	if f.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = f.server.Shutdown(ctx)
	}
}

func (f *OAuthFlow) handleCallback(w http.ResponseWriter, r *http.Request) {
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		errDesc := r.URL.Query().Get("error_description")
		f.sendError(w, fmt.Errorf("authentication failed: %s - %s", errMsg, errDesc))
		return
	}
	
	state := r.URL.Query().Get("state")
	if state != f.state {
		f.sendError(w, fmt.Errorf("invalid state parameter"))
		return
	}
	
	code := r.URL.Query().Get("code")
	if code == "" {
		f.sendError(w, fmt.Errorf("no authorization code received"))
		return
	}
	
	tokens, err := f.exchangeCode(code)
	if err != nil {
		f.sendError(w, err)
		return
	}
	
	f.sendSuccess(w)
	f.resultChan <- &authResult{tokens: tokens}
}

func (f *OAuthFlow) exchangeCode(code string) (*TokenSet, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {f.config.ClientID},
		"redirect_uri":  {f.getRedirectURI()},
		"code_verifier": {f.codeVerifier},
	}
	
	req, err := http.NewRequest("POST", f.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s - %s", resp.Status, string(body))
	}
	
	var tokens TokenSet
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	
	tokens.ExpiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	
	return &tokens, nil
}

func (f *OAuthFlow) sendSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(successHTML))
}

func (f *OAuthFlow) sendError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusBadRequest)
	html := strings.ReplaceAll(errorHTML, "{{ERROR}}", err.Error())
	w.Write([]byte(html))
	f.resultChan <- &authResult{err: err}
}

func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	
	verifier = base64.RawURLEncoding.EncodeToString(b)
	
	h := sha256.New()
	h.Write([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	
	return verifier, challenge, nil
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:length], nil
}

const successHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Login successful</title>
  <style>
    :root {
      --bg: #1C1C2D;
      --panel:#111122;
      --text: #e8e9f0;
      --muted: #9aa0a6;
    }
    * { box-sizing: border-box; }
    html, body {
      height: 100%;
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font-family: "Inter", "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    }
    .wrap {
      min-height: 100%;
      display: grid;
      place-items: center;
      padding: 4rem 1rem;
    }
    .stack {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 1.75rem;
    }
    .logo {
      width: 220px;
      height: auto;
      display: block;
      filter: drop-shadow(0 2px 12px rgba(0,0,0,0.35));
    }
    .card {
      width: min(540px, 92vw);
      background: var(--panel);
      padding: 2.25rem 2rem;
      position: relative;
      border-radius: 25px;
    }
    .icon {
      border-radius: 50%;
      display: grid;
      place-items: center;
      margin: 0 auto 1rem;
      color: var(--text);
    }
    h1 {
      font-size: clamp(1.25rem, 2.8vw, 1.6rem);
      line-height: 1.2;
      margin: 0 0 0.5rem;
      text-align: center;
      font-weight: 700;
      letter-spacing: 0.2px;
    }
    p {
      margin: 0;
      text-align: center;
      color: var(--muted);
      font-size: 0.95rem;
    }
    @media (prefers-reduced-motion: no-preference) {
      .icon { animation: float 3s ease-in-out infinite; }
      @keyframes float {
        0%, 100% { transform: translateY(0); }
        50% { transform: translateY(-3px); }
      }
    }
  </style>
</head>
<body>
  <main class="wrap">
    <div class="stack">
      <svg width="200" height="36" viewBox="0 0 200 36" fill="none" xmlns="http://www.w3.org/2000/svg">
<g clip-path="url(#clip0_3403_122778)">
<path d="M144.65 9.5502H140.05C138.75 9.5502 137.85 9.8002 137.3 10.3502C136.75 10.8752 136.475 11.8002 136.475 13.0502V28.9502H133.725V12.8502C133.75 11.6252 133.95 10.3002 134.6 9.3002C135.775 7.4252 138.025 7.0752 139.7 7.0752H144.675V9.5502H144.65Z" fill="white"/>
<path d="M126.9 26.45C125.6 26.45 124.7 26.2 124.15 25.65C123.6 25.125 123.325 24.25 123.325 23V9.55H129.1V7.075H123.325V0.5H120.575V23.15C120.6 24.375 120.8 25.7 121.45 26.7C122.625 28.575 124.875 28.925 126.55 28.925H131.525V26.45H126.9Z" fill="white"/>
<path d="M54.825 0.5V7.05H49.0499C45.5499 7.05 43.4249 8.1 42.0999 9.375C40.0749 11.35 40.075 14.175 40.075 14.475C40.075 14.825 40.075 21.175 40.075 21.525C40.075 21.825 40.0749 24.65 42.0999 26.625C43.3999 27.9 45.5249 28.95 49.0499 28.95H51.625C53.3 28.95 55.5499 28.575 56.7249 26.725C57.3499 25.725 57.5749 24.4 57.5999 23.175V0.5H54.825ZM54 25.675C53.45 26.2 52.55 26.475 51.25 26.475H48.75C46.475 26.475 45.05 25.825 44.2 24.975C43.175 23.975 42.825 22.65 42.825 21.625V14.4C42.825 13.375 43.175 12.05 44.2 11.05C45.075 10.2 46.475 9.55 48.75 9.55H54.7999V23.025C54.8249 24.25 54.55 25.125 54 25.675Z" fill="white"/>
<path d="M169.55 11.0502C170.425 10.2002 171.825 9.5502 174.1 9.5502H180.725V7.0752H174.375C170.875 7.0752 168.75 8.1252 167.425 9.4002C165.4 11.3752 165.4 14.2002 165.4 14.5002C165.4 14.7002 165.4 21.3502 165.4 21.5502C165.4 21.8502 165.4 24.6752 167.425 26.6502C168.725 27.9252 170.85 28.9752 174.375 28.9752H180.725V26.5002H174.1C171.825 26.5002 170.4 25.8502 169.55 25.0002C168.525 24.0002 168.175 22.6752 168.175 21.6502V14.4002C168.175 13.3752 168.525 12.0252 169.55 11.0502Z" fill="white"/>
<path d="M117.725 14.4748C117.725 14.1748 117.725 11.3498 115.7 9.3748C114.4 8.0998 112.275 7.0498 108.75 7.0498H102.625V9.5248H109.025C111.3 9.5248 112.725 10.1748 113.575 11.0248C114.6 12.0248 114.95 13.3498 114.95 14.3748V16.4998H106.175C104.5 16.4998 102.25 16.8748 101.075 18.7248C100.45 19.7248 100.225 21.0498 100.2 22.2748V23.1498C100.225 24.3748 100.425 25.6998 101.075 26.6998C102.25 28.5748 104.5 28.9248 106.175 28.9248H111.75C113.425 28.9248 115.675 28.5498 116.85 26.6998C117.475 25.6998 117.7 24.3748 117.725 23.1498C117.725 23.1498 117.725 15.8998 117.725 14.4748ZM114.15 25.6748C113.6 26.1998 112.7 26.4748 111.4 26.4748H106.55C105.25 26.4748 104.35 26.2248 103.8 25.6748C103.25 25.1248 103 24.2498 103 23.0248V22.4248C103 21.1748 103.275 20.2998 103.825 19.7748C104.375 19.2498 105.275 18.9748 106.575 18.9748H114.975V23.0248C114.975 24.2498 114.7 25.1248 114.15 25.6748Z" fill="white"/>
<path d="M163.225 14.4748C163.225 14.1748 163.225 11.3498 161.2 9.3748C159.9 8.0998 157.775 7.0498 154.25 7.0498H148.125V9.5248H154.525C156.8 9.5248 158.225 10.1748 159.075 11.0248C160.1 12.0248 160.45 13.3498 160.45 14.3748V16.4998H151.675C150 16.4998 147.75 16.8748 146.575 18.7248C145.95 19.7248 145.725 21.0498 145.7 22.2748V23.1498C145.725 24.3748 145.925 25.6998 146.575 26.6998C147.75 28.5748 150 28.9248 151.675 28.9248H157.25C158.925 28.9248 161.175 28.5498 162.35 26.6998C162.975 25.6998 163.2 24.3748 163.225 23.1498C163.225 23.1498 163.225 15.8998 163.225 14.4748ZM159.65 25.6748C159.1 26.1998 158.2 26.4748 156.9 26.4748H152.05C150.75 26.4748 149.85 26.2248 149.3 25.6748C148.75 25.1248 148.5 24.2498 148.5 23.0248V22.4248C148.5 21.1748 148.775 20.2998 149.325 19.7748C149.875 19.2498 150.775 18.9748 152.075 18.9748H160.475V23.0248C160.475 24.2498 160.2 25.1248 159.65 25.6748Z" fill="white"/>
<path d="M79.45 7.0498H76.575L69.6 25.1998L62.625 7.0498H59.775L68.175 28.9248L65.65 35.4998H68.525L79.45 7.0498Z" fill="white"/>
<path d="M98.05 14.4748C98.05 14.1748 98.05 11.3498 96.05 9.3748C94.775 8.1248 92.75 7.0998 89.45 7.0498H89.15C85.85 7.0998 83.825 8.1248 82.55 9.3748C80.55 11.3498 80.55 14.1748 80.55 14.4748C80.55 14.8248 80.55 27.8248 80.55 28.9498H83.3V14.3998C83.3 13.3748 83.625 12.0498 84.65 11.0498C85.5 10.2248 87.125 9.5748 89.3 9.5498C91.475 9.5748 93.1 10.2248 93.95 11.0498C94.95 12.0498 95.3 13.3748 95.3 14.3998V28.9498H98.05C98.05 27.8248 98.05 14.8248 98.05 14.4748Z" fill="white"/>
<path d="M197.75 9.3748C196.475 8.1248 194.45 7.0998 191.15 7.0498H190.85C187.55 7.0998 185.525 8.1248 184.25 9.3748C182.25 11.3498 182.25 14.1748 182.25 14.4748V21.5248C182.25 21.8248 182.25 24.6498 184.25 26.6248C185.525 27.8748 187.55 28.8998 190.85 28.9498H197.35V26.4498H191C188.825 26.4248 187.2 25.7748 186.35 24.9498C185.35 23.9498 185 22.6248 185 21.5998V19.0248H199.75V14.4748C199.775 14.1748 199.775 11.3498 197.75 9.3748ZM185.025 16.5498V14.3998C185.025 13.3748 185.35 12.0498 186.375 11.0498C187.225 10.2248 188.85 9.5748 191.025 9.5498C193.2 9.5748 194.825 10.2248 195.675 11.0498C196.675 12.0498 197.025 13.3748 197.025 14.3998V16.5498H185.025Z" fill="white"/>
<path d="M11.925 3.4246C11.475 5.7996 10.925 9.3246 10.625 12.8996C10.1 19.1996 10.425 23.4246 10.425 23.4246L1.55 31.8496C1.55 31.8496 0.875 27.1246 0.525 21.7996C0.325 18.4996 0.25 15.5996 0.25 13.8496C0.25 13.7496 0.3 13.6496 0.3 13.5496C0.3 13.4246 0.45 12.2496 1.6 11.1496C2.85 9.9496 12.075 2.7246 11.925 3.4246Z" fill="#1496FF"/>
<path d="M11.925 3.42492C11.475 5.79992 10.925 9.32492 10.625 12.8999C10.625 12.8999 0.8 11.7249 0.25 14.0999C0.25 13.9749 0.425 12.5249 1.575 11.4249C2.825 10.2249 12.075 2.72492 11.925 3.42492Z" fill="#1284EA"/>
<path d="M0.249998 13.5251C0.249998 13.7001 0.249998 13.8751 0.249998 14.0751C0.349998 13.6501 0.524998 13.3501 0.874997 12.8751C1.6 11.9501 2.775 11.7001 3.25 11.6501C5.65 11.3251 9.2 10.9501 12.775 10.8501C19.1 10.6501 23.275 11.1751 23.275 11.1751L32.15 2.75005C32.15 2.75005 27.5 1.87505 22.2 1.25005C18.725 0.825052 15.675 0.600052 13.95 0.500052C13.825 0.500052 12.6 0.350052 11.45 1.45005C10.2 2.65005 3.85 8.67505 1.3 11.1001C0.149997 12.2001 0.249998 13.4251 0.249998 13.5251Z" fill="#B4DC00"/>
<path d="M31.825 24.3003C29.425 24.6253 25.875 25.0253 22.3 25.1503C15.975 25.3503 11.775 24.8253 11.775 24.8253L2.90002 33.2753C2.90002 33.2753 7.60002 34.2003 12.9 34.8003C16.15 35.1753 19.025 35.3753 20.775 35.4753C20.9 35.4753 21.1 35.3753 21.225 35.3753C21.35 35.3753 22.575 35.1503 23.725 34.0503C24.975 32.8503 32.525 24.2253 31.825 24.3003Z" fill="#6F2DA8"/>
<path d="M31.825 24.3003C29.425 24.6253 25.875 25.0253 22.3 25.1503C22.3 25.1503 22.975 35.0253 20.6 35.4503C20.725 35.4503 22.35 35.3753 23.5 34.2753C24.75 33.0753 32.525 24.2253 31.825 24.3003Z" fill="#591F91"/>
<path d="M21.125 35.5004C20.95 35.5004 20.775 35.4754 20.575 35.4754C21.025 35.4004 21.3249 35.2504 21.7999 34.9004C22.7499 34.2254 23.05 33.0504 23.15 32.5754C23.5749 30.2004 24.1499 26.6754 24.4249 23.1004C24.9249 16.8004 24.625 12.6004 24.625 12.6004L33.5 4.15039C33.5 4.15039 34.15 8.85039 34.525 14.1754C34.75 17.6504 34.8249 20.7254 34.8499 22.4254C34.8499 22.5504 34.9499 23.7754 33.7999 24.8754C32.5499 26.0754 26.1999 32.1254 23.6749 34.5504C22.4749 35.6504 21.25 35.5004 21.125 35.5004Z" fill="#73BE28"/>
</g>
<defs>
<clipPath id="clip0_3403_122778">
<rect width="200" height="35.5" fill="white" transform="translate(0 0.25)"/>
</clipPath>
</defs>
</svg>
      <section class="card" aria-label="Login status">
        <div class="icon" aria-hidden="true">
            <svg width="32" height="33" viewBox="0 0 32 33" fill="none" xmlns="http://www.w3.org/2000/svg">
<path d="M13.6062 22.3502L22.6572 13.2992L20.9588 11.6008L13.6049 18.9548L11.3435 16.6933L9.6464 18.3904L13.6062 22.3502Z" fill="#6FC3BA"/>
<path d="M3.19995 16.7502C3.19995 23.8194 8.93071 29.5502 16 29.5502C19.027 29.5502 21.8086 28.4994 24 26.7428C26.9262 24.397 28.7999 20.7924 28.7999 16.7502C28.7999 12.708 26.9262 9.10339 24 6.7576C21.8086 5.00094 19.027 3.9502 16 3.9502C8.93071 3.9502 3.19995 9.68095 3.19995 16.7502ZM16 27.1502C10.2562 27.1502 5.59995 22.494 5.59995 16.7502C5.59995 11.0064 10.2562 6.3502 16 6.3502C18.4617 6.3502 20.7179 7.20255 22.4988 8.6302C24.8813 10.5401 26.4 13.467 26.4 16.7502C26.4 20.0334 24.8813 22.9603 22.4988 24.8702C20.7179 26.2978 18.4617 27.1502 16 27.1502Z" fill="#6FC3BA"/>
</svg>
        </div>
        <h1>Login successful</h1>
        <p>You can close this tab now.</p>
      </section>
    </div>
  </main>
</body>
</html>
`

const errorHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Login Failed</title>
  <style>
    :root {
      --bg: #1C1C2D;
      --panel:#111122;
      --text: #e8e9f0;
      --muted: #9aa0a6;
    }
    * { box-sizing: border-box; }
    html, body {
      height: 100%;
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font-family: "Inter", "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    }
    .wrap {
      min-height: 100%;
      display: grid;
      place-items: center;
      padding: 4rem 1rem;
    }
    .stack {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 1.75rem;
    }
    .logo {
      width: 220px;
      height: auto;
      display: block;
      filter: drop-shadow(0 2px 12px rgba(0,0,0,0.35));
    }
    .card {
      width: min(540px, 92vw);
      background: var(--panel);
      padding: 2.25rem 2rem;
      position: relative;
      border-radius: 25px;
    }
    .icon {
      border-radius: 50%;
      display: grid;
      place-items: center;
      margin: 0 auto 1rem;
      color: var(--text);
    }
    h1 {
      font-size: clamp(1.25rem, 2.8vw, 1.6rem);
      line-height: 1.2;
      margin: 0 0 0.5rem;
      text-align: center;
      font-weight: 700;
      letter-spacing: 0.2px;
    }
    p {
      margin: 0;
      text-align: center;
      color: var(--muted);
      font-size: 0.95rem;
    }
    .error-message {
      margin-top: 1rem;
      padding: 1rem;
      background: rgba(255, 153, 156, 0.1);
      border-radius: 8px;
      color: #FF999C;
      font-size: 0.9rem;
      word-break: break-word;
    }
    @media (prefers-reduced-motion: no-preference) {
      .icon { animation: float 3s ease-in-out infinite; }
      @keyframes float {
        0%, 100% { transform: translateY(0); }
        50% { transform: translateY(-3px); }
      }
    }
  </style>
</head>
<body>
  <main class="wrap">
    <div class="stack">
      <svg width="200" height="36" viewBox="0 0 200 36" fill="none" xmlns="http://www.w3.org/2000/svg">
<g clip-path="url(#clip0_3403_122778)">
<path d="M144.65 9.5502H140.05C138.75 9.5502 137.85 9.8002 137.3 10.3502C136.75 10.8752 136.475 11.8002 136.475 13.0502V28.9502H133.725V12.8502C133.75 11.6252 133.95 10.3002 134.6 9.3002C135.775 7.4252 138.025 7.0752 139.7 7.0752H144.675V9.5502H144.65Z" fill="white"/>
<path d="M126.9 26.45C125.6 26.45 124.7 26.2 124.15 25.65C123.6 25.125 123.325 24.25 123.325 23V9.55H129.1V7.075H123.325V0.5H120.575V23.15C120.6 24.375 120.8 25.7 121.45 26.7C122.625 28.575 124.875 28.925 126.55 28.925H131.525V26.45H126.9Z" fill="white"/>
<path d="M54.825 0.5V7.05H49.0499C45.5499 7.05 43.4249 8.1 42.0999 9.375C40.0749 11.35 40.075 14.175 40.075 14.475C40.075 14.825 40.075 21.175 40.075 21.525C40.075 21.825 40.0749 24.65 42.0999 26.625C43.3999 27.9 45.5249 28.95 49.0499 28.95H51.625C53.3 28.95 55.5499 28.575 56.7249 26.725C57.3499 25.725 57.5749 24.4 57.5999 23.175V0.5H54.825ZM54 25.675C53.45 26.2 52.55 26.475 51.25 26.475H48.75C46.475 26.475 45.05 25.825 44.2 24.975C43.175 23.975 42.825 22.65 42.825 21.625V14.4C42.825 13.375 43.175 12.05 44.2 11.05C45.075 10.2 46.475 9.55 48.75 9.55H54.7999V23.025C54.8249 24.25 54.55 25.125 54 25.675Z" fill="white"/>
<path d="M169.55 11.0502C170.425 10.2002 171.825 9.5502 174.1 9.5502H180.725V7.0752H174.375C170.875 7.0752 168.75 8.1252 167.425 9.4002C165.4 11.3752 165.4 14.2002 165.4 14.5002C165.4 14.7002 165.4 21.3502 165.4 21.5502C165.4 21.8502 165.4 24.6752 167.425 26.6502C168.725 27.9252 170.85 28.9752 174.375 28.9752H180.725V26.5002H174.1C171.825 26.5002 170.4 25.8502 169.55 25.0002C168.525 24.0002 168.175 22.6752 168.175 21.6502V14.4002C168.175 13.3752 168.525 12.0252 169.55 11.0502Z" fill="white"/>
<path d="M117.725 14.4748C117.725 14.1748 117.725 11.3498 115.7 9.3748C114.4 8.0998 112.275 7.0498 108.75 7.0498H102.625V9.5248H109.025C111.3 9.5248 112.725 10.1748 113.575 11.0248C114.6 12.0248 114.95 13.3498 114.95 14.3748V16.4998H106.175C104.5 16.4998 102.25 16.8748 101.075 18.7248C100.45 19.7248 100.225 21.0498 100.2 22.2748V23.1498C100.225 24.3748 100.425 25.6998 101.075 26.6998C102.25 28.5748 104.5 28.9248 106.175 28.9248H111.75C113.425 28.9248 115.675 28.5498 116.85 26.6998C117.475 25.6998 117.7 24.3748 117.725 23.1498C117.725 23.1498 117.725 15.8998 117.725 14.4748ZM114.15 25.6748C113.6 26.1998 112.7 26.4748 111.4 26.4748H106.55C105.25 26.4748 104.35 26.2248 103.8 25.6748C103.25 25.1248 103 24.2498 103 23.0248V22.4248C103 21.1748 103.275 20.2998 103.825 19.7748C104.375 19.2498 105.275 18.9748 106.575 18.9748H114.975V23.0248C114.975 24.2498 114.7 25.1248 114.15 25.6748Z" fill="white"/>
<path d="M163.225 14.4748C163.225 14.1748 163.225 11.3498 161.2 9.3748C159.9 8.0998 157.775 7.0498 154.25 7.0498H148.125V9.5248H154.525C156.8 9.5248 158.225 10.1748 159.075 11.0248C160.1 12.0248 160.45 13.3498 160.45 14.3748V16.4998H151.675C150 16.4998 147.75 16.8748 146.575 18.7248C145.95 19.7248 145.725 21.0498 145.7 22.2748V23.1498C145.725 24.3748 145.925 25.6998 146.575 26.6998C147.75 28.5748 150 28.9248 151.675 28.9248H157.25C158.925 28.9248 161.175 28.5498 162.35 26.6998C162.975 25.6998 163.2 24.3748 163.225 23.1498C163.225 23.1498 163.225 15.8998 163.225 14.4748ZM159.65 25.6748C159.1 26.1998 158.2 26.4748 156.9 26.4748H152.05C150.75 26.4748 149.85 26.2248 149.3 25.6748C148.75 25.1248 148.5 24.2498 148.5 23.0248V22.4248C148.5 21.1748 148.775 20.2998 149.325 19.7748C149.875 19.2498 150.775 18.9748 152.075 18.9748H160.475V23.0248C160.475 24.2498 160.2 25.1248 159.65 25.6748Z" fill="white"/>
<path d="M79.45 7.0498H76.575L69.6 25.1998L62.625 7.0498H59.775L68.175 28.9248L65.65 35.4998H68.525L79.45 7.0498Z" fill="white"/>
<path d="M98.05 14.4748C98.05 14.1748 98.05 11.3498 96.05 9.3748C94.775 8.1248 92.75 7.0998 89.45 7.0498H89.15C85.85 7.0998 83.825 8.1248 82.55 9.3748C80.55 11.3498 80.55 14.1748 80.55 14.4748C80.55 14.8248 80.55 27.8248 80.55 28.9498H83.3V14.3998C83.3 13.3748 83.625 12.0498 84.65 11.0498C85.5 10.2248 87.125 9.5748 89.3 9.5498C91.475 9.5748 93.1 10.2248 93.95 11.0498C94.95 12.0498 95.3 13.3748 95.3 14.3998V28.9498H98.05C98.05 27.8248 98.05 14.8248 98.05 14.4748Z" fill="white"/>
<path d="M197.75 9.3748C196.475 8.1248 194.45 7.0998 191.15 7.0498H190.85C187.55 7.0998 185.525 8.1248 184.25 9.3748C182.25 11.3498 182.25 14.1748 182.25 14.4748V21.5248C182.25 21.8248 182.25 24.6498 184.25 26.6248C185.525 27.8748 187.55 28.8998 190.85 28.9498H197.35V26.4498H191C188.825 26.4248 187.2 25.7748 186.35 24.9498C185.35 23.9498 185 22.6248 185 21.5998V19.0248H199.75V14.4748C199.775 14.1748 199.775 11.3498 197.75 9.3748ZM185.025 16.5498V14.3998C185.025 13.3748 185.35 12.0498 186.375 11.0498C187.225 10.2248 188.85 9.5748 191.025 9.5498C193.2 9.5748 194.825 10.2248 195.675 11.0498C196.675 12.0498 197.025 13.3748 197.025 14.3998V16.5498H185.025Z" fill="white"/>
<path d="M11.925 3.4246C11.475 5.7996 10.925 9.3246 10.625 12.8996C10.1 19.1996 10.425 23.4246 10.425 23.4246L1.55 31.8496C1.55 31.8496 0.875 27.1246 0.525 21.7996C0.325 18.4996 0.25 15.5996 0.25 13.8496C0.25 13.7496 0.3 13.6496 0.3 13.5496C0.3 13.4246 0.45 12.2496 1.6 11.1496C2.85 9.9496 12.075 2.7246 11.925 3.4246Z" fill="#1496FF"/>
<path d="M11.925 3.42492C11.475 5.79992 10.925 9.32492 10.625 12.8999C10.625 12.8999 0.8 11.7249 0.25 14.0999C0.25 13.9749 0.425 12.5249 1.575 11.4249C2.825 10.2249 12.075 2.72492 11.925 3.42492Z" fill="#1284EA"/>
<path d="M0.249998 13.5251C0.249998 13.7001 0.249998 13.8751 0.249998 14.0751C0.349998 13.6501 0.524998 13.3501 0.874997 12.8751C1.6 11.9501 2.775 11.7001 3.25 11.6501C5.65 11.3251 9.2 10.9501 12.775 10.8501C19.1 10.6501 23.275 11.1751 23.275 11.1751L32.15 2.75005C32.15 2.75005 27.5 1.87505 22.2 1.25005C18.725 0.825052 15.675 0.600052 13.95 0.500052C13.825 0.500052 12.6 0.350052 11.45 1.45005C10.2 2.65005 3.85 8.67505 1.3 11.1001C0.149997 12.2001 0.249998 13.4251 0.249998 13.5251Z" fill="#B4DC00"/>
<path d="M31.825 24.3003C29.425 24.6253 25.875 25.0253 22.3 25.1503C15.975 25.3503 11.775 24.8253 11.775 24.8253L2.90002 33.2753C2.90002 33.2753 7.60002 34.2003 12.9 34.8003C16.15 35.1753 19.025 35.3753 20.775 35.4753C20.9 35.4753 21.1 35.3753 21.225 35.3753C21.35 35.3753 22.575 35.1503 23.725 34.0503C24.975 32.8503 32.525 24.2253 31.825 24.3003Z" fill="#6F2DA8"/>
<path d="M31.825 24.3003C29.425 24.6253 25.875 25.0253 22.3 25.1503C22.3 25.1503 22.975 35.0253 20.6 35.4503C20.725 35.4503 22.35 35.3753 23.5 34.2753C24.75 33.0753 32.525 24.2253 31.825 24.3003Z" fill="#591F91"/>
<path d="M21.125 35.5004C20.95 35.5004 20.775 35.4754 20.575 35.4754C21.025 35.4004 21.3249 35.2504 21.7999 34.9004C22.7499 34.2254 23.05 33.0504 23.15 32.5754C23.5749 30.2004 24.1499 26.6754 24.4249 23.1004C24.9249 16.8004 24.625 12.6004 24.625 12.6004L33.5 4.15039C33.5 4.15039 34.15 8.85039 34.525 14.1754C34.75 17.6504 34.8249 20.7254 34.8499 22.4254C34.8499 22.5504 34.9499 23.7754 33.7999 24.8754C32.5499 26.0754 26.1999 32.1254 23.6749 34.5504C22.4749 35.6504 21.25 35.5004 21.125 35.5004Z" fill="#73BE28"/>
</g>
<defs>
<clipPath id="clip0_3403_122778">
<rect width="200" height="35.5" fill="white" transform="translate(0 0.25)"/>
</clipPath>
</defs>
</svg>
      <section class="card" aria-label="Login status">
        <div class="icon" aria-hidden="true">
            <svg width="32" height="33" viewBox="0 0 32 33" fill="none" xmlns="http://www.w3.org/2000/svg">
<path d="M14.7997 10.23H17.1997V17.43H14.7997V10.23Z" fill="#FF999C"/>
<path d="M17.5997 20.63C17.5997 19.7464 16.8834 19.03 15.9997 19.03C15.1161 19.03 14.3997 19.7464 14.3997 20.63C14.3997 21.5137 15.1161 22.23 15.9997 22.23C16.8834 22.23 17.5997 21.5137 17.5997 20.63Z" fill="#FF999C"/>
<path fill-rule="evenodd" clip-rule="evenodd" d="M14.8684 2.67078C15.4932 2.04594 16.5063 2.04594 17.1311 2.67078L29.579 15.1186C30.2038 15.7435 30.2038 16.7565 29.579 17.3814L17.1311 29.8292C16.5063 30.454 15.4932 30.454 14.8684 29.8292L2.42053 17.3814C1.79569 16.7565 1.79569 15.7435 2.42053 15.1186L14.8684 2.67078ZM27.3162 16.25L15.9997 4.93352L4.68327 16.25L15.9997 27.5665L27.3162 16.25Z" fill="#FF999C"/>
</svg>
        </div>
        <h1>Login Failed</h1>
        <p>Please close this window and try again.</p>
        <div class="error-message">{{ERROR}}</div>
      </section>
    </div>
  </main>
</body>
</html>
`
