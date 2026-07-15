package auth

import "testing"

// The scope-composing wrappers are this package's contract on top of the
// sdk session layer (which carries scopes opaquely): configs built here must
// arrive with the safety level's composed, duplicate-free scope set.
// (Relocated from the pre-promotion OAuth config tests.)
func TestOAuthConfigScopes(t *testing.T) {
	config := DefaultOAuthConfig()

	if len(config.Scopes) == 0 {
		t.Fatal("Scopes should not be empty")
	}

	// Verify some expected scopes are present
	expectedScopes := []string{"openid", "storage:logs:read", "storage:buckets:read", "dev-obs:breakpoints:set"}
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

func TestOAuthConfigFromEnvironmentURLWithSafety_ComposesScopes(t *testing.T) {
	cfg := OAuthConfigFromEnvironmentURLWithSafety("https://abc.apps.dynatrace.com", "readonly")
	if cfg.EnvironmentURL != "https://abc.apps.dynatrace.com" {
		t.Errorf("EnvironmentURL = %q", cfg.EnvironmentURL)
	}
	if len(cfg.Scopes) == 0 {
		t.Error("readonly scopes should not be empty")
	}
	for _, s := range cfg.Scopes {
		if s == "dev-obs:breakpoints:set" {
			t.Error("readonly config must not carry write scopes")
		}
	}
}
