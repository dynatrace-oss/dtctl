package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/apply"
	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/resources/document"
)

func TestEnsureEnvironmentShareForResults_SharesDocumentsOnly(t *testing.T) {
	shareCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(document.EnvironmentShareList{TotalCount: 0})
			return
		}
		if r.Method == http.MethodPost {
			shareCalls++
			var body document.CreateEnvironmentShareRequest
			json.NewDecoder(r.Body).Decode(&body)
			json.NewEncoder(w).Encode(document.EnvironmentShare{
				ID:         "s-" + body.DocumentID,
				DocumentID: body.DocumentID,
				Access:     []string{"read"},
			})
		}
	})
	mux.HandleFunc("/platform/document/v1/documents/nb-1/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(document.DocumentMetadata{ID: "nb-1", Name: "nb", Type: "notebook", Version: 1, IsPrivate: false})
	})
	mux.HandleFunc("/platform/document/v1/documents/db-1/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(document.DocumentMetadata{ID: "db-1", Name: "db", Type: "dashboard", Version: 1, IsPrivate: false})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	results := []apply.ApplyResult{
		&apply.NotebookApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "nb-1", ResourceType: "notebook", Action: "created"}},
		&apply.DashboardApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "db-1", ResourceType: "dashboard", Action: "created"}},
		&apply.WorkflowApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "wf-1", ResourceType: "workflow", Action: "created"}},
		&apply.SLOApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "slo-1", ResourceType: "slo", Action: "created"}},
	}

	err = ensureEnvironmentShareForResults(c, results, "read")
	if err != nil {
		t.Fatalf("ensureEnvironmentShareForResults: %v", err)
	}
	if shareCalls != 2 {
		t.Errorf("expected 2 share create calls (notebook + dashboard), got %d", shareCalls)
	}
}

func TestEnsureEnvironmentShareForResults_SkipsEmptyID(t *testing.T) {
	shareCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/document/v1/environment-shares", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			shareCalls++
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(document.EnvironmentShareList{TotalCount: 0})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	results := []apply.ApplyResult{
		&apply.NotebookApplyResult{ApplyResultBase: apply.ApplyResultBase{ID: "", ResourceType: "notebook", Action: "created"}},
	}

	err = ensureEnvironmentShareForResults(c, results, "read")
	if err != nil {
		t.Fatalf("ensureEnvironmentShareForResults: %v", err)
	}
	if shareCalls != 0 {
		t.Errorf("expected 0 share calls for empty ID, got %d", shareCalls)
	}
}

func TestEnsureEnvironmentShareForResults_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.NewServeMux())
	defer srv.Close()
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	err = ensureEnvironmentShareForResults(c, nil, "read")
	if err != nil {
		t.Fatalf("expected no error for empty results, got: %v", err)
	}
}
