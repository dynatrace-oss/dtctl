package auth

import (
	"testing"

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
			config: OAuthConfigForEnvironment(EnvironmentProd, config.DefaultSafetyLevel),
			wantEnv: EnvironmentProd,
			wantErr: false,
		},
		{
			name:    "Development config",
			config: OAuthConfigForEnvironment(EnvironmentDev, config.DefaultSafetyLevel),
			wantEnv: EnvironmentDev,
			wantErr: false,
		},
		{
			name:    "Hardening config",
			config: OAuthConfigForEnvironment(EnvironmentHard, config.DefaultSafetyLevel),
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
