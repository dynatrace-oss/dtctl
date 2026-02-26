package client

import (
	"github.com/dynatrace-oss/dtctl/pkg/auth"
	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// GetTokenWithOAuthSupport retrieves a token from config with OAuth token refresh support
func GetTokenWithOAuthSupport(cfg *config.Config, tokenRef string) (string, error) {
	// First, try to get it as an OAuth token
	if config.IsKeyringAvailable() {
		// Get current context to detect environment
		ctx, err := cfg.CurrentContextObj()
		if err == nil && ctx.Environment != "" {
			// Detect environment from context
			oauthConfig := auth.OAuthConfigFromEnvironmentURL(ctx.Environment)
			tokenManager, err := auth.NewTokenManager(oauthConfig)
			if err == nil {
				// Try to get as OAuth token (will auto-refresh if needed)
				token, err := tokenManager.GetToken(tokenRef)
				if err == nil {
					return token, nil
				}
				// If error is not "not found", return it
				// Otherwise fall through to try as regular API token
			}
		}
	}
	
	// Fall back to regular token lookup
	return cfg.GetToken(tokenRef)
}

// NewFromConfigWithOAuth creates a new client from config with OAuth support
// Deprecated: Use NewFromConfig instead, which now supports OAuth tokens automatically
func NewFromConfigWithOAuth(cfg *config.Config) (*Client, error) {
	return NewFromConfig(cfg)
}
