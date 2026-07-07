package client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

func TestDiscoverAccountUUID(t *testing.T) {
	t.Parallel()

	canned := client.AccessInfoResponse{
		Accounts: []client.AccessInfoAccount{
			{
				UUID: "11111111-2222-3333-4444-555555555555",
				Name: "Acme Corp",
				Environments: []client.AccessInfoEnvironment{
					{ID: "abc12345", Name: "Production"},
					{ID: "abc99999", Name: "Staging"},
				},
			},
			{
				UUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				Name: "Other Corp",
				Environments: []client.AccessInfoEnvironment{
					{ID: "xyz11111", Name: "Dev"},
				},
			},
		},
	}

	tests := []struct {
		name        string
		envID       string
		wantUUID    string
		wantName    string
		wantErrFrag string
	}{
		{
			name:     "found in first account",
			envID:    "abc12345",
			wantUUID: "11111111-2222-3333-4444-555555555555",
			wantName: "Acme Corp",
		},
		{
			name:     "found in second account",
			envID:    "xyz11111",
			wantUUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			wantName: "Other Corp",
		},
		{
			name:        "not found",
			envID:       "unknown000",
			wantErrFrag: `no account found for environment "unknown000"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/iam/v1/access-info" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(canned)
			}))
			defer srv.Close()

			uuid, name, err := client.DiscoverAccountUUID(srv.URL, "test-token", tt.envID)
			if tt.wantErrFrag != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrFrag)
				}
				if err.Error() != tt.wantErrFrag {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErrFrag)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if uuid != tt.wantUUID {
				t.Errorf("uuid = %q, want %q", uuid, tt.wantUUID)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestDiscoverAccountUUID_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, _, err := client.DiscoverAccountUUID(srv.URL, "bad-token", "abc12345")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}
