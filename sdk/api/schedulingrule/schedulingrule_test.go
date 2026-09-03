package schedulingrule

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

func TestList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/scheduling-rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := SchedulingRuleList{
			Count: 2,
			Results: []SchedulingRule{
				{ID: "sr-1", Title: "Business Hours", Rule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", Timezone: "UTC"},
				{ID: "sr-2", Title: "Maintenance Window", Rule: "FREQ=MONTHLY;BYMONTHDAY=1", Timezone: "Europe/Vienna"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.List(context.Background())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("Count = %d, want 2", result.Count)
	}
	if len(result.Results) != 2 {
		t.Errorf("got %d rules, want 2", len(result.Results))
	}
}

func TestList_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/scheduling-rules", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"message":"internal error"}}`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.List(context.Background())
	if err == nil {
		t.Fatal("List() expected error for 500")
	}
}

func TestGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/scheduling-rules/sr-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rule := SchedulingRule{ID: "sr-1", Title: "Business Hours", Rule: "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", Timezone: "UTC"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rule)
	})

	h := NewHandler(newTestClient(t, mux))
	rule, err := h.Get(context.Background(), "sr-1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if rule.ID != "sr-1" {
		t.Errorf("ID = %q, want %q", rule.ID, "sr-1")
	}
}

func TestGet_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/scheduling-rules/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":404,"message":"not found"}}`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("Get() expected error for 404")
	}
}

func TestCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/scheduling-rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rule := SchedulingRule{ID: "sr-new", Title: "New Rule", Rule: "FREQ=DAILY", Timezone: "UTC"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(rule)
	})

	h := NewHandler(newTestClient(t, mux))
	data := []byte(`{"title":"New Rule","rule":"FREQ=DAILY","timezone":"UTC"}`)
	result, err := h.Create(context.Background(), data)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if result.ID != "sr-new" {
		t.Errorf("ID = %q, want %q", result.ID, "sr-new")
	}
}

func TestUpdate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/scheduling-rules/sr-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rule := SchedulingRule{ID: "sr-1", Title: "Updated Rule", Rule: "FREQ=WEEKLY;BYDAY=MO", Timezone: "UTC"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rule)
	})

	h := NewHandler(newTestClient(t, mux))
	data := []byte(`{"title":"Updated Rule","rule":"FREQ=WEEKLY;BYDAY=MO","timezone":"UTC"}`)
	result, err := h.Update(context.Background(), "sr-1", data)
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if result.Title != "Updated Rule" {
		t.Errorf("Title = %q, want %q", result.Title, "Updated Rule")
	}
}

func TestDelete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/scheduling-rules/sr-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	h := NewHandler(newTestClient(t, mux))
	if err := h.Delete(context.Background(), "sr-1"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestGetRaw(t *testing.T) {
	body := `{"id":"sr-1","title":"Business Hours","rule":"FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR","timezone":"UTC"}`
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/automation/v1/scheduling-rules/sr-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	})

	h := NewHandler(newTestClient(t, mux))
	raw, err := h.GetRaw(context.Background(), "sr-1")
	if err != nil {
		t.Fatalf("GetRaw() error: %v", err)
	}
	if string(raw) != body {
		t.Errorf("GetRaw() = %q, want %q", string(raw), body)
	}
}
