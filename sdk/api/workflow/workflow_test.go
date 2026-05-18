package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

func newTestClient(t *testing.T, handler http.Handler) *httpclient.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := httpclient.New(srv.URL, httpclient.WithToken("dt0c01.test"))
	if err != nil {
		t.Fatalf("httpclient.New: %v", err)
	}
	return c
}

func stringPtr(v string) *string { return &v }

func intPtr(v int) *int { return &v }

func TestList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := WorkflowList{
			Count: 2,
			Results: []Workflow{
				{ID: "wf-1", Title: "Deploy", Owner: "user-1"},
				{ID: "wf-2", Title: "Remediation", Owner: "user-2"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.List(context.Background(), WorkflowFilters{})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("Count = %d, want 2", result.Count)
	}
	if len(result.Results) != 2 {
		t.Errorf("got %d workflows, want 2", len(result.Results))
	}
}

func TestList_WithOwnerFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("owner"); got != "user-1" {
			t.Fatalf("owner query = %q, want %q", got, "user-1")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"count":0,"results":[]}`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.List(context.Background(), WorkflowFilters{Owner: "user-1"})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
}

func TestList_ParseError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{invalid-json`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.List(context.Background(), WorkflowFilters{})
	if err == nil {
		t.Fatal("List() expected parse error")
	}
}

func TestGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := Workflow{
			ID:                   "wf-1",
			Title:                "Deploy",
			IsDeployed:           true,
			Description:          "Workflow description",
			Actor:                "user-1",
			Owner:                "user-1",
			OwnerType:            "USER",
			Private:              false,
			SchemaVersion:        4,
			Trigger:              map[string]interface{}{"manual": true},
			Result:               stringPtr("{{ result('deploy') }}"),
			Type:                 "STANDARD",
			Input:                map[string]interface{}{"env": "prod"},
			HourlyExecutionLimit: intPtr(1000),
			Guide:                stringPtr("# Deploy\n\nRun after validation."),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.Get(context.Background(), "wf-1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if result.ID != "wf-1" {
		t.Errorf("ID = %q, want %q", result.ID, "wf-1")
	}
	if result.Title != "Deploy" {
		t.Errorf("Title = %q, want %q", result.Title, "Deploy")
	}
	if !result.IsDeployed {
		t.Errorf("IsDeployed = %v, want true", result.IsDeployed)
	}
	if result.Description != "Workflow description" {
		t.Errorf("Description = %q, want %q", result.Description, "Workflow description")
	}
	if result.Actor != "user-1" {
		t.Errorf("Actor = %q, want %q", result.Actor, "user-1")
	}
	if result.OwnerType != "USER" {
		t.Errorf("OwnerType = %q, want %q", result.OwnerType, "USER")
	}
	if result.Private {
		t.Errorf("Private = %v, want false", result.Private)
	}
	if result.SchemaVersion != 4 {
		t.Errorf("SchemaVersion = %d, want 4", result.SchemaVersion)
	}
	if got := result.Trigger["manual"]; got != true {
		t.Errorf("Trigger[manual] = %#v, want true", got)
	}
	if result.Type != "STANDARD" {
		t.Errorf("Type = %q, want %q", result.Type, "STANDARD")
	}
	if got := result.Input["env"]; got != "prod" {
		t.Errorf("Input[env] = %#v, want %q", got, "prod")
	}
	if result.Result == nil || *result.Result != "{{ result('deploy') }}" {
		t.Fatalf("Result = %#v, want %q", result.Result, "{{ result('deploy') }}")
	}
	if result.HourlyExecutionLimit == nil || *result.HourlyExecutionLimit != 1000 {
		t.Fatalf("HourlyExecutionLimit = %#v, want %d", result.HourlyExecutionLimit, 1000)
	}
	if result.Guide == nil || *result.Guide != "# Deploy\n\nRun after validation." {
		t.Fatalf("Guide = %#v, want %q", result.Guide, "# Deploy\n\nRun after validation.")
	}
}

func TestGet_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":{"message":"not found"}}`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("Get() expected error for 404")
	}
}

func TestCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := Workflow{ID: "wf-new", Title: "New Workflow", Owner: "user-1"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.Create(context.Background(), []byte(`{"title":"New Workflow"}`))
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if result.ID != "wf-new" {
		t.Errorf("ID = %q, want %q", result.ID, "wf-new")
	}
}

func TestCreate_ParseError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{invalid-json`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Create(context.Background(), []byte(`{"title":"New Workflow"}`))
	if err == nil {
		t.Fatal("Create() expected parse error")
	}
}

func TestDelete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	h := NewHandler(newTestClient(t, mux))
	err := h.Delete(context.Background(), "wf-1")
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestDelete_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"message":"internal error"}}`)
	})

	h := NewHandler(newTestClient(t, mux))
	err := h.Delete(context.Background(), "wf-1")
	if err == nil {
		t.Fatal("Delete() expected error for 500")
	}
}

func TestGetRaw(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-raw", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"wf-raw","title":"Raw"}`)
	})

	h := NewHandler(newTestClient(t, mux))
	body, err := h.GetRaw(context.Background(), "wf-raw")
	if err != nil {
		t.Fatalf("GetRaw() error: %v", err)
	}
	if got := string(body); got != `{"id":"wf-raw","title":"Raw"}` {
		t.Fatalf("GetRaw() = %q, want %q", got, `{"id":"wf-raw","title":"Raw"}`)
	}
}

func TestUpdate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", ct)
		}
		resp := Workflow{ID: "wf-1", Title: "Updated Workflow", Owner: "user-1"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.Update(context.Background(), "wf-1", []byte(`{"title":"Updated Workflow"}`))
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if result.Title != "Updated Workflow" {
		t.Fatalf("Title = %q, want %q", result.Title, "Updated Workflow")
	}
}

func TestUpdate_ParseError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{invalid-json`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Update(context.Background(), "wf-1", []byte(`{"title":"Updated Workflow"}`))
	if err == nil {
		t.Fatal("Update() expected parse error")
	}
}

func TestListHistory(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1/history", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := HistoryList{
			Count: 2,
			Results: []HistoryRecord{
				{Version: 1, User: "user-1", DateCreated: "2026-05-19T10:00:00Z"},
				{Version: 2, User: "user-1", DateCreated: "2026-05-19T11:00:00Z"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.ListHistory(context.Background(), "wf-1")
	if err != nil {
		t.Fatalf("ListHistory() error: %v", err)
	}
	if result.Count != 2 || len(result.Results) != 2 {
		t.Fatalf("ListHistory() = %#v, want count=2 and two results", result)
	}
}

func TestListHistory_ParseError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1/history", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{invalid-json`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.ListHistory(context.Background(), "wf-1")
	if err == nil {
		t.Fatal("ListHistory() expected parse error")
	}
}

func TestGetHistoryRecord(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1/history/2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := Workflow{ID: "wf-1", Title: "Version 2", Owner: "user-1"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.GetHistoryRecord(context.Background(), "wf-1", 2)
	if err != nil {
		t.Fatalf("GetHistoryRecord() error: %v", err)
	}
	if result.Title != "Version 2" {
		t.Fatalf("Title = %q, want %q", result.Title, "Version 2")
	}
}

func TestGetHistoryRecord_ParseError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1/history/2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{invalid-json`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.GetHistoryRecord(context.Background(), "wf-1", 2)
	if err == nil {
		t.Fatal("GetHistoryRecord() expected parse error")
	}
}

func TestRestoreHistory(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1/history/2/restore", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := Workflow{ID: "wf-1", Title: "Restored", Owner: "user-1"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.RestoreHistory(context.Background(), "wf-1", 2)
	if err != nil {
		t.Fatalf("RestoreHistory() error: %v", err)
	}
	if result.Title != "Restored" {
		t.Fatalf("Title = %q, want %q", result.Title, "Restored")
	}
}

func TestRestoreHistory_ParseError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1/history/2/restore", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{invalid-json`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.RestoreHistory(context.Background(), "wf-1", 2)
	if err == nil {
		t.Fatal("RestoreHistory() expected parse error")
	}
}

func TestGet_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/workflows/wf-1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":{"message":"internal error"}}`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Get(context.Background(), "wf-1")
	if err == nil {
		t.Fatal("Get() expected error for 500")
	}
}
