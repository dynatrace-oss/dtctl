package auth

import (
	"testing"
)

func TestDetectEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		envURL      string
		wantEnv     Environment
	}{
		{
			name:    "Production environment",
			envURL:  "https://abc12345.apps.dynatrace.com",
			wantEnv: EnvironmentProd,
		},
		{
			name:    "Production environment with live prefix",
			envURL:  "https://abc12345.live.dynatrace.com",
			wantEnv: EnvironmentProd,
		},
		{
			name:    "Development environment",
			envURL:  "https://abc12345.dev.apps.dynatracelabs.com",
			wantEnv: EnvironmentDev,
		},
		{
			name:    "Hardening/Sprint environment",
			envURL:  "https://abc12345.sprint.apps.dynatracelabs.com",
			wantEnv: EnvironmentHard,
		},
		{
			name:    "Unknown environment defaults to prod",
			envURL:  "https://some-other-environment.com",
			wantEnv: EnvironmentProd,
		},
		{
			name:    "Empty URL defaults to prod",
			envURL:  "",
			wantEnv: EnvironmentProd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectEnvironment(tt.envURL)
			if got != tt.wantEnv {
				t.Errorf("DetectEnvironment(%q) = %v, want %v", tt.envURL, got, tt.wantEnv)
			}
		})
	}
}

func TestOAuthConfigForEnvironment(t *testing.T) {
	tests := []struct {
		name           string
		env            Environment
		wantAuthURL    string
		wantTokenURL   string
		wantUserInfo   string
		wantClientID   string
	}{
		{
			name:         "Production configuration",
			env:          EnvironmentProd,
			wantAuthURL:  prodAuthURL,
			wantTokenURL: prodTokenURL,
			wantUserInfo: prodUserInfoURL,
			wantClientID: prodClientID,
		},
		{
			name:         "Development configuration",
			env:          EnvironmentDev,
			wantAuthURL:  devAuthURL,
			wantTokenURL: devTokenURL,
			wantUserInfo: devUserInfoURL,
			wantClientID: devClientID,
		},
		{
			name:         "Hardening configuration",
			env:          EnvironmentHard,
			wantAuthURL:  hardAuthURL,
			wantTokenURL: hardTokenURL,
			wantUserInfo: hardUserInfoURL,
			wantClientID: hardClientID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := OAuthConfigForEnvironment(tt.env)
			
			if config.AuthURL != tt.wantAuthURL {
				t.Errorf("AuthURL = %v, want %v", config.AuthURL, tt.wantAuthURL)
			}
			if config.TokenURL != tt.wantTokenURL {
				t.Errorf("TokenURL = %v, want %v", config.TokenURL, tt.wantTokenURL)
			}
			if config.UserInfoURL != tt.wantUserInfo {
				t.Errorf("UserInfoURL = %v, want %v", config.UserInfoURL, tt.wantUserInfo)
			}
			if config.ClientID != tt.wantClientID {
				t.Errorf("ClientID = %v, want %v", config.ClientID, tt.wantClientID)
			}
			if config.Environment != tt.env {
				t.Errorf("Environment = %v, want %v", config.Environment, tt.env)
			}
			if config.Port != callbackPort {
				t.Errorf("Port = %v, want %v", config.Port, callbackPort)
			}
			if len(config.Scopes) == 0 {
				t.Error("Scopes should not be empty")
			}
		})
	}
}

func TestOAuthConfigFromEnvironmentURL(t *testing.T) {
	tests := []struct {
		name         string
		envURL       string
		wantEnv      Environment
		wantClientID string
	}{
		{
			name:         "Production URL",
			envURL:       "https://abc12345.apps.dynatrace.com",
			wantEnv:      EnvironmentProd,
			wantClientID: prodClientID,
		},
		{
			name:         "Development URL",
			envURL:       "https://abc12345.dev.apps.dynatracelabs.com",
			wantEnv:      EnvironmentDev,
			wantClientID: devClientID,
		},
		{
			name:         "Hardening URL",
			envURL:       "https://abc12345.sprint.apps.dynatracelabs.com",
			wantEnv:      EnvironmentHard,
			wantClientID: hardClientID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := OAuthConfigFromEnvironmentURL(tt.envURL)
			
			if config.Environment != tt.wantEnv {
				t.Errorf("Environment = %v, want %v", config.Environment, tt.wantEnv)
			}
			if config.ClientID != tt.wantClientID {
				t.Errorf("ClientID = %v, want %v", config.ClientID, tt.wantClientID)
			}
		})
	}
}

func TestDefaultOAuthConfig(t *testing.T) {
	config := DefaultOAuthConfig()
	
	// Should return production config by default
	if config.Environment != EnvironmentProd {
		t.Errorf("Default environment = %v, want %v", config.Environment, EnvironmentProd)
	}
	if config.ClientID != prodClientID {
		t.Errorf("Default ClientID = %v, want %v", config.ClientID, prodClientID)
	}
	if config.AuthURL != prodAuthURL {
		t.Errorf("Default AuthURL = %v, want %v", config.AuthURL, prodAuthURL)
	}
}

func TestEnvironmentConstants(t *testing.T) {
	// Ensure environment constants are distinct
	envs := []Environment{EnvironmentProd, EnvironmentDev, EnvironmentHard}
	seen := make(map[Environment]bool)
	
	for _, env := range envs {
		if seen[env] {
			t.Errorf("Duplicate environment constant: %v", env)
		}
		seen[env] = true
	}
}
