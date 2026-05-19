package client

import (
	"errors"
	"fmt"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/auth"
	"github.com/dynatrace-oss/dtctl/pkg/config"
)

func TestErrOAuthSessionRevoked_IsRecognised(t *testing.T) {
	wrapped := fmt.Errorf("token %q: %w; re-authenticate", "my-token", auth.ErrOAuthSessionRevoked)
	if !errors.Is(wrapped, auth.ErrOAuthSessionRevoked) {
		t.Fatal("errors.Is should match wrapped ErrOAuthSessionRevoked")
	}
	// And it should NOT match isOAuthTokenNotFoundError (the message no longer says "not found").
	if isOAuthTokenNotFoundError(wrapped) {
		t.Error("isOAuthTokenNotFoundError should not match a session-revoked error")
	}
}

func TestIsOAuthTokenNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "keyring not found", err: errors.New("failed to load token from keyring: token \"oauth:prod:my-token\" not found in keyring"), want: true},
		{name: "generic token not found", err: errors.New("token not found"), want: true},
		{name: "refresh token expired", err: errors.New("failed to refresh token: invalid_grant"), want: false},
		{name: "network", err: errors.New("token refresh request failed: dial tcp timeout"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOAuthTokenNotFoundError(tt.err); got != tt.want {
				t.Errorf("isOAuthTokenNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTokenWithOAuthSupport_FallsBackWithoutOAuthContext(t *testing.T) {
	t.Setenv(config.EnvDisableKeyring, "1")

	cfg := config.NewConfig()
	if err := cfg.SetToken("api-token", "dt0c01.test"); err != nil {
		t.Fatalf("SetToken() error = %v", err)
	}

	got, err := GetTokenWithOAuthSupport(cfg, "api-token")
	if err != nil {
		t.Fatalf("GetTokenWithOAuthSupport() error = %v", err)
	}
	if got != "dt0c01.test" {
		t.Fatalf("GetTokenWithOAuthSupport() = %q, want %q", got, "dt0c01.test")
	}
}
