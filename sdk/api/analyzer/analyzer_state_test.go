package analyzer

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestExecuteAndWait_FailedExecuteStatusIsAnError covers #495: ABORTED or
// FAILED in the execute response, which carries no request token, must be an
// error and not a returned result.
func TestExecuteAndWait_FailedExecuteStatusIsAnError(t *testing.T) {
	for _, status := range []string{"ABORTED", "FAILED", "SOMETHING_NEW"} {
		t.Run(status, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:execute", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(ExecuteResult{Result: &AnalyzerResult{ExecutionStatus: status}})
			})

			h := NewHandler(newTestClient(t, mux))
			if _, err := h.ExecuteAndWait(context.Background(), "dt.test", map[string]interface{}{}, 60); err == nil {
				t.Fatalf("ExecuteAndWait() error = nil, want an error for status %s", status)
			}
		})
	}
}

func TestExecuteAndWait_UnknownStatusWithTokenIsPolled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:execute", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ExecuteResult{RequestToken: "tok", Result: &AnalyzerResult{ExecutionStatus: "QUEUED"}})
	})
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:poll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ExecuteResult{Result: &AnalyzerResult{ExecutionStatus: "COMPLETED"}})
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
