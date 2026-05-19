package client

import (
	"errors"
	"strings"

	"github.com/dynatrace-oss/dtctl/pkg/auth"
	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// GetTokenWithOAuthSupport retrieves a token from config with OAuth token refresh support
func GetTokenWithOAuthSupport(cfg *config.Config, tokenRef string) (string, error) {
	// First, try to get it as an OAuth token (via keyring or file-based storage)
	if config.IsOAuthStorageAvailable() {
		// Get current context to detect environment
		ctx, err := cfg.CurrentContextObj()
		if err == nil && ctx.Environment != "" {
			// Detect environment from context
			oauthConfig := auth.OAuthConfigFromEnvironmentURL(ctx.Environment)
			tokenManager, err := auth.NewTokenManager(oauthConfig)
			if err != nil {
				return "", err
			}

			// Try to get as OAuth token (will auto-refresh if needed)
			token, err := tokenManager.GetToken(tokenRef)
			if err == nil {
				return token, nil
			}

			// Fall back to regular API token lookup when:
			//   - the OAuth entry does not exist, or
			//   - the cached OAuth session was revoked server-side (invalid_grant);
			//     the auth layer has already evicted the stale cache entry.
			if !isOAuthTokenNotFoundError(err) && !errors.Is(err, auth.ErrOAuthSessionRevoked) {
				return "", err
			}
		}
	}

	// Fall back to regular token lookup
	return cfg.GetToken(tokenRef)
}

// NewFromConfigWithOAuth creates a new client from config with OAuth support.
//
// Deprecated: Use NewFromConfig instead, which now supports OAuth tokens automatically.
func NewFromConfigWithOAuth(cfg *config.Config) (*Client, error) {
	return NewFromConfig(cfg)
}

func isOAuthTokenNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "not found in keyring") ||
		strings.Contains(errMsg, "not found in file store") ||
		strings.Contains(errMsg, "token") && strings.Contains(errMsg, "not found")
}
