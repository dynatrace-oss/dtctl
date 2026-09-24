package analyzer

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestExecuteAndWait_FailedExecuteStatusIsAnError covers #495: ABORTED or
// FAILED in the execute response, which carries no request token, must be an
// error and not a returned result.
func TestExecuteAndWait_FailedExecuteStatusIsAnError(t *testing.T) {
	cases := map[string]string{
		"ABORTED":       "analyzer execution was aborted",
		"FAILED":        "analyzer execution failed",
		"SOMETHING_NEW": `ended in status "SOMETHING_NEW" with no request token`,
	}
	for status, wantMsg := range cases {
		t.Run(status, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:execute", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(ExecuteResult{Result: &AnalyzerResult{ExecutionStatus: status}})
			})

			h := NewHandler(newTestClient(t, mux))
			_, err := h.ExecuteAndWait(context.Background(), "dt.test", map[string]interface{}{}, 60)
			if err == nil {
				t.Fatalf("ExecuteAndWait() error = nil, want an error for status %s", status)
			}
			// Assert which branch fired: "some error" would also pass when the
			// no-result branch reports a result that is in fact present.
			if !strings.Contains(err.Error(), wantMsg) {
				t.Errorf("ExecuteAndWait() error = %q, want it to contain %q", err, wantMsg)
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

// TestExecuteAndWait_ResultWithoutStatusIsDelivered pins that a synchronous
// response carrying a result but no executionStatus is handed over. It is not
// "neither a result nor a request token", and inventing a failure out of a
// missing field would drop data the caller already has.
func TestExecuteAndWait_ResultWithoutStatusIsDelivered(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:execute", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ExecuteResult{Result: &AnalyzerResult{ResultID: "res-1"}})
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.ExecuteAndWait(context.Background(), "dt.test", map[string]interface{}{}, 60)
	if err != nil {
		t.Fatalf("ExecuteAndWait() error: %v", err)
	}
	if result.Result.ResultID != "res-1" {
		t.Errorf("ResultID = %q, want res-1", result.Result.ResultID)
	}
}

// TestExecuteAndWait_NoResultAndNoTokenIsAnError keeps the other half of that
// branch honest: an empty envelope has nothing to return and nothing to poll.
func TestExecuteAndWait_NoResultAndNoTokenIsAnError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/davis/analyzers/v1/analyzers/dt.test:execute", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ExecuteResult{})
	})

	h := NewHandler(newTestClient(t, mux))
	if _, err := h.ExecuteAndWait(context.Background(), "dt.test", map[string]interface{}{}, 60); err == nil {
		t.Fatal("ExecuteAndWait() error = nil, want an error for an empty envelope")
	}
}
