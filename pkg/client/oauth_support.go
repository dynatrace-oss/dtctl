package client

import (
	"github.com/dynatrace-oss/dtctl/pkg/auth"
	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// GetTokenWithOAuthSupport retrieves a token from config with OAuth token refresh support
func GetTokenWithOAuthSupport(cfg *config.Config, tokenRef string) (string, error) {
	// First, try to get it as an OAuth token
	if config.IsKeyringAvailable() {
		oauthConfig := auth.DefaultOAuthConfig()
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
	
	// Fall back to regular token lookup
	return cfg.GetToken(tokenRef)
}

// NewFromConfigWithOAuth creates a new client from config with OAuth support
// This is like NewFromConfig but supports OAuth tokens with automatic refresh
func NewFromConfigWithOAuth(cfg *config.Config) (*Client, error) {
	ctx, err := cfg.CurrentContextObj()
	if err != nil {
		return nil, err
	}

	token, err := GetTokenWithOAuthSupport(cfg, ctx.TokenRef)
	if err != nil {
		return nil, err
	}

	return New(ctx.Environment, token)
}
