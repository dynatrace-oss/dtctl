package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// ErrOAuthSessionRevoked indicates the cached OAuth refresh token has been
// invalidated server-side (HTTP 400 invalid_grant). Callers should evict the
// cache and fall back to a non-OAuth credential where available.
var ErrOAuthSessionRevoked = errors.New("OAuth session revoked")

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
	// File-based storage fallback
	fileStoreAvailable func() bool
	fileGetToken       func(name string) (string, error)
	fileSetToken       func(name, token string) error
	fileDeleteToken    func(name string) error
}

// NewTokenManager creates a new token manager
func NewTokenManager(oauthConfig *OAuthConfig) (*TokenManager, error) {
	if oauthConfig == nil {
		oauthConfig = DefaultOAuthConfig()
	}

	fileStore := config.NewOAuthFileStore()

	return &TokenManager{
		flow:        &OAuthFlow{config: oauthConfig, openURL: defaultOAuthOpenURL, httpDo: defaultOAuthHTTPDo},
		tokenStore:  config.NewTokenStore(),
		environment: oauthConfig.Environment,
		deps: tokenStoreDeps{
			keyringAvailable:   config.IsKeyringAvailable,
			getToken:           func(ts *config.TokenStore, name string) (string, error) { return ts.GetToken(name) },
			setToken:           func(ts *config.TokenStore, name, token string) error { return ts.SetToken(name, token) },
			deleteToken:        func(ts *config.TokenStore, name string) error { return ts.DeleteToken(name) },
			fileStoreAvailable: func() bool { return !config.IsKeyringAvailable() && config.IsFileTokenStorage() },
			fileGetToken:       func(name string) (string, error) { return fileStore.GetToken(name) },
			fileSetToken:       func(name, token string) error { return fileStore.SetToken(name, token) },
			fileDeleteToken:    func(name string) error { return fileStore.DeleteToken(name) },
		},
	}, nil
}

// StoredToken represents a stored OAuth token set
type StoredToken struct {
	TokenSet
	Name string `json:"name"`
}

// GetToken retrieves and optionally refreshes a token.
//
// When multiple processes run in parallel (e.g. concurrent dtctl invocations)
// they may all see a compact token (no access_token) or an about-to-expire
// token and all attempt to refresh simultaneously. Because OAuth uses refresh
// token rotation, only the first refresh succeeds; the others receive
// "invalid_grant". To prevent this, we acquire a cross-process advisory lock
// before any refresh and re-read the token after acquiring it, so that the
// 2nd+ processes reuse the access_token the first one wrote.
func (tm *TokenManager) GetToken(tokenName string) (string, error) {
	// Load stored token
	stored, err := tm.loadToken(tokenName)
	if err != nil {
		return "", err
	}

	// Fast path: access_token present and not near expiry — no lock needed.
	if stored.AccessToken != "" && !tm.needsRefresh(&stored.TokenSet) {
		return stored.AccessToken, nil
	}

	// Slow path: need to refresh. Acquire a cross-process lock so that only
	// one parallel invocation actually calls the OAuth endpoint.
	unlock, lockErr := acquireRefreshLock(string(tm.environment), tokenName)
	if lockErr != nil {
		// Lock is best-effort. Log a warning so the operator knows why they may
		// still see occasional invalid_grant errors under high parallelism, but
		// do not abort — the single-process case is unaffected.
		fmt.Fprintf(os.Stderr, "dtctl: warning: could not acquire token refresh lock: %v\n", lockErr)
	} else {
		defer unlock()

		// Re-read after acquiring the lock: another process may have already
		// refreshed and saved a new token while we were waiting.
		reread, rereadErr := tm.loadToken(tokenName)
		switch {
		case rereadErr == nil && reread.AccessToken != "" && !tm.needsRefresh(&reread.TokenSet):
			// Another process already refreshed — reuse its access token.
			return reread.AccessToken, nil
		case rereadErr == nil:
			// Use the freshest data (may carry an updated refresh_token from
			// rotation). If reread failed we keep the pre-lock `stored` rather
			// than risking a nil-dereference; the compact-refresh path below
			// will surface any underlying storage error.
			stored = reread
		}
	}

	// Refresh if the access token is absent or near expiry.
	//
	// The access token may be absent for two reasons:
	//   1. Minimal-compact keyring format: only refresh_token is stored, ExpiresAt
	//      is zero. This is the most common case on macOS where the full token JSON
	//      exceeds the keychain 4096-char limit.
	//   2. Medium-compact keyring format: access_token and id_token are stripped but
	//      ExpiresAt is preserved. The re-read-after-lock path above can land here
	//      when the lock winner saved a medium-compact token: ExpiresAt would be in
	//      the future (so needsRefresh returns false) but AccessToken is still "".
	//      Checking AccessToken == "" directly avoids silently returning an empty
	//      bearer token to the caller.
	if stored.AccessToken == "" && stored.RefreshToken != "" {
		refreshed, err := tm.RefreshToken(tokenName)
		if err != nil {
			if isInvalidGrantError(err) {
				_ = tm.DeleteToken(tokenName)
				return "", fmt.Errorf("token %q: %w; re-authenticate with `dtctl auth login` or re-run `dtctl config set-credentials`", tokenName, ErrOAuthSessionRevoked)
			}
			return "", fmt.Errorf("failed to refresh token from compact storage: %w", err)
		}
		return refreshed.AccessToken, nil
	}

	// Access token is present — refresh proactively if it is near expiry.
	if tm.needsRefresh(&stored.TokenSet) {
		refreshed, err := tm.RefreshToken(tokenName)
		if err != nil {
			if isInvalidGrantError(err) {
				_ = tm.DeleteToken(tokenName)
				return "", fmt.Errorf("token %q: %w; re-authenticate with `dtctl auth login` or re-run `dtctl config set-credentials`", tokenName, ErrOAuthSessionRevoked)
			}
			// Transient failure (network, 5xx): use existing token if not yet expired
			if time.Now().Before(stored.ExpiresAt) {
				return stored.AccessToken, nil
			}
			return "", fmt.Errorf("token expired and refresh failed: %w", err)
		}
		return refreshed.AccessToken, nil
	}

	return stored.AccessToken, nil
}

// isInvalidGrantError reports whether err is an OAuth2 invalid_grant error,
// meaning the refresh token has been revoked or expired server-side.
// This string only appears in 400 responses from the OAuth token endpoint —
// never in network errors or 5xx responses — so it is a safe, specific match.
func isInvalidGrantError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid_grant")
}

// RefreshToken refreshes an OAuth token by exchanging the stored refresh token
// for a new token set and persisting the result.
//
// WARNING: RefreshToken bypasses the cross-process refresh lock. Callers that
// may run concurrently (e.g. multiple parallel dtctl invocations) should use
// GetToken instead, which holds the lock around the refresh call so that only
// one process contacts the OAuth endpoint at a time. Calling RefreshToken
// directly from concurrent goroutines or processes risks "invalid_grant" errors
// caused by OAuth refresh-token rotation invalidating the token after first use.
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

	// Fall back to file-based storage
	if tm.deps.fileStoreAvailable() {
		return tm.deps.fileDeleteToken(keyringName)
	}

	return fmt.Errorf("OAuth token deletion requires a storage backend (keyring or file); set %s=file to use file-based storage", config.EnvTokenStorage)
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

	// Fall back to file-based storage
	if tm.deps.fileStoreAvailable() {
		data, err := tm.deps.fileGetToken(keyringName)
		if err != nil {
			return nil, fmt.Errorf("failed to load token from file store: %w", err)
		}

		var stored StoredToken
		if err := json.Unmarshal([]byte(data), &stored); err != nil {
			return nil, fmt.Errorf("failed to parse stored token: %w", err)
		}

		return &stored, nil
	}

	return nil, fmt.Errorf("OAuth tokens require a storage backend (keyring or file); set %s=file to use file-based storage", config.EnvTokenStorage)
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
			// Full token is too large for the keyring. Try medium-compact first: drop the
			// access and ID token JWTs but keep scope and expiry so auth status / doctor
			// can still show meaningful information.
			medium := mediumCompactStoredTokenForKeyring(stored)
			mediumData, marshalErr := json.Marshal(medium)
			if marshalErr == nil {
				if mediumErr := tm.deps.setToken(tm.tokenStore, keyringName, string(mediumData)); mediumErr == nil {
					return nil
				}
			}
			// Still too large — fall back to minimal compact (refresh token + name only).
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

	// Fall back to file-based storage
	if tm.deps.fileStoreAvailable() {
		if err := tm.deps.fileSetToken(keyringName, string(data)); err != nil {
			return fmt.Errorf("failed to save token to file store: %w", err)
		}
		return nil
	}

	return fmt.Errorf("OAuth tokens require a storage backend (keyring or file); set %s=file to use file-based storage", config.EnvTokenStorage)
}

// mediumCompactStoredTokenForKeyring drops the large access/ID token JWTs but
// preserves scope and expiry so auth status and doctor can still display useful info.
func mediumCompactStoredTokenForKeyring(stored *StoredToken) *StoredToken {
	if stored == nil {
		return nil
	}

	compact := *stored
	compact.AccessToken = ""
	compact.IDToken = ""
	compact.ExpiresIn = 0
	return &compact
}

// compactStoredTokenForKeyring drops everything except the refresh token and name.
// Used only when mediumCompactStoredTokenForKeyring is still too large for the keyring.
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

// DecodeRefreshTokenExpiry returns the exp claim from a JWT refresh token.
// Returns zero time and false if the token is not a decodable JWT with an exp claim.
func DecodeRefreshTokenExpiry(refreshToken string) (time.Time, bool) {
	if refreshToken == "" {
		return time.Time{}, false
	}

	parts := strings.Split(refreshToken, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}

	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return time.Time{}, false
	}

	if claims.Exp == 0 {
		return time.Time{}, false
	}

	return time.Unix(claims.Exp, 0), true
}
