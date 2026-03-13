package auth

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

const (
	// OAuthTokenPrefix is prepended to OAuth token names in keyring
	OAuthTokenPrefix = "oauth:"

	// TokenRefreshBuffer is how long before expiry we refresh tokens
	TokenRefreshBuffer = 5 * time.Minute
)

// TokenManager manages OAuth tokens including storage and refresh
type TokenManager struct {
	flow        *OAuthFlow
	tokenStore  *config.TokenStore
	environment Environment
	deps        tokenStoreDeps
}

type tokenStoreDeps struct {
	keyringAvailable func() bool
	getToken         func(ts *config.TokenStore, name string) (string, error)
	setToken         func(ts *config.TokenStore, name, token string) error
	deleteToken      func(ts *config.TokenStore, name string) error
}

// NewTokenManager creates a new token manager
func NewTokenManager(oauthConfig *OAuthConfig) (*TokenManager, error) {
	if oauthConfig == nil {
		oauthConfig = DefaultOAuthConfig()
	}

	return &TokenManager{
		flow:        &OAuthFlow{config: oauthConfig, openURL: defaultOAuthOpenURL, httpDo: defaultOAuthHTTPDo},
		tokenStore:  config.NewTokenStore(),
		environment: oauthConfig.Environment,
		deps: tokenStoreDeps{
			keyringAvailable: config.IsKeyringAvailable,
			getToken:         func(ts *config.TokenStore, name string) (string, error) { return ts.GetToken(name) },
			setToken:         func(ts *config.TokenStore, name, token string) error { return ts.SetToken(name, token) },
			deleteToken:      func(ts *config.TokenStore, name string) error { return ts.DeleteToken(name) },
		},
	}, nil
}

// StoredToken represents a stored OAuth token set
type StoredToken struct {
	TokenSet
	Name string `json:"name"`
}

// GetToken retrieves and optionally refreshes a token
func (tm *TokenManager) GetToken(tokenName string) (string, error) {
	// Load stored token
	stored, err := tm.loadToken(tokenName)
	if err != nil {
		return "", err
	}

	// If only refresh token is stored (compact keyring format), refresh immediately
	if stored.AccessToken == "" && stored.RefreshToken != "" {
		refreshed, err := tm.RefreshToken(tokenName)
		if err != nil {
			return "", fmt.Errorf("failed to refresh token from compact storage: %w", err)
		}
		return refreshed.AccessToken, nil
	}

	// Check if token needs refresh
	if tm.needsRefresh(&stored.TokenSet) {
		refreshed, err := tm.RefreshToken(tokenName)
		if err != nil {
			// If refresh fails, try to use existing token if not expired
			if time.Now().Before(stored.ExpiresAt) {
				return stored.AccessToken, nil
			}
			return "", fmt.Errorf("token expired and refresh failed: %w", err)
		}
		return refreshed.AccessToken, nil
	}

	return stored.AccessToken, nil
}

// RefreshToken refreshes an OAuth token
func (tm *TokenManager) RefreshToken(tokenName string) (*TokenSet, error) {
	// Load current token
	stored, err := tm.loadToken(tokenName)
	if err != nil {
		return nil, err
	}

	if stored.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	// Refresh the token
	newTokens, err := tm.flow.RefreshToken(stored.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	// Preserve existing refresh token if the provider does not return a new one
	if newTokens.RefreshToken == "" {
		newTokens.RefreshToken = stored.RefreshToken
	}

	// Update stored token set
	stored.TokenSet = *newTokens

	// Save refreshed token
	if err := tm.saveToken(tokenName, stored); err != nil {
		return nil, fmt.Errorf("failed to save refreshed token: %w", err)
	}

	return newTokens, nil
}

// SaveToken stores an OAuth token set
func (tm *TokenManager) SaveToken(tokenName string, tokens *TokenSet) error {
	stored := &StoredToken{
		TokenSet: *tokens,
		Name:     tokenName,
	}

	return tm.saveToken(tokenName, stored)
}

// DeleteToken removes a stored OAuth token
func (tm *TokenManager) DeleteToken(tokenName string) error {
	keyringName := tm.getKeyringName(tokenName)

	if tm.deps.keyringAvailable() {
		return tm.deps.deleteToken(tm.tokenStore, keyringName)
	}

	// OAuth tokens require keyring, so if keyring is not available,
	// the token doesn't exist in our OAuth storage
	return fmt.Errorf("OAuth token deletion requires keyring support")
}

// IsOAuthToken checks if a token name refers to an OAuth token
func IsOAuthToken(tokenName string) bool {
	// Check if stored token is OAuth (has refresh token, etc.)
	// This is determined by the presence of the oauth: prefix in keyring
	// or by checking the structure of the stored data
	return len(tokenName) > len(OAuthTokenPrefix) && tokenName[:len(OAuthTokenPrefix)] == OAuthTokenPrefix
}

// needsRefresh checks if a token needs to be refreshed
func (tm *TokenManager) needsRefresh(tokens *TokenSet) bool {
	if tokens.ExpiresAt.IsZero() {
		// If no expiry set, assume it needs refresh if more than 1 hour old
		// This shouldn't happen, but is a safety fallback
		return false
	}

	// Refresh if token expires within the buffer period
	return time.Now().Add(TokenRefreshBuffer).After(tokens.ExpiresAt)
}

// loadToken loads a token from storage
func (tm *TokenManager) loadToken(tokenName string) (*StoredToken, error) {
	keyringName := tm.getKeyringName(tokenName)

	// Try to load from keyring
	if tm.deps.keyringAvailable() {
		data, err := tm.deps.getToken(tm.tokenStore, keyringName)
		if err != nil {
			return nil, fmt.Errorf("failed to load token from keyring: %w", err)
		}

		var stored StoredToken
		if err := json.Unmarshal([]byte(data), &stored); err != nil {
			return nil, fmt.Errorf("failed to parse stored token: %w", err)
		}

		return &stored, nil
	}

	return nil, fmt.Errorf("OAuth tokens require keyring support (not available on this system)")
}

// saveToken saves a token to storage
func (tm *TokenManager) saveToken(tokenName string, stored *StoredToken) error {
	keyringName := tm.getKeyringName(tokenName)

	// Serialize token
	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to serialize token: %w", err)
	}

	// Save to keyring
	if tm.deps.keyringAvailable() {
		if err := tm.deps.setToken(tm.tokenStore, keyringName, string(data)); err != nil {
			compact := compactStoredTokenForKeyring(stored)
			compactData, marshalErr := json.Marshal(compact)
			if marshalErr != nil {
				return fmt.Errorf("failed to save token to keyring: %w", err)
			}
			if compactErr := tm.deps.setToken(tm.tokenStore, keyringName, string(compactData)); compactErr != nil {
				return fmt.Errorf("failed to save token to keyring: %w (compact fallback also failed: %v)", err, compactErr)
			}
			return nil
		}
		return nil
	}

	return fmt.Errorf("OAuth tokens require keyring support (not available on this system)")
}

func compactStoredTokenForKeyring(stored *StoredToken) *StoredToken {
	if stored == nil {
		return nil
	}

	compact := *stored
	compact.AccessToken = ""
	compact.IDToken = ""
	compact.Scope = ""
	compact.ExpiresIn = 0
	compact.ExpiresAt = time.Time{}
	return &compact
}

// getKeyringName returns the keyring storage name for a token
func (tm *TokenManager) getKeyringName(tokenName string) string {
	// Add prefix and environment to distinguish OAuth tokens per environment
	// Format: oauth:<env>:<tokenName>
	return fmt.Sprintf("%s%s:%s", OAuthTokenPrefix, tm.environment, tokenName)
}

// GetTokenInfo retrieves information about a stored OAuth token
func (tm *TokenManager) GetTokenInfo(tokenName string) (*StoredToken, error) {
	return tm.loadToken(tokenName)
}

// IsTokenExpired checks if a token is expired
func IsTokenExpired(tokens *TokenSet) bool {
	if tokens == nil {
		return true
	}

	if tokens.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().After(tokens.ExpiresAt)
}
