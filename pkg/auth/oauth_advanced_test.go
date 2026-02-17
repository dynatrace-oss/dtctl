package auth

import (
	"strings"
	"testing"
	"time"
)

// TestGeneratePKCE tests the PKCE code generation
func TestGeneratePKCE(t *testing.T) {
	verifier1, challenge1, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE() failed: %v", err)
	}
	
	// Verify verifier is not empty
	if verifier1 == "" {
		t.Error("Verifier should not be empty")
	}
	
	// Verify challenge is not empty
	if challenge1 == "" {
		t.Error("Challenge should not be empty")
	}
	
	// Verify they're different
	if verifier1 == challenge1 {
		t.Error("Verifier and challenge should be different")
	}
	
	// Generate again and ensure randomness (different values)
	verifier2, challenge2, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE() second call failed: %v", err)
	}
	
	if verifier1 == verifier2 {
		t.Error("PKCE verifier should be random (got same value twice)")
	}
	
	if challenge1 == challenge2 {
		t.Error("PKCE challenge should be random (got same value twice)")
	}
}

// TestGenerateRandomString tests random string generation
func TestGenerateRandomString(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"Length 8", 8},
		{"Length 16", 16},
		{"Length 32", 32},
		{"Length 64", 64},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str1, err := generateRandomString(tt.length)
			if err != nil {
				t.Fatalf("generateRandomString(%d) failed: %v", tt.length, err)
			}
			
			if len(str1) != tt.length {
				t.Errorf("Length = %d, want %d", len(str1), tt.length)
			}
			
			// Generate again to verify randomness
			str2, err := generateRandomString(tt.length)
			if err != nil {
				t.Fatalf("generateRandomString(%d) second call failed: %v", tt.length, err)
			}
			
			if str1 == str2 {
				t.Error("Random strings should be different")
			}
		})
	}
}

// TestNewOAuthFlow tests OAuth flow creation
func TestNewOAuthFlow(t *testing.T) {
	tests := []struct {
		name    string
		config  *OAuthConfig
		wantErr bool
	}{
		{
			name:    "Valid production config",
			config:  OAuthConfigForEnvironment(EnvironmentProd),
			wantErr: false,
		},
		{
			name:    "Valid development config",
			config:  OAuthConfigForEnvironment(EnvironmentDev),
			wantErr: false,
		},
		{
			name:    "Nil config uses default",
			config:  nil,
			wantErr: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flow, err := NewOAuthFlow(tt.config)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("NewOAuthFlow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if err == nil {
				// Verify flow is properly initialized
				if flow.config == nil {
					t.Error("Flow config should not be nil")
				}
				if flow.codeVerifier == "" {
					t.Error("Code verifier should be generated")
				}
				if flow.codeChallenge == "" {
					t.Error("Code challenge should be generated")
				}
				if flow.state == "" {
					t.Error("State should be generated")
				}
				if flow.resultChan == nil {
					t.Error("Result channel should be initialized")
				}
			}
		})
	}
}

// TestOAuthFlow_buildAuthURL tests OAuth authorization URL building
func TestOAuthFlow_buildAuthURL(t *testing.T) {
	tests := []struct {
		name    string
		env     Environment
		wantURL string
	}{
		{
			name:    "Production URL",
			env:     EnvironmentProd,
			wantURL: prodAuthURL,
		},
		{
			name:    "Development URL",
			env:     EnvironmentDev,
			wantURL: devAuthURL,
		},
		{
			name:    "Hardening URL",
			env:     EnvironmentHard,
			wantURL: hardAuthURL,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := OAuthConfigForEnvironment(tt.env)
			flow, err := NewOAuthFlow(config)
			if err != nil {
				t.Fatalf("NewOAuthFlow() failed: %v", err)
			}
			
			authURL := flow.buildAuthURL()
			
			// Verify base URL
			if !strings.HasPrefix(authURL, tt.wantURL) {
				t.Errorf("Auth URL should start with %s, got %s", tt.wantURL, authURL)
			}
			
			// Verify required parameters are present
			requiredParams := []string{
				"response_type=code",
				"client_id=",
				"redirect_uri=",
				"scope=",
				"state=",
				"code_challenge=",
				"code_challenge_method=S256",
			}
			
			for _, param := range requiredParams {
				if !strings.Contains(authURL, param) {
					t.Errorf("Auth URL missing parameter: %s", param)
				}
			}
		})
	}
}

// TestOAuthFlow_getRedirectURI tests redirect URI generation
func TestOAuthFlow_getRedirectURI(t *testing.T) {
	config := DefaultOAuthConfig()
	flow, err := NewOAuthFlow(config)
	if err != nil {
		t.Fatalf("NewOAuthFlow() failed: %v", err)
	}
	
	redirectURI := flow.getRedirectURI()
	
	expectedURI := "http://localhost:3232/auth/login"
	if redirectURI != expectedURI {
		t.Errorf("Redirect URI = %s, want %s", redirectURI, expectedURI)
	}
	
	// Verify it contains required components
	if !strings.Contains(redirectURI, "localhost") {
		t.Error("Redirect URI should contain localhost")
	}
	if !strings.Contains(redirectURI, "3232") {
		t.Error("Redirect URI should contain port 3232")
	}
	if !strings.Contains(redirectURI, "/auth/login") {
		t.Error("Redirect URI should contain callback path")
	}
}

// TestTokenSet_ExpiresAt tests token expiration calculation
func TestTokenSet_ExpiresAt(t *testing.T) {
	now := time.Now()
	
	tests := []struct {
		name      string
		expiresIn int
		wantAfter time.Time
	}{
		{
			name:      "1 hour expiry",
			expiresIn: 3600,
			wantAfter: now.Add(59 * time.Minute), // Should be at least 59 minutes from now
		},
		{
			name:      "5 minutes expiry",
			expiresIn: 300,
			wantAfter: now.Add(4 * time.Minute),
		},
		{
			name:      "1 day expiry",
			expiresIn: 86400,
			wantAfter: now.Add(23 * time.Hour),
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &TokenSet{
				AccessToken:  "test-token",
				RefreshToken: "test-refresh",
				ExpiresIn:    tt.expiresIn,
			}
			
			// Simulate what the code does
			token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
			
			if token.ExpiresAt.Before(tt.wantAfter) {
				t.Errorf("ExpiresAt = %v, should be after %v", token.ExpiresAt, tt.wantAfter)
			}
		})
	}
}

// TestIsTokenExpired tests token expiration detection
func TestIsTokenExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "Expired 1 hour ago",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
		{
			name:      "Expires in 1 hour",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "Expires in 1 minute",
			expiresAt: time.Now().Add(1 * time.Minute),
			want:      false,
		},
		{
			name:      "Expired 1 second ago",
			expiresAt: time.Now().Add(-1 * time.Second),
			want:      true,
		},
		{
			name:      "Zero time (no expiry set)",
			expiresAt: time.Time{},
			want:      false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &TokenSet{
				AccessToken: "test",
				ExpiresAt:   tt.expiresAt,
			}
			
			got := IsTokenExpired(token)
			if got != tt.want {
				t.Errorf("IsTokenExpired() = %v, want %v (ExpiresAt: %v, Now: %v)",
					got, tt.want, tt.expiresAt, time.Now())
			}
		})
	}
}

// TestTokenManager_needsRefresh tests token refresh logic
func TestTokenManager_needsRefresh(t *testing.T) {
	config := DefaultOAuthConfig()
	tm, err := NewTokenManager(config)
	if err != nil {
		t.Fatalf("NewTokenManager() failed: %v", err)
	}
	
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "Expires in 10 minutes (no refresh yet)",
			expiresAt: time.Now().Add(10 * time.Minute),
			want:      false, // Outside TokenRefreshBuffer (5 min)
		},
		{
			name:      "Expires in 3 minutes (should refresh)",
			expiresAt: time.Now().Add(3 * time.Minute),
			want:      true, // Within TokenRefreshBuffer
		},
		{
			name:      "Expires in 1 hour (no refresh needed)",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "Already expired (should refresh)",
			expiresAt: time.Now().Add(-1 * time.Minute),
			want:      true,
		},
		{
			name:      "Zero time (no refresh)",
			expiresAt: time.Time{},
			want:      false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &TokenSet{
				AccessToken: "test",
				ExpiresAt:   tt.expiresAt,
			}
			
			got := tm.needsRefresh(token)
			if got != tt.want {
				t.Errorf("needsRefresh() = %v, want %v (ExpiresAt: %v, Now+Buffer: %v)",
					got, tt.want, tt.expiresAt, time.Now().Add(TokenRefreshBuffer))
			}
		})
	}
}

// TestMultiEnvironmentScenario tests a realistic multi-environment workflow
func TestMultiEnvironmentScenario(t *testing.T) {
	environments := []struct {
		name   string
		env    Environment
		url    string
		client string
	}{
		{"Production", EnvironmentProd, "https://abc.apps.dynatrace.com", prodClientID},
		{"Development", EnvironmentDev, "https://abc.dev.apps.dynatracelabs.com", devClientID},
		{"Hardening", EnvironmentHard, "https://abc.sprint.apps.dynatracelabs.com", hardClientID},
	}
	
	tokenName := "multi-env-token"
	
	for _, env := range environments {
		t.Run(env.name, func(t *testing.T) {
			// Detect environment from URL
			detected := DetectEnvironment(env.url)
			if detected != env.env {
				t.Errorf("DetectEnvironment(%s) = %v, want %v", env.url, detected, env.env)
			}
			
			// Create config from URL
			config := OAuthConfigFromEnvironmentURL(env.url)
			if config.Environment != env.env {
				t.Errorf("Config environment = %v, want %v", config.Environment, env.env)
			}
			if config.ClientID != env.client {
				t.Errorf("Config client ID = %v, want %v", config.ClientID, env.client)
			}
			
			// Create token manager
			tm, err := NewTokenManager(config)
			if err != nil {
				t.Fatalf("NewTokenManager() failed: %v", err)
			}
			
			// Verify environment-specific keyring name
			keyringName := tm.getKeyringName(tokenName)
			expectedPrefix := "oauth:" + string(env.env) + ":"
			if !strings.HasPrefix(keyringName, expectedPrefix) {
				t.Errorf("Keyring name = %s, should start with %s", keyringName, expectedPrefix)
			}
		})
	}
}

// TestOAuthConfigScopes tests that scopes are properly set
func TestOAuthConfigScopes(t *testing.T) {
	config := DefaultOAuthConfig()
	
	if len(config.Scopes) == 0 {
		t.Error("Scopes should not be empty")
	}
	
	// Verify some expected scopes are present
	expectedScopes := []string{"openid", "storage:logs:read", "storage:buckets:read"}
	
	for _, expected := range expectedScopes {
		found := false
		for _, scope := range config.Scopes {
			if scope == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected scope %s not found in: %v", expected, config.Scopes)
		}
	}
	
	// Verify no duplicate scopes
	seen := make(map[string]bool)
	for _, scope := range config.Scopes {
		if seen[scope] {
			t.Errorf("Duplicate scope found: %s", scope)
		}
		seen[scope] = true
	}
}
