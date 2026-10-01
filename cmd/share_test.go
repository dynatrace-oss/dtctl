package cmd

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/config"
)

// setupShareCmdTest wires a fresh file-backed config pointed at srv and
// resets the command tree/dry-run flag afterward, the same shape
// TestShareDocument_NoNotify uses.
func setupShareCmdTest(t *testing.T, srv *httptest.Server) {
	t.Helper()

	t.Setenv("DTCTL_DISABLE_KEYRING", "1")
	t.Setenv(config.EnvTokenStorage, "file")
	configPath := filepath.Join(t.TempDir(), "config")
	originalCfgFile, originalDryRun := cfgFile, dryRun
	restorePristineTree()
	t.Cleanup(func() {
		cfgFile, dryRun = originalCfgFile, originalDryRun
		restorePristineTree()
	})
	cfgFile = configPath

	cfg := config.NewConfig()
	cfg.SetContext("test", srv.URL, "test-token")
	if err := cfg.SetToken("test-token", "dt0c01.ST.test-token-value.test-secret"); err != nil {
		t.Fatalf("failed to set token: %v", err)
	}
	cfg.CurrentContext = "test"
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
}

// TestShareGet_ResolvesEnvironmentShare checks the happy path: a share ID that
// resolves as an environment share never reaches the direct-share endpoint.
func TestShareGet_ResolvesEnvironmentShare(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares/share-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"share-1","documentId":"doc-123","access":["read"],"claimCount":3}`))
	})
	mux.HandleFunc("/platform/document/v1/direct-shares/share-1", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("share get should not fall back to direct-share once the environment-share resolves")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	setupShareCmdTest(t, srv)

	out := captureExtStdout(t, func() {
		if err := shareGetCmd.RunE(shareGetCmd, []string{"share-1"}); err != nil {
			t.Fatalf("share get failed: %v", err)
		}
	})
	if !strings.Contains(out, "doc-123") {
		t.Errorf("output %q does not mention the resolved document ID", out)
	}
}

// TestShareGet_FallsBackToDirectShare checks that a 404 on the environment-share
// lookup falls back to the direct-share endpoint rather than failing outright.
func TestShareGet_FallsBackToDirectShare(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares/share-2", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not found"}}`))
	})
	mux.HandleFunc("/platform/document/v1/direct-shares/share-2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"share-2","documentId":"doc-456","access":["read"]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	setupShareCmdTest(t, srv)

	out := captureExtStdout(t, func() {
		if err := shareGetCmd.RunE(shareGetCmd, []string{"share-2"}); err != nil {
			t.Fatalf("share get failed: %v", err)
		}
	})
	if !strings.Contains(out, "doc-456") {
		t.Errorf("output %q does not mention the resolved document ID", out)
	}
}

// TestShareGet_StripsPastedShareURL checks that a full share URL copied from
// the browser resolves the same as the bare share ID.
func TestShareGet_StripsPastedShareURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares/018f1234-abcd-7000-8000-000000000000", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"018f1234-abcd-7000-8000-000000000000","documentId":"doc-789","access":["read"]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	setupShareCmdTest(t, srv)

	url := "https://xyz.apps.dynatrace.com/ui/document/v0/#share=018f1234-abcd-7000-8000-000000000000"
	out := captureExtStdout(t, func() {
		if err := shareGetCmd.RunE(shareGetCmd, []string{url}); err != nil {
			t.Fatalf("share get failed: %v", err)
		}
	})
	if !strings.Contains(out, "doc-789") {
		t.Errorf("output %q does not mention the resolved document ID", out)
	}
}

// TestShareClaim_DryRun checks that --dry-run sends no PUT.
func TestShareClaim_DryRun(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares/share-1/claim", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("dry run must not send the claim request")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	setupShareCmdTest(t, srv)
	dryRun = true

	out := captureExtStdout(t, func() {
		if err := shareClaimCmd.RunE(shareClaimCmd, []string{"share-1"}); err != nil {
			t.Fatalf("share claim dry run failed: %v", err)
		}
	})
	if !strings.Contains(out, "Dry run") {
		t.Errorf("output %q does not read as a dry run", out)
	}
}

// TestShareClaim_Success checks the real claim path, including that a pasted
// share URL is accepted and resolved to its document ID.
func TestShareClaim_Success(t *testing.T) {
	var claimed bool
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares/share-1/claim", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		claimed = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"share-1","documentId":"doc-123","access":["read"],"claimCount":1}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	setupShareCmdTest(t, srv)

	url := "https://xyz.apps.dynatrace.com/ui/document/v0/#share=share-1"
	err := shareClaimCmd.RunE(shareClaimCmd, []string{url})
	if err != nil {
		t.Fatalf("share claim failed: %v", err)
	}
	if !claimed {
		t.Error("expected the claim endpoint to be called")
	}
}

// TestShareDocument_NoNotify drives the real share command against a mock
// Document API and checks that --no-notify reaches the API on both paths the
// command can take: creating a new share, and adding to an existing one.
// Without it the API notifies every recipient, and a group recipient is every
// member of the group.
func TestShareDocument_NoNotify(t *testing.T) {
	tests := []struct {
		name          string
		existingShare bool
		noNotify      bool
		wantPath      string
		wantParam     string
	}{
		{"create notifies by default", false, false, "/platform/document/v1/direct-shares", ""},
		{"create with --no-notify", false, true, "/platform/document/v1/direct-shares", "false"},
		{"add notifies by default", true, false, "/platform/document/v1/direct-shares/share-1/recipients/add", ""},
		{"add with --no-notify", true, true, "/platform/document/v1/direct-shares/share-1/recipients/add", "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			mutations := map[string]string{}
			record := func(r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				mutations[r.URL.Path] = r.URL.Query().Get("send-notification")
			}

			mux := http.NewServeMux()
			mux.HandleFunc("/platform/document/v1/documents/doc-123/metadata", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"doc-123","name":"Dash","type":"dashboard","owner":"owner-1","version":1}`))
			})
			mux.HandleFunc("/platform/document/v1/direct-shares", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					if tt.existingShare {
						_, _ = w.Write([]byte(`{"direct-shares":[{"id":"share-1","documentId":"doc-123","access":["read"]}],"totalCount":1}`))
					} else {
						_, _ = w.Write([]byte(`{"direct-shares":[],"totalCount":0}`))
					}
					return
				}
				record(r)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":"share-1","documentId":"doc-123","access":["read"]}`))
			})
			mux.HandleFunc("/platform/document/v1/direct-shares/share-1/recipients/add", func(w http.ResponseWriter, r *http.Request) {
				record(r)
				w.WriteHeader(http.StatusNoContent)
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			t.Setenv("DTCTL_DISABLE_KEYRING", "1")
			t.Setenv(config.EnvTokenStorage, "file")
			configPath := filepath.Join(t.TempDir(), "config")
			originalCfgFile, originalDryRun := cfgFile, dryRun
			// Another test may have left a stability floor wrapped around this
			// command; start from, and leave behind, the as-registered tree.
			restorePristineTree()
			t.Cleanup(func() {
				cfgFile, dryRun = originalCfgFile, originalDryRun
				restorePristineTree()
			})
			cfgFile, dryRun = configPath, false

			cfg := config.NewConfig()
			cfg.SetContext("test", srv.URL, "test-token")
			if err := cfg.SetToken("test-token", "dt0c01.ST.test-token-value.test-secret"); err != nil {
				t.Fatalf("failed to set token: %v", err)
			}
			cfg.CurrentContext = "test"
			if err := cfg.SaveTo(configPath); err != nil {
				t.Fatalf("failed to save config: %v", err)
			}

			_ = shareDocumentCmd.Flags().Set("group", "group-1")
			if tt.noNotify {
				_ = shareDocumentCmd.Flags().Set("no-notify", "true")
			}

			captureExtStdout(t, func() {
				if err := shareDocumentCmd.RunE(shareDocumentCmd, []string{"doc-123"}); err != nil {
					t.Fatalf("share document failed: %v", err)
				}
			})

			if len(mutations) != 1 {
				t.Fatalf("got %d mutating calls %v, want exactly one to %s", len(mutations), mutations, tt.wantPath)
			}
			got, ok := mutations[tt.wantPath]
			if !ok {
				t.Fatalf("mutating calls %v, want one to %s", mutations, tt.wantPath)
			}
			if got != tt.wantParam {
				t.Errorf("send-notification = %q, want %q", got, tt.wantParam)
			}
		})
	}
}
