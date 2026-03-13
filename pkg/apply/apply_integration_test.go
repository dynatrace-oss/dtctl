package apply

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
)

// newApplyTestServer creates a multiplexed test server that handles multiple resource endpoints.
func newApplyTestServer(t *testing.T, handlers map[string]http.HandlerFunc) (*httptest.Server, *client.Client) {
	t.Helper()
	mux := http.NewServeMux()
	for path, h := range handlers {
		mux.HandleFunc(path, h)
	}
	srv := httptest.NewServer(mux)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return srv, c
}

// --- NewApplier / WithSafetyChecker ---

func TestNewApplier_CreatesApplier(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized) // CurrentUserID will fallback to empty
		},
	})
	defer srv.Close()

	a := NewApplier(c)
	if a == nil {
		t.Fatal("NewApplier returned nil")
	}
	if a.client == nil {
		t.Error("applier.client is nil")
	}
}

func TestWithSafetyChecker_Sets(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()

	a := NewApplier(c)
	// WithSafetyChecker returns self (fluent)
	returned := a.WithSafetyChecker(nil)
	if returned != a {
		t.Error("WithSafetyChecker should return the same applier")
	}
}

// --- Apply: invalid input ---

func TestApply_InvalidJSON(t *testing.T) {
	srv, c := newApplyTestServer(t, nil)
	defer srv.Close()
	a := NewApplier(c)

	_, err := a.Apply([]byte(`not json`), ApplyOptions{})
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestApply_UnknownResourceType(t *testing.T) {
	srv, c := newApplyTestServer(t, nil)
	defer srv.Close()
	a := NewApplier(c)

	// JSON that doesn't match any known resource type
	_, err := a.Apply([]byte(`{"foo":"bar"}`), ApplyOptions{})
	if err == nil {
		t.Fatal("expected error for unknown resource type, got nil")
	}
}

// --- Apply: workflow create (no id) ---

func TestApply_WorkflowCreate_NoID(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/automation/v1/workflows": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    "wf-new-123",
				"title": "My Workflow",
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	wfJSON := `{"title":"My Workflow","tasks":{},"trigger":{}}`
	results, err := a.Apply([]byte(wfJSON), ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	base := results[0].(*WorkflowApplyResult).ApplyResultBase
	if base.Action != ActionCreated {
		t.Errorf("expected action 'created', got %q", base.Action)
	}
}

// --- Apply: workflow update (has id, exists) ---

func TestApply_WorkflowUpdate_Exists(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/automation/v1/workflows/wf-existing": func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":    "wf-existing",
					"title": "Old Title",
					"owner": "user-xyz",
				})
			case http.MethodPut:
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":    "wf-existing",
					"title": "New Title",
				})
			}
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	wfJSON := `{"id":"wf-existing","title":"New Title","tasks":{},"trigger":{}}`
	results, err := a.Apply([]byte(wfJSON), ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	base := results[0].(*WorkflowApplyResult).ApplyResultBase
	if base.Action != ActionUpdated {
		t.Errorf("expected action 'updated', got %q", base.Action)
	}
}

// --- Apply: workflow with id but not found → create ---

func TestApply_WorkflowCreate_IDNotFound(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/automation/v1/workflows/wf-missing": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
		"/platform/automation/v1/workflows": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    "wf-missing",
				"title": "New Workflow",
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	wfJSON := `{"id":"wf-missing","title":"New Workflow","tasks":{},"trigger":{}}`
	results, err := a.Apply([]byte(wfJSON), ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	base := results[0].(*WorkflowApplyResult).ApplyResultBase
	if base.Action != ActionCreated {
		t.Errorf("expected action 'created', got %q", base.Action)
	}
}

// --- Apply: SLO create ---

func TestApply_SLOCreate(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/slo/v1/slos": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   "slo-new-1",
				"name": "My SLO",
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	sloJSON := `{"name":"My SLO","criteria":{"pass":[{"criteria":[{"metric":"<100","steps":600}]}]},"target":99.0,"timeframe":"now-7d","metricExpression":"100*..."}`
	results, err := a.Apply([]byte(sloJSON), ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply() SLO error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	base := results[0].(*SLOApplyResult).ApplyResultBase
	if base.Action != ActionCreated {
		t.Errorf("expected 'created', got %q", base.Action)
	}
}

// --- Apply: bucket create ---

func TestApply_BucketCreate(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/storage/management/v1/bucket-definitions": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"bucketName": "my-logs",
				"table":      "logs",
				"status":     "creating",
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	bucketJSON := `{"bucketName":"my-logs","table":"logs","displayName":"My Logs","retentionDays":35}`
	results, err := a.Apply([]byte(bucketJSON), ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply() bucket error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	base := results[0].(*BucketApplyResult).ApplyResultBase
	if base.Action != ActionCreated {
		t.Errorf("expected 'created', got %q", base.Action)
	}
}

// --- Apply: dryRun workflow ---

func TestApply_DryRun_Workflow(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	wfJSON := `{"title":"My Workflow","id":"wf-dry","tasks":{},"trigger":{}}`
	_, err := a.Apply([]byte(wfJSON), ApplyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Apply() dryRun error = %v", err)
	}
}

// --- Apply: settings create ---

func TestApply_SettingsCreate(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/classic/environment-api/v2/settings/objects": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"objectId": "obj-new-1"},
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	settingsJSON := `{"schemaId":"builtin:alerting.profile","scope":"environment","value":{"name":"Test"}}`
	results, err := a.Apply([]byte(settingsJSON), ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply() settings error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	base := results[0].(*SettingsApplyResult).ApplyResultBase
	if base.Action != ActionCreated {
		t.Errorf("expected 'created', got %q", base.Action)
	}
}

// --- stderrWarn ---

func TestStderrWarn_AppendsToSlice(t *testing.T) {
	var warnings []string
	stderrWarn(&warnings, "test warning %d", 42)
	if len(warnings) != 1 || warnings[0] != "test warning 42" {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestStderrWarn_NilSlice(t *testing.T) {
	// Should not panic with nil slice
	stderrWarn(nil, "no-op warning")
}

// --- Apply: template vars ---

func TestApply_WithTemplateVars(t *testing.T) {
	srv, c := newApplyTestServer(t, map[string]http.HandlerFunc{
		"/platform/automation/v1/workflows": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			title, _ := body["title"].(string)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    "wf-templated",
				"title": title,
			})
		},
		"/platform/metadata/v1/user": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer srv.Close()
	a := NewApplier(c)

	wfTemplate := `{"title":"{{.name}}","tasks":{},"trigger":{}}`
	results, err := a.Apply([]byte(wfTemplate), ApplyOptions{
		TemplateVars: map[string]interface{}{"name": "Rendered Workflow"},
	})
	if err != nil {
		t.Fatalf("Apply() template error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	fmt.Println("template vars test passed:", results[0].(*WorkflowApplyResult).Name)
}
