//go:build integration
// +build integration

package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/resources/platformtoken"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// setupAccountTokenTest checks required env vars and returns an httpclient.Client
// pointed at the account management API, plus the account UUID.
func setupAccountTokenTest(t *testing.T) (*httpclient.Client, string) {
	t.Helper()

	token := os.Getenv("DTCTL_ACCOUNT_TOKEN")
	if token == "" {
		t.Skip("Skipping account token test: DTCTL_ACCOUNT_TOKEN not set")
	}

	accountUUID := os.Getenv("DTCTL_ACCOUNT_UUID")
	if accountUUID == "" {
		t.Skip("Skipping account token test: DTCTL_ACCOUNT_UUID not set")
	}

	baseURL := os.Getenv("DTCTL_ACCOUNT_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.dynatrace.com"
	}

	c, err := httpclient.New(baseURL, httpclient.WithToken(token))
	if err != nil {
		t.Fatalf("failed to create account client: %v", err)
	}

	return c, accountUUID
}

func TestAccountTokenLifecycle(t *testing.T) {
	c, accountUUID := setupAccountTokenTest(t)

	userUUID := os.Getenv("DTCTL_ACCOUNT_USER_UUID")
	if userUUID == "" {
		t.Skip("Skipping account token lifecycle test: DTCTL_ACCOUNT_USER_UUID not set")
	}

	handler := platformtoken.NewHandler(c, accountUUID)

	// Step 1: List tokens (read-only, always safe)
	t.Log("Step 1: Listing platform tokens...")
	tokens, err := handler.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	t.Logf("Found %d existing platform tokens", len(tokens))

	// Step 2: Create a token
	t.Log("Step 2: Creating platform token...")
	created, err := handler.Create(platformtoken.PlatformTokenCreate{
		Name:           fmt.Sprintf("dtctl-e2e-test-token-%d", time.Now().UnixMilli()),
		UserUUID:       userUUID,
		Scope:          []string{"account-idm-read"},
		Resource:       []string{},
		Tags:           []string{},
		ExpirationDate: "2026-12-31T00:00:00.000Z",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if created.TokenID == "" {
		t.Fatal("created token has no TokenID")
	}
	if created.Token == "" {
		t.Fatal("created token secret missing from response")
	}
	t.Logf("Created token: %s (ID: %s)", created.Name, created.TokenID)

	// Step 3: Revoke the token
	t.Log("Step 3: Revoking platform token...")
	if err := handler.Revoke(created.TokenID); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}
	t.Logf("Revoked token: %s", created.TokenID)
}

func TestAccountTokenList_ReadOnly(t *testing.T) {
	c, accountUUID := setupAccountTokenTest(t)
	handler := platformtoken.NewHandler(c, accountUUID)

	tokens, err := handler.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	t.Logf("Listed %d platform tokens", len(tokens))
}
