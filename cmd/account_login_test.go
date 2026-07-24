package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// TestResolveLoginAccountUUID exercises the resolution order used by
// `dtctl account login`: flag > DTCTL_ACCOUNT_UUID > context config >
// auto-discovery via the IAM access-info endpoint.
func TestResolveLoginAccountUUID(t *testing.T) {
	canned := client.AccessInfoResponse{
		Accounts: []client.AccessInfoAccount{
			{
				UUID: "11111111-2222-3333-4444-555555555555",
				Name: "Acme Corp",
				Environments: []client.AccessInfoEnvironment{
					{ID: "abc12345", Name: "Production"},
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/iam/v1/access-info" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(canned)
	}))
	defer srv.Close()

	prodCtx := func() *config.Context {
		return &config.Context{Environment: "https://abc12345.apps.dynatrace.com"}
	}

	t.Run("flag wins over env and config", func(t *testing.T) {
		t.Setenv("DTCTL_ACCOUNT_UUID", "env-uuid")
		ctx := &config.Context{Environment: "https://abc12345.apps.dynatrace.com", AccountUUID: "cfg-uuid"}
		uuid, name, err := resolveLoginAccountUUID(ctx, "flag-uuid", "envtok", srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if uuid != "flag-uuid" {
			t.Errorf("uuid = %q, want flag-uuid", uuid)
		}
		if name != "" {
			t.Errorf("discoveredName should be empty for a non-discovered UUID, got %q", name)
		}
	})

	t.Run("env var when no flag", func(t *testing.T) {
		t.Setenv("DTCTL_ACCOUNT_UUID", "env-uuid")
		ctx := &config.Context{Environment: "https://abc12345.apps.dynatrace.com", AccountUUID: "cfg-uuid"}
		uuid, name, err := resolveLoginAccountUUID(ctx, "", "envtok", srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if uuid != "env-uuid" {
			t.Errorf("uuid = %q, want env-uuid", uuid)
		}
		if name != "" {
			t.Errorf("discoveredName should be empty, got %q", name)
		}
	})

	t.Run("context config when no flag or env", func(t *testing.T) {
		t.Setenv("DTCTL_ACCOUNT_UUID", "")
		ctx := &config.Context{Environment: "https://abc12345.apps.dynatrace.com", AccountUUID: "cfg-uuid"}
		uuid, name, err := resolveLoginAccountUUID(ctx, "", "envtok", srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if uuid != "cfg-uuid" {
			t.Errorf("uuid = %q, want cfg-uuid", uuid)
		}
		if name != "" {
			t.Errorf("discoveredName should be empty, got %q", name)
		}
	})

	t.Run("auto-discovery when nothing else set", func(t *testing.T) {
		t.Setenv("DTCTL_ACCOUNT_UUID", "")
		uuid, name, err := resolveLoginAccountUUID(prodCtx(), "", "envtok", srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if uuid != "11111111-2222-3333-4444-555555555555" {
			t.Errorf("uuid = %q, want discovered UUID", uuid)
		}
		if name != "Acme Corp" {
			t.Errorf("discoveredName = %q, want Acme Corp", name)
		}
	})

	t.Run("error when no environment token for discovery", func(t *testing.T) {
		t.Setenv("DTCTL_ACCOUNT_UUID", "")
		_, _, err := resolveLoginAccountUUID(prodCtx(), "", "", srv.URL)
		if err == nil {
			t.Fatal("expected error when no environment token is available")
		}
	})

	t.Run("discovery error propagates", func(t *testing.T) {
		t.Setenv("DTCTL_ACCOUNT_UUID", "")
		ctx := &config.Context{Environment: "https://unknown99.apps.dynatrace.com"}
		_, _, err := resolveLoginAccountUUID(ctx, "", "envtok", srv.URL)
		if err == nil {
			t.Fatal("expected error when environment is not found in any account")
		}
	})
}
