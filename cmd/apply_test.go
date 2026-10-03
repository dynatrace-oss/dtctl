package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/apply"
	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
)

// newEnvShareMock builds an httptest server that mocks the minimal Document
// Service endpoints `apply --share-environment` touches. It returns deterministic
// behaviour per documentID so tests can drive success/failure paths without a
// real API. `fail` is the set of document IDs whose POST should return a 500.
func newEnvShareMock(t *testing.T, fail map[string]bool) (*httptest.Server, map[string]*int64) {
	t.Helper()
	counts := map[string]*int64{}
	mux := http.NewServeMux()

	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			// No existing shares — forces create path.
			_ = json.NewEncoder(w).Encode(document.EnvironmentShareList{Shares: nil, TotalCount: 0})
			return
		}
		if r.Method == http.MethodPost {
			var body document.CreateEnvironmentShareRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			if fail[body.DocumentID] {
				http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
				return
			}
			if _, ok := counts[body.DocumentID]; !ok {
				var n int64
				counts[body.DocumentID] = &n
			}
			atomic.AddInt64(counts[body.DocumentID], 1)
			_ = json.NewEncoder(w).Encode(document.EnvironmentShare{
				ID: "share-" + body.DocumentID, DocumentID: body.DocumentID, Access: []string{"read"},
			})
		}
	})

	// Metadata + isPrivate PATCH (needed by --share-environment public). A
	// PATCH is counted under "patch:<id>".
	mux.HandleFunc("/platform/document/v1/documents/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/metadata") {
			w.Header().Set("Content-Type", "application/json")
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/platform/document/v1/documents/"), "/metadata")
			_ = json.NewEncoder(w).Encode(document.DocumentMetadata{ID: id, Name: "doc", Type: "notebook", Version: 1, IsPrivate: true})
			return
		}
		if r.Method == http.MethodPatch {
			key := "patch:" + strings.TrimPrefix(r.URL.Path, "/platform/document/v1/documents/")
			if _, ok := counts[key]; !ok {
				var n int64
				counts[key] = &n
			}
			atomic.AddInt64(counts[key], 1)
			w.WriteHeader(http.StatusOK)
			return
		}
	})

	return httptest.NewServer(mux), counts
}

func newEnvShareClient(t *testing.T, srv *httptest.Server) *client.Client {
	t.Helper()
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("client.NewForTesting: %v", err)
	}
	return c
}

func TestEnsureEnvironmentShareForResults_SkipsNonDocuments(t *testing.T) {
	srv, counts := newEnvShareMock(t, nil)
	defer srv.Close()
	c := newEnvShareClient(t, srv)

	results := []apply.ApplyResult{
		&apply.NotebookApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "nb-1", ResourceType: "notebook"}},
		&apply.WorkflowApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "wf-1", ResourceType: "workflow"}},
		&apply.SLOApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "slo-1", ResourceType: "slo"}},
		&apply.DashboardApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "db-1", ResourceType: "dashboard"}},
		&apply.DocumentApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "lp-1", ResourceType: "launchpad"}},
	}
	if err := ensureEnvironmentShareForResults(c, results, linkAndPublic, "read", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := counts["lp-1"]; got == nil || atomic.LoadInt64(got) != 1 {
		t.Errorf("expected launchpad lp-1 to be shared once, counts=%v", counts)
	}
	if got := counts["nb-1"]; got == nil || atomic.LoadInt64(got) != 1 {
		t.Errorf("expected notebook nb-1 to be shared once, counts=%v", counts)
	}
	if got := counts["db-1"]; got == nil || atomic.LoadInt64(got) != 1 {
		t.Errorf("expected dashboard db-1 to be shared once, counts=%v", counts)
	}
	if _, ok := counts["wf-1"]; ok {
		t.Errorf("workflow wf-1 must not be shared")
	}
	if _, ok := counts["slo-1"]; ok {
		t.Errorf("slo slo-1 must not be shared")
	}
}

func TestEnsureEnvironmentShareForResults_SkipsEmptyID(t *testing.T) {
	srv, counts := newEnvShareMock(t, nil)
	defer srv.Close()
	c := newEnvShareClient(t, srv)

	results := []apply.ApplyResult{
		&apply.NotebookApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "", ResourceType: "notebook"}},
	}
	if err := ensureEnvironmentShareForResults(c, results, linkAndPublic, "read", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("expected no share calls for empty ID, got %v", counts)
	}
}

func TestEnsureEnvironmentShareForResults_ContinuesAfterFailure(t *testing.T) {
	// First doc fails; remaining documents must still be attempted and the
	// combined error must reference the failing ID.
	srv, counts := newEnvShareMock(t, map[string]bool{"nb-bad": true})
	defer srv.Close()
	c := newEnvShareClient(t, srv)

	results := []apply.ApplyResult{
		&apply.NotebookApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "nb-bad", ResourceType: "notebook"}},
		&apply.DashboardApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "db-ok", ResourceType: "dashboard"}},
	}
	err := ensureEnvironmentShareForResults(c, results, linkAndPublic, "read", false)
	if err == nil {
		t.Fatal("expected error from failing share")
	}
	if !strings.Contains(err.Error(), "nb-bad") {
		t.Errorf("error should reference failing document, got %q", err.Error())
	}
	if got := counts["db-ok"]; got == nil || atomic.LoadInt64(got) != 1 {
		t.Errorf("second document should still be shared after first failure, counts=%v", counts)
	}
}

func TestEnsureEnvironmentShareForResults_MultipleFailuresCombined(t *testing.T) {
	srv, _ := newEnvShareMock(t, map[string]bool{"nb-1": true, "db-1": true})
	defer srv.Close()
	c := newEnvShareClient(t, srv)

	results := []apply.ApplyResult{
		&apply.NotebookApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "nb-1", ResourceType: "notebook"}},
		&apply.DashboardApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "db-1", ResourceType: "dashboard"}},
	}
	err := ensureEnvironmentShareForResults(c, results, linkAndPublic, "read", false)
	if err == nil {
		t.Fatal("expected combined error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "2 documents failed") {
		t.Errorf("combined error should count failures, got %q", msg)
	}
	if !strings.Contains(msg, "nb-1") || !strings.Contains(msg, "db-1") {
		t.Errorf("combined error should reference both failing IDs, got %q", msg)
	}
}

// linkAndPublic is what `--share-environment link,public` asks for.
var linkAndPublic = environmentModes{link: true, public: true}

// TestEnsureEnvironmentShareForResults_Modes: each mode does only its own
// half, as on `share document --environment`.
func TestEnsureEnvironmentShareForResults_Modes(t *testing.T) {
	tests := []struct {
		name                string
		modes               environmentModes
		wantPost, wantPatch bool
	}{
		{"link", environmentModes{link: true}, true, false},
		{"public", environmentModes{public: true}, false, true},
		{"link,public", linkAndPublic, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, counts := newEnvShareMock(t, nil)
			defer srv.Close()
			c := newEnvShareClient(t, srv)

			results := []apply.ApplyResult{
				&apply.DocumentApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "lp-1", ResourceType: "launchpad"}},
			}
			if err := ensureEnvironmentShareForResults(c, results, tt.modes, "read", false); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, posted := counts["lp-1"]; posted != tt.wantPost {
				t.Errorf("environment share created = %v, want %v", posted, tt.wantPost)
			}
			if _, patched := counts["patch:lp-1"]; patched != tt.wantPatch {
				t.Errorf("isPrivate patched = %v, want %v", patched, tt.wantPatch)
			}
		})
	}
}

// TestEnsureEnvironmentShareForResults_ExistingLinkAtAnotherLevel: without
// --share-access, a link at another level is neither reused nor replaced,
// and the error names both ways out.
func TestEnsureEnvironmentShareForResults_ExistingLinkAtAnotherLevel(t *testing.T) {
	s, srv := newEnvShareServer(t) // lp-1 has a read share
	c := newEnvShareClient(t, srv)

	results := []apply.ApplyResult{
		&apply.DocumentApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "lp-1", ResourceType: "launchpad"}},
	}
	err := ensureEnvironmentShareForResults(c, results, environmentModes{link: true}, "read-write", false)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"already has a read environment share", "--share-access read ", "--share-access read-write"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	assertCalls(t, s, "")
}

func TestApplyCmd_ShareEnvironmentFlagValidation(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
		want  string // "" = valid
	}{
		{"link", map[string]string{"share-environment": "link"}, ""},
		{"public", map[string]string{"share-environment": "public"}, ""},
		{"both", map[string]string{"share-environment": "link,public"}, ""},
		{"link with --share-access", map[string]string{"share-environment": "link", "share-access": "read-write"}, ""},
		{"old value read", map[string]string{"share-environment": "read"}, "use --share-environment link,public for"},
		{"old value read-write", map[string]string{"share-environment": "read-write"},
			"--share-environment link,public --share-access read-write"},
		{"unknown mode", map[string]string{"share-environment": "bogus"}, "must be 'link', 'public', or both"},
		{"empty", map[string]string{"share-environment": ""}, "needs a value"},
		{"--share-access without link", map[string]string{"share-environment": "public", "share-access": "read"},
			"needs --share-environment link"},
		{"unknown --share-access", map[string]string{"share-environment": "link", "share-access": "rw"},
			"invalid --share-access"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restorePristineTree()
			t.Cleanup(restorePristineTree)
			for k, v := range tt.flags {
				if err := applyCmd.Flags().Set(k, v); err != nil {
					t.Fatalf("set --%s: %v", k, err)
				}
			}

			modes, err := applyShareEnvironmentModes(applyCmd)
			if err == nil {
				access, _ := applyCmd.Flags().GetString("share-access")
				err = validateShareAccess(applyCmd, modes, access)
			}
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("error = %v, want one mentioning %q", err, tt.want)
			}
		})
	}
}
