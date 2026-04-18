package auth

import (
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

func TestTokenManager_getKeyringName(t *testing.T) {
	tests := []struct {
		name        string
		environment Environment
		tokenName   string
		want        string
	}{
		{
			name:        "Production environment token",
			environment: EnvironmentProd,
			tokenName:   "my-token",
			want:        "oauth:prod:my-token",
		},
		{
			name:        "Development environment token",
			environment: EnvironmentDev,
			tokenName:   "dev-token",
			want:        "oauth:dev:dev-token",
		},
		{
			name:        "Hardening environment token",
			environment: EnvironmentHard,
			tokenName:   "sprint-token",
			want:        "oauth:hard:sprint-token",
		},
		{
			name:        "Token with special characters",
			environment: EnvironmentProd,
			tokenName:   "my-env-oauth",
			want:        "oauth:prod:my-env-oauth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a token manager with the specified environment
			config := OAuthConfigForEnvironment(tt.environment, config.DefaultSafetyLevel)
			tm, err := NewTokenManager(config)
			if err != nil {
				t.Fatalf("Failed to create TokenManager: %v", err)
			}

			got := tm.getKeyringName(tt.tokenName)
			if got != tt.want {
				t.Errorf("getKeyringName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewTokenManager(t *testing.T) {
	tests := []struct {
		name    string
		config  *OAuthConfig
		wantEnv Environment
		wantErr bool
	}{
		{
			name:    "Production config",
			config:  OAuthConfigForEnvironment(EnvironmentProd, config.DefaultSafetyLevel),
			wantEnv: EnvironmentProd,
			wantErr: false,
		},
		{
			name:    "Development config",
			config:  OAuthConfigForEnvironment(EnvironmentDev, config.DefaultSafetyLevel),
			wantEnv: EnvironmentDev,
			wantErr: false,
		},
		{
			name:    "Hardening config",
			config:  OAuthConfigForEnvironment(EnvironmentHard, config.DefaultSafetyLevel),
			wantEnv: EnvironmentHard,
			wantErr: false,
		},
		{
			name:    "Nil config defaults to production",
			config:  nil,
			wantEnv: EnvironmentProd,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm, err := NewTokenManager(tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewTokenManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && tm.environment != tt.wantEnv {
				t.Errorf("TokenManager.environment = %v, want %v", tm.environment, tt.wantEnv)
			}
		})
	}
}

func TestTokenManager_EnvironmentIsolation(t *testing.T) {
	// Test that tokens from different environments have different keyring names
	tokenName := "same-token-name"

	prodConfig := OAuthConfigForEnvironment(EnvironmentProd, config.DefaultSafetyLevel)
	prodTM, err := NewTokenManager(prodConfig)
	if err != nil {
		t.Fatalf("Failed to create prod TokenManager: %v", err)
	}

	devConfig := OAuthConfigForEnvironment(EnvironmentDev, config.DefaultSafetyLevel)
	devTM, err := NewTokenManager(devConfig)
	if err != nil {
		t.Fatalf("Failed to create dev TokenManager: %v", err)
	}

	hardConfig := OAuthConfigForEnvironment(EnvironmentHard, config.DefaultSafetyLevel)
	hardTM, err := NewTokenManager(hardConfig)
	if err != nil {
		t.Fatalf("Failed to create hard TokenManager: %v", err)
	}

	prodKey := prodTM.getKeyringName(tokenName)
	devKey := devTM.getKeyringName(tokenName)
	hardKey := hardTM.getKeyringName(tokenName)

	// All three should be different
	if prodKey == devKey || prodKey == hardKey || devKey == hardKey {
		t.Errorf("Token keys should be different across environments: prod=%s, dev=%s, hard=%s",
			prodKey, devKey, hardKey)
	}

	// Verify the expected formats
	expectedProd := "oauth:prod:same-token-name"
	expectedDev := "oauth:dev:same-token-name"
	expectedHard := "oauth:hard:same-token-name"

	if prodKey != expectedProd {
		t.Errorf("Production key = %v, want %v", prodKey, expectedProd)
	}
	if devKey != expectedDev {
		t.Errorf("Development key = %v, want %v", devKey, expectedDev)
	}
	if hardKey != expectedHard {
		t.Errorf("Hardening key = %v, want %v", hardKey, expectedHard)
	}
}

func TestCompactStoredTokenForKeyring(t *testing.T) {
	expiresAt := time.Now().Add(30 * time.Minute).UTC()
	stored := &StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			AccessToken:  "access",
			RefreshToken: "refresh",
			IDToken:      "id",
			TokenType:    "Bearer",
			ExpiresIn:    1800,
			Scope:        "openid profile",
			ExpiresAt:    expiresAt,
		},
	}

	compact := compactStoredTokenForKeyring(stored)
	if compact == nil {
		t.Fatalf("compactStoredTokenForKeyring() returned nil")
	}

	if compact.Name != stored.Name {
		t.Errorf("Name = %q, want %q", compact.Name, stored.Name)
	}
	if compact.RefreshToken != stored.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", compact.RefreshToken, stored.RefreshToken)
	}
	if compact.TokenType != stored.TokenType {
		t.Errorf("TokenType = %q, want %q", compact.TokenType, stored.TokenType)
	}

	if compact.AccessToken != "" {
		t.Errorf("AccessToken = %q, want empty", compact.AccessToken)
	}
	if compact.IDToken != "" {
		t.Errorf("IDToken = %q, want empty", compact.IDToken)
	}
	if compact.Scope != "" {
		t.Errorf("Scope = %q, want empty", compact.Scope)
	}
	if compact.ExpiresIn != 0 {
		t.Errorf("ExpiresIn = %d, want 0", compact.ExpiresIn)
	}
	if !compact.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero value", compact.ExpiresAt)
	}
}

func TestMediumCompactStoredTokenForKeyring(t *testing.T) {
	expiresAt := time.Now().Add(30 * time.Minute).UTC()
	stored := &StoredToken{
		Name: "my-token",
		TokenSet: TokenSet{
			AccessToken:  "access",
			RefreshToken: "refresh",
			IDToken:      "id",
			TokenType:    "Bearer",
			ExpiresIn:    1800,
			Scope:        "openid offline_access profile",
			ExpiresAt:    expiresAt,
		},
	}

	compact := mediumCompactStoredTokenForKeyring(stored)
	if compact == nil {
		t.Fatalf("mediumCompactStoredTokenForKeyring() returned nil")
	}

	if compact.Name != stored.Name {
		t.Errorf("Name = %q, want %q", compact.Name, stored.Name)
	}
	if compact.RefreshToken != stored.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", compact.RefreshToken, stored.RefreshToken)
	}
	if compact.TokenType != stored.TokenType {
		t.Errorf("TokenType = %q, want %q", compact.TokenType, stored.TokenType)
	}
	if compact.Scope != stored.Scope {
		t.Errorf("Scope = %q, want %q", compact.Scope, stored.Scope)
	}
	if !compact.ExpiresAt.Equal(expiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", compact.ExpiresAt, expiresAt)
	}

	if compact.AccessToken != "" {
		t.Errorf("AccessToken = %q, want empty", compact.AccessToken)
	}
	if compact.IDToken != "" {
		t.Errorf("IDToken = %q, want empty", compact.IDToken)
	}
	if compact.ExpiresIn != 0 {
		t.Errorf("ExpiresIn = %d, want 0", compact.ExpiresIn)
	}

	if mediumCompactStoredTokenForKeyring(nil) != nil {
		t.Error("expected nil input to return nil")
	}
}
