package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

func TestAnalyzerTypes(t *testing.T) {
	// Verify SDK types have no display-only fields
	a := Analyzer{
		Name:        "test",
		DisplayName: "Test",
		Type:        "builtin",
	}
	if a.Name != "test" {
		t.Errorf("Name = %q, want %q", a.Name, "test")
	}

	r := ExecuteResult{
		RequestToken: "token",
		Result: &AnalyzerResult{
			ResultID:        "result-123",
			ResultStatus:    "SUCCESS",
			ExecutionStatus: "COMPLETED",
		},
	}
	if r.Result.ResultID != "result-123" {
		t.Errorf("ResultID = %q, want %q", r.Result.ResultID, "result-123")
	}
}

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
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := AnalyzerList{
			Analyzers: []Analyzer{
				{Name: "dt.statistics.GenericForecastAnalyzer", DisplayName: "Forecast"},
				{Name: "dt.statistics.GenericOutlierAnalyzer", DisplayName: "Outlier"},
			},
			TotalCount: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.List("")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(result.Analyzers) != 2 {
		t.Errorf("got %d analyzers, want 2", len(result.Analyzers))
	}
}

func TestGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test", func(w http.ResponseWriter, r *http.Request) {
		resp := AnalyzerDefinition{Name: "dt.test", DisplayName: "Test"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.Get("dt.test")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if result.Name != "dt.test" {
		t.Errorf("Name = %q, want %q", result.Name, "dt.test")
	}
}

func TestGet_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":{"message":"not found"}}`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Get("dt.missing")
	if err == nil {
		t.Fatal("Get() expected error for 404")
	}
}

func TestExecute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := ExecuteResult{
			Result: &AnalyzerResult{
				ResultID:        "r-1",
				ResultStatus:    "SUCCESS",
				ExecutionStatus: "COMPLETED",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.Execute("dt.test", map[string]interface{}{"key": "val"}, 30)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.Result.ExecutionStatus != "COMPLETED" {
		t.Errorf("ExecutionStatus = %q, want COMPLETED", result.Result.ExecutionStatus)
	}
}

func TestExecuteAndWait_ImmediateCompletion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := ExecuteResult{
			Result: &AnalyzerResult{
				ResultID:        "r-1",
				ResultStatus:    "SUCCESS",
				ExecutionStatus: "COMPLETED",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.ExecuteAndWait(context.Background(), "dt.test", map[string]interface{}{}, 60)
	if err != nil {
		t.Fatalf("ExecuteAndWait() error: %v", err)
	}
	if result.Result.ExecutionStatus != "COMPLETED" {
		t.Errorf("ExecutionStatus = %q, want COMPLETED", result.Result.ExecutionStatus)
	}
}

func TestExecuteAndWait_PollsUntilComplete(t *testing.T) {
	var pollCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := ExecuteResult{
			RequestToken: "tok-123",
			Result: &AnalyzerResult{
				ResultID:        "r-1",
				ExecutionStatus: "RUNNING",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:poll", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("request-token") != "tok-123" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		n := pollCount.Add(1)
		status := "RUNNING"
		if n >= 2 {
			status = "COMPLETED"
		}
		resp := ExecuteResult{
			RequestToken: "tok-123",
			Result: &AnalyzerResult{
				ResultID:        "r-1",
				ResultStatus:    "SUCCESS",
				ExecutionStatus: status,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.ExecuteAndWait(context.Background(), "dt.test", map[string]interface{}{}, 60)
	if err != nil {
		t.Fatalf("ExecuteAndWait() error: %v", err)
	}
	if result.Result.ExecutionStatus != "COMPLETED" {
		t.Errorf("ExecutionStatus = %q, want COMPLETED", result.Result.ExecutionStatus)
	}
	if c := pollCount.Load(); c < 2 {
		t.Errorf("poll count = %d, want >= 2", c)
	}
}

func TestExecuteAndWait_Aborted(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := ExecuteResult{
			RequestToken: "tok-abort",
			Result: &AnalyzerResult{
				ResultID:        "r-1",
				ExecutionStatus: "RUNNING",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:poll", func(w http.ResponseWriter, r *http.Request) {
		resp := ExecuteResult{
			Result: &AnalyzerResult{
				ResultID:        "r-1",
				ExecutionStatus: "ABORTED",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.ExecuteAndWait(context.Background(), "dt.test", map[string]interface{}{}, 60)
	if err == nil {
		t.Fatal("ExecuteAndWait() expected error for ABORTED execution")
	}
}

func TestExecuteAndWait_Timeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := ExecuteResult{
			RequestToken: "tok-slow",
			Result: &AnalyzerResult{
				ResultID:        "r-1",
				ExecutionStatus: "RUNNING",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:poll", func(w http.ResponseWriter, r *http.Request) {
		// Always return RUNNING — never completes
		resp := ExecuteResult{
			RequestToken: "tok-slow",
			Result: &AnalyzerResult{
				ResultID:        "r-1",
				ExecutionStatus: "RUNNING",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	// maxWaitSeconds=0 should timeout immediately
	_, err := h.ExecuteAndWait(context.Background(), "dt.test", map[string]interface{}{}, 0)
	if err == nil {
		t.Fatal("ExecuteAndWait() expected timeout error")
	}
}

func TestPoll_Expired(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:poll", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		fmt.Fprintf(w, `{"error":{"message":"result expired"}}`)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Poll("dt.test", "expired-token", 10)
	if err == nil {
		t.Fatal("Poll() expected error for 410 Gone")
	}
}
