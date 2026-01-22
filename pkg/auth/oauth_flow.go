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

	"github.com/pkg/browser"
)

const (
	prodAuthURL      = "https://sso.dynatrace.com/oauth2/authorize"
	prodTokenURL     = "https://token.dynatrace.com/sso/oauth2/token"
	prodUserInfoURL  = "https://sso.dynatrace.com/sso/oauth2/userinfo"
	callbackPort     = 3232
	// Must match the registered redirect URI for the OAuth client
	callbackPath     = "/auth/login"
	// Using the existing VS Code plugin client ID for now
	// TODO: Register dt0s12.dtctl with Dynatrace SSO team
	clientID         = "dt0s12.live-debugging-prod"
	// Scopes must match what the client is registered for
	defaultScopes    = "storage:application.snapshots:read storage:logs:read storage:buckets:read dev-obs:breakpoints:set openid app-engine:apps:run"
)

type OAuthConfig struct {
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	ClientID    string
	Scopes      []string
	Port        int
}

func DefaultOAuthConfig() *OAuthConfig {
	return &OAuthConfig{
		AuthURL:     prodAuthURL,
		TokenURL:    prodTokenURL,
		UserInfoURL: prodUserInfoURL,
		ClientID:    clientID,
		Scopes:      strings.Split(defaultScopes, " "),
		Port:        callbackPort,
	}
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
	w.Write([]byte(fmt.Sprintf(errorHTML, err.Error())))
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

const successHTML = "<!DOCTYPE html><html><head><title>Success</title><style>body{font-family:-apple-system,sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:linear-gradient(135deg,#667eea 0%,#764ba2 100%)}.container{text-align:center;background:white;padding:3rem;border-radius:10px;box-shadow:0 10px 40px rgba(0,0,0,0.2)}h1{color:#1a202c;margin-bottom:1rem}p{color:#4a5568;font-size:1.1rem}.checkmark{color:#48bb78;font-size:4rem;margin-bottom:1rem}</style></head><body><div class='container'><div class='checkmark'>✓</div><h1>Authentication Successful!</h1><p>You can close this window.</p></div></body></html>"

const errorHTML = "<!DOCTYPE html><html><head><title>Error</title><style>body{font-family:-apple-system,sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:linear-gradient(135deg,#f093fb 0%,#f5576c 100%)}.container{text-align:center;background:white;padding:3rem;border-radius:10px;box-shadow:0 10px 40px rgba(0,0,0,0.2);max-width:500px}h1{color:#1a202c;margin-bottom:1rem}.error{color:#e53e3e;font-size:3rem;margin-bottom:1rem}.message{color:#4a5568;background:#fed7d7;padding:1rem;border-radius:5px;margin-top:1rem}</style></head><body><div class='container'><div class='error'>✗</div><h1>Authentication Failed</h1><div class='message'>%s</div><p style='margin-top:1.5rem'>Please close this window and try again.</p></div></body></html>"
