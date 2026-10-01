package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func claimMux(t *testing.T, called *bool) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares/share-1/claim", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		*called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"documentId":"doc-123","documentType":"dashboard","access":["read"]}`))
	})
	return mux
}

func TestClaimEnvironmentShare_Success(t *testing.T) {
	var called bool
	srv := httptest.NewServer(claimMux(t, &called))
	t.Cleanup(srv.Close)
	setupShareCmdTest(t, srv)

	out := captureExtStdout(t, func() {
		if err := claimEnvironmentShareCmd.RunE(claimEnvironmentShareCmd, []string{"share-1"}); err != nil {
			t.Fatalf("claim failed: %v", err)
		}
	})
	if !called {
		t.Error("expected the claim endpoint to be called")
	}
	if !strings.Contains(out, "doc-123") || !strings.Contains(out, "dashboard") {
		t.Errorf("output %q lacks the document ID/type", out)
	}
}

func TestClaimEnvironmentShare_PastedURL(t *testing.T) {
	var called bool
	srv := httptest.NewServer(claimMux(t, &called))
	t.Cleanup(srv.Close)
	setupShareCmdTest(t, srv)

	link := srv.URL + "/ui/document/v0/#share=share-1"
	if err := claimEnvironmentShareCmd.RunE(claimEnvironmentShareCmd, []string{link}); err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if !called {
		t.Error("expected the claim endpoint to be called")
	}
}

func TestClaimEnvironmentShare_HostMismatch(t *testing.T) {
	var called bool
	srv := httptest.NewServer(claimMux(t, &called))
	t.Cleanup(srv.Close)
	setupShareCmdTest(t, srv)

	link := "https://other.apps.example.invalid/ui/document/v0/#share=share-1"
	err := claimEnvironmentShareCmd.RunE(claimEnvironmentShareCmd, []string{link})
	if err == nil || !strings.Contains(err.Error(), "current context") {
		t.Fatalf("expected a host mismatch error, got %v", err)
	}
	if called {
		t.Error("a mismatched link must not reach the claim endpoint")
	}
}

func TestClaimEnvironmentShare_DryRun(t *testing.T) {
	var called bool
	srv := httptest.NewServer(claimMux(t, &called))
	t.Cleanup(srv.Close)
	setupShareCmdTest(t, srv)
	dryRun = true

	out := captureExtStdout(t, func() {
		if err := claimEnvironmentShareCmd.RunE(claimEnvironmentShareCmd, []string{"share-1"}); err != nil {
			t.Fatalf("dry run failed: %v", err)
		}
	})
	if called {
		t.Error("dry run must not send the claim request")
	}
	if !strings.Contains(out, "Dry run") {
		t.Errorf("output %q does not read as a dry run", out)
	}
}

func TestSameEnvironment(t *testing.T) {
	tests := []struct {
		link, env string
		want      bool
	}{
		{"abc.apps.dynatrace.com", "https://abc.apps.dynatrace.com", true},
		{"abc.apps.dynatrace.com", "https://abc.live.dynatrace.com", true},
		{"abc.apps.dynatrace.com", "https://xyz.apps.dynatrace.com", false},
		{"abc.attacker.example", "https://abc.apps.dynatrace.com", false},
		{"abc.apps.dynatrace.com.attacker.example", "https://abc.apps.dynatrace.com", false},
		{"127.0.0.1:8080", "http://127.0.0.1:8080", true},
	}
	for _, tt := range tests {
		if got := sameEnvironment(tt.link, tt.env); got != tt.want {
			t.Errorf("sameEnvironment(%q, %q) = %v, want %v", tt.link, tt.env, got, tt.want)
		}
	}
}
