package session

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
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

	// scopeCompanionSuffix names a keyring entry that holds only the granted
	// scope string. It preserves the scope list — permission names, not a
	// secret — when the primary token entry had to drop it to fit the keyring
	// item size limit while keeping the (larger) access token cached.
	scopeCompanionSuffix = ":scopes"
)

// TokenManager manages OAuth tokens including storage and refresh
type TokenManager struct {
	flow        *OAuthFlow
	tokenStore  *TokenStore
	environment Environment
	deps        tokenStoreDeps
	warn        func(format string, args ...any)
}

// SetWarnFunc replaces the destination for non-fatal warnings (e.g. a failed
// best-effort refresh-lock acquisition). The default writes to stderr, which
// suits a CLI; embedders that own the terminal (a TUI like dynatui) should
// route warnings into their own surface instead. A nil fn silences warnings.
func (tm *TokenManager) SetWarnFunc(fn func(format string, args ...any)) {
	if fn == nil {
		fn = func(string, ...any) {}
	}
	tm.warn = fn
}

// warnf reports a non-fatal condition via the configured warn hook.
func (tm *TokenManager) warnf(format string, args ...any) {
	tm.warn(format, args...)
}

type tokenStoreDeps struct {
	keyringAvailable func() bool
	getToken         func(ts *TokenStore, name string) (string, error)
	setToken         func(ts *TokenStore, name, token string) error
	deleteToken      func(ts *TokenStore, name string) error
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

	fileStore := NewOAuthFileStore()

	return &TokenManager{
		flow:        &OAuthFlow{config: oauthConfig, openURL: defaultOAuthOpenURL, httpDo: defaultOAuthHTTPDo},
		tokenStore:  NewTokenStore(),
		environment: oauthConfig.Environment,
		warn: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "dtctl: warning: "+format+"\n", args...)
		},
		deps: tokenStoreDeps{
			keyringAvailable:   IsKeyringAvailable,
			getToken:           func(ts *TokenStore, name string) (string, error) { return ts.GetToken(name) },
			setToken:           func(ts *TokenStore, name, token string) error { return ts.SetToken(name, token) },
			deleteToken:        func(ts *TokenStore, name string) error { return ts.DeleteToken(name) },
			fileStoreAvailable: func() bool { return !IsKeyringAvailable() && IsFileTokenStorage() },
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
		tm.warnf("could not acquire token refresh lock: %v", lockErr)
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
		refreshed, err := tm.refreshTokenLocked(tokenName)
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
		refreshed, err := tm.refreshTokenLocked(tokenName)
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

// RefreshToken forces a refresh of an OAuth token by exchanging the stored
// refresh token for a new token set and persisting the result.
//
// The refresh runs under the same cross-process lock GetToken uses: OAuth
// refresh-token rotation invalidates a refresh token after first use, so an
// unguarded forced refresh (e.g. a long-running consumer reacting to a 401 while a
// parallel dtctl invocation refreshes on expiry) would strand one side with
// "invalid_grant". After acquiring the lock the token is re-read — if another
// process refreshed while we waited (the stored access token changed and is
// not near expiry), that fresher token set is returned instead of spending
// another refresh. A genuinely-forced refresh (nothing changed while waiting)
// always proceeds, so a token the server rejects despite looking valid can
// never be returned to the caller unrefreshed. Like GetToken, the lock is
// best-effort: on lock failure a warning is printed and the refresh proceeds
// unguarded.
func (tm *TokenManager) RefreshToken(tokenName string) (*TokenSet, error) {
	// Snapshot before locking so a refresh completed by another process while
	// we wait is detectable as a change.
	before, beforeErr := tm.loadToken(tokenName)

	unlock, lockErr := acquireRefreshLock(string(tm.environment), tokenName)
	if lockErr != nil {
		tm.warnf("could not acquire token refresh lock: %v", lockErr)
	} else {
		defer unlock()

		if beforeErr == nil {
			reread, err := tm.loadToken(tokenName)
			if err == nil && reread.AccessToken != "" &&
				reread.AccessToken != before.AccessToken &&
				!tm.needsRefresh(&reread.TokenSet) {
				tokens := reread.TokenSet
				return &tokens, nil
			}
		}
	}

	return tm.refreshTokenLocked(tokenName)
}

// refreshTokenLocked performs the refresh-token exchange and persists the
// result, without touching the cross-process refresh lock. Callers must hold
// the lock (GetToken does; RefreshToken acquires it) — an unguarded call
// races refresh-token rotation across processes.
func (tm *TokenManager) refreshTokenLocked(tokenName string) (*TokenSet, error) {
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
		err := tm.deps.deleteToken(tm.tokenStore, keyringName)
		// Best-effort cleanup of the scope companion and any file-based fallback token.
		_ = tm.deps.deleteToken(tm.tokenStore, keyringName+scopeCompanionSuffix)
		_ = tm.deps.fileDeleteToken(keyringName)
		return err
	}

	// Fall back to file-based storage
	if tm.deps.fileStoreAvailable() {
		return tm.deps.fileDeleteToken(keyringName)
	}

	return fmt.Errorf("OAuth token deletion requires a storage backend (keyring or file); set %s=file to use file-based storage", EnvTokenStorage)
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

	// Refresh if token expires within the buffer period.
	return time.Now().Add(refreshBufferFor(tokens)).After(tokens.ExpiresAt)
}

// refreshBufferFor returns how long before expiry a token should be refreshed.
//
// The default buffer (TokenRefreshBuffer, 5 min) suits long-lived tokens, but
// some Dynatrace SSO environments issue short-lived access tokens (~5 min).
// When the token's lifetime is at or below the fixed buffer, every GetToken
// call sees the token as "about to expire" and refreshes — an SSO round-trip
// on every invocation that defeats the cached-token fast path. When the token
// lifetime is known (ExpiresIn), cap the buffer at a fraction of it (with a
// small floor) so a freshly cached token is reused for most of its life while
// still being refreshed slightly ahead of expiry.
func refreshBufferFor(tokens *TokenSet) time.Duration {
	buffer := TokenRefreshBuffer
	if tokens.ExpiresIn > 0 {
		if capped := time.Duration(tokens.ExpiresIn) * time.Second / 5; capped < buffer {
			buffer = capped
		}
		if buffer < 30*time.Second {
			buffer = 30 * time.Second
		}
	}
	return buffer
}

// loadToken loads a token from storage
func (tm *TokenManager) loadToken(tokenName string) (*StoredToken, error) {
	keyringName := tm.getKeyringName(tokenName)

	// Try to load from keyring
	if tm.deps.keyringAvailable() {
		data, err := tm.deps.getToken(tm.tokenStore, keyringName)
		if err == nil {
			var stored StoredToken
			if err := json.Unmarshal([]byte(data), &stored); err != nil {
				return nil, fmt.Errorf("failed to parse stored token: %w", err)
			}
			return &stored, nil
		}
		// Non-"not found" errors are fatal (e.g. keyring locked or corrupted).
		if !strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("failed to load token from keyring: %w", err)
		}
		// Token not in keyring — may have been saved to file as a size-limit fallback.
		// Also try file store before returning an error.
		if data, fileErr := tm.deps.fileGetToken(keyringName); fileErr == nil {
			var stored StoredToken
			if err := json.Unmarshal([]byte(data), &stored); err != nil {
				return nil, fmt.Errorf("failed to parse stored token: %w", err)
			}
			return &stored, nil
		}
		return nil, fmt.Errorf("failed to load token from keyring: %w", err)
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

	return nil, fmt.Errorf("OAuth tokens require a storage backend (keyring or file); set %s=file to use file-based storage", EnvTokenStorage)
}

// saveToken saves a token to storage.
//
// The full token set (access + ID token JWTs + scope) can exceed a keyring
// backend's per-item size limit — most notably the macOS Keychain, where the
// full JSON routinely overflows. Rather than immediately dropping the access
// token (which would defeat GetToken's fast path and force an SSO refresh on
// every invocation), saveToken tries progressively smaller keyring encodings
// via keyringEncodings, keeping the access token as long as it fits. Only when
// even the access token alone is too large does it fall back to an
// access-token-less form, and finally to file storage that can hold the full
// token.
func (tm *TokenManager) saveToken(tokenName string, stored *StoredToken) error {
	keyringName := tm.getKeyringName(tokenName)

	// Serialize the full token for the file-store fallback below.
	fullData, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to serialize token: %w", err)
	}

	// Save to keyring
	if tm.deps.keyringAvailable() {
		var lastErr error
		for _, enc := range keyringEncodings(stored) {
			data, marshalErr := json.Marshal(enc)
			if marshalErr != nil {
				lastErr = marshalErr
				continue
			}
			if setErr := tm.deps.setToken(tm.tokenStore, keyringName, string(data)); setErr == nil {
				tm.syncScopeCompanion(keyringName, stored.Scope, enc.Scope)
				return nil
			} else {
				lastErr = setErr
			}
		}
		// Even the minimal (refresh-token-only) encoding could not be written.
		// If that was a size limit, fall back to file storage, which can hold
		// the full token (including the access token, so the fast path still
		// works). Any other error (locked/corrupt keyring) is returned as-is.
		if isKeyringTooLargeErr(lastErr) {
			if fileErr := tm.deps.fileSetToken(keyringName, string(fullData)); fileErr == nil {
				// Remove any stale keyring entry (and scope companion) so loadToken
				// reads the full token — scope included — from the file next time.
				_ = tm.deps.deleteToken(tm.tokenStore, keyringName)
				_ = tm.deps.deleteToken(tm.tokenStore, keyringName+scopeCompanionSuffix)
				return nil
			}
		}
		return fmt.Errorf("failed to save token to keyring: %w", lastErr)
	}

	// Fall back to file-based storage
	if tm.deps.fileStoreAvailable() {
		if err := tm.deps.fileSetToken(keyringName, string(fullData)); err != nil {
			return fmt.Errorf("failed to save token to file store: %w", err)
		}
		return nil
	}

	return fmt.Errorf("OAuth tokens require a storage backend (keyring or file); set %s=file to use file-based storage", EnvTokenStorage)
}

// isKeyringTooLargeErr reports whether err is a keyring "data too large" error.
// On macOS the go-keyring library surfaces errSecDataTooLarge (-25313) as
// "data passed to Set was too big"; the security(1) CLI exits with code 161.
func isKeyringTooLargeErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "too big") || strings.Contains(msg, "exit status 161")
}

// keyringEncodings returns candidate serializations of stored for the keyring,
// ordered from most complete to most compact. saveToken tries each in turn and
// keeps the first that fits the backend's size limit.
//
// The ordering deliberately prefers retaining the access token (and its
// expiry) over the id_token and the scope string, because a cached access
// token lets GetToken's fast path skip the ~1s SSO refresh round-trip on every
// invocation. The id_token is unused past login, and when the scope string is
// dropped saveToken preserves it in a scope-companion entry (see
// syncScopeCompanion), so dropping either from the primary entry is lossless.
// Only when the access token itself does not fit do we fall back to the
// access-token-less medium/minimal forms.
func keyringEncodings(stored *StoredToken) []*StoredToken {
	return []*StoredToken{
		stored,                                     // full: access + id + scope + refresh
		withoutIDTokenForKeyring(stored),           // drop id_token; keep access + scope
		accessOnlyStoredTokenForKeyring(stored),    // drop id_token + scope; keep access (scope -> companion)
		mediumCompactStoredTokenForKeyring(stored), // drop access + id; keep scope (no fast path)
		compactStoredTokenForKeyring(stored),       // minimal: refresh token + name only
	}
}

// withoutIDTokenForKeyring drops only the id_token JWT (unused by dtctl beyond
// login), keeping the access token, scope, and expiry so both GetToken's fast
// path and auth-status scope display keep working.
func withoutIDTokenForKeyring(stored *StoredToken) *StoredToken {
	if stored == nil {
		return nil
	}

	compact := *stored
	compact.IDToken = ""
	return &compact
}

// accessOnlyStoredTokenForKeyring keeps the access token and its expiry (so the
// fast path works) but drops the id_token JWT and the large scope string. The
// dropped scope is preserved in the scope-companion entry so auth status still
// shows the full granted scopes.
func accessOnlyStoredTokenForKeyring(stored *StoredToken) *StoredToken {
	if stored == nil {
		return nil
	}

	compact := *stored
	compact.IDToken = ""
	compact.Scope = ""
	return &compact
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

// syncScopeCompanion keeps the scope-companion keyring entry in sync with what
// the primary token encoding actually stored. When the primary encoding had to
// drop the scope string to fit the keyring size limit (origScope is non-empty
// but the stored encoding's scope is empty), the scope is preserved in a
// companion entry so auth status and doctor can still show the full granted
// scopes without keeping the large scope string in the primary entry. In every
// other case any stale companion is removed. The scope list is not a secret, so
// this introduces no credential exposure. Best-effort: errors are ignored since
// the companion is a display aid, not required for authentication.
func (tm *TokenManager) syncScopeCompanion(keyringName, origScope, storedScope string) {
	companion := keyringName + scopeCompanionSuffix
	if origScope != "" && storedScope == "" {
		_ = tm.deps.setToken(tm.tokenStore, companion, origScope)
		return
	}
	_ = tm.deps.deleteToken(tm.tokenStore, companion)
}

// CachedScopes returns the granted scopes preserved in the scope-companion
// keyring entry, or nil when there is none. It is used as a fallback for scope
// display when the primary token entry dropped the scope string to fit size
// limits. Only consulted for keyring storage; the file store keeps the full
// token (scope included), so no companion is needed there.
func (tm *TokenManager) CachedScopes(tokenName string) []string {
	if !tm.deps.keyringAvailable() {
		return nil
	}
	companion := tm.getKeyringName(tokenName) + scopeCompanionSuffix
	v, err := tm.deps.getToken(tm.tokenStore, companion)
	if err != nil || v == "" {
		return nil
	}
	return strings.Fields(v)
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
