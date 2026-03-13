package wait

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dynatrace-oss/dtctl/pkg/client"
	"github.com/dynatrace-oss/dtctl/pkg/exec"
)

// newWaiterTestExecutor creates a DQL executor backed by a test server.
func newWaiterTestExecutor(t *testing.T, handler http.HandlerFunc) (*exec.DQLExecutor, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := client.NewForTesting(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return exec.NewDQLExecutor(c), srv.Close
}

// --- NewQueryWaiter ---

func TestNewQueryWaiter_Defaults(t *testing.T) {
	executor, cleanup := newWaiterTestExecutor(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()

	config := WaitConfig{
		Query:     "fetch logs",
		Condition: Condition{Type: ConditionTypeAny, Operator: OpGreater, Value: 0},
		Timeout:   0,
	}
	waiter := NewQueryWaiter(executor, config)
	if waiter == nil {
		t.Fatal("NewQueryWaiter returned nil")
	}
	// ProgressOut should default to os.Stderr (non-nil)
	if waiter.config.ProgressOut == nil {
		t.Error("expected ProgressOut to be set to default")
	}
}

func TestNewQueryWaiter_CustomProgressOut(t *testing.T) {
	executor, cleanup := newWaiterTestExecutor(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()

	buf := &bytes.Buffer{}
	config := WaitConfig{
		Query:       "fetch logs",
		Condition:   Condition{Type: ConditionTypeAny, Operator: OpGreater, Value: 0},
		ProgressOut: buf,
	}
	waiter := NewQueryWaiter(executor, config)
	if waiter.config.ProgressOut != buf {
		t.Error("expected custom ProgressOut to be preserved")
	}
}

// --- Wait: immediate success ---

func TestWait_ImmediateSuccess(t *testing.T) {
	executor, cleanup := newWaiterTestExecutor(t, func(w http.ResponseWriter, r *http.Request) {
		response := exec.DQLQueryResponse{
			State: "SUCCEEDED",
			Result: &exec.DQLResult{
				Records: []map[string]interface{}{
					{"host.name": "server-01"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})
	defer cleanup()

	buf := &bytes.Buffer{}
	config := WaitConfig{
		Query:       "fetch hosts",
		Condition:   Condition{Type: ConditionTypeAny, Operator: OpGreater, Value: 0},
		MaxAttempts: 5,
		Quiet:       true,
		ProgressOut: buf,
		Backoff:     DefaultBackoffConfig(),
	}
	// Override backoff for speed
	config.Backoff.MinInterval = 0
	config.Backoff.MaxInterval = 0

	waiter := NewQueryWaiter(executor, config)
	ctx := context.Background()
	result, err := waiter.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.FailureReason)
	}
}

// --- Wait: max attempts exceeded ---

func TestWait_MaxAttemptsExceeded(t *testing.T) {
	callCount := 0
	executor, cleanup := newWaiterTestExecutor(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Always return empty records (condition never met)
		response := exec.DQLQueryResponse{
			State:  "SUCCEEDED",
			Result: &exec.DQLResult{Records: []map[string]interface{}{}},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})
	defer cleanup()

	buf := &bytes.Buffer{}
	config := WaitConfig{
		Query:       "fetch logs",
		Condition:   Condition{Type: ConditionTypeCount, Operator: OpGreaterEqual, Value: 10},
		MaxAttempts: 2,
		Quiet:       true,
		ProgressOut: buf,
		Backoff:     BackoffConfig{MinInterval: 0, MaxInterval: 0},
	}
	waiter := NewQueryWaiter(executor, config)
	ctx := context.Background()
	result, err := waiter.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result.Success {
		t.Error("expected failure (max attempts), got success")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

// --- Wait: context timeout ---

func TestWait_ContextTimeout(t *testing.T) {
	executor, cleanup := newWaiterTestExecutor(t, func(w http.ResponseWriter, r *http.Request) {
		// Return empty - condition never met
		response := exec.DQLQueryResponse{
			State:  "SUCCEEDED",
			Result: &exec.DQLResult{Records: []map[string]interface{}{}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
	defer cleanup()

	buf := &bytes.Buffer{}
	config := WaitConfig{
		Query:       "fetch logs",
		Condition:   Condition{Type: ConditionTypeCount, Operator: OpGreaterEqual, Value: 100},
		Quiet:       true,
		ProgressOut: buf,
		Timeout:     50, // 50ns — will expire almost immediately
		Backoff:     BackoffConfig{MinInterval: 0, MaxInterval: 0},
	}
	waiter := NewQueryWaiter(executor, config)
	ctx := context.Background()
	result, err := waiter.Wait(ctx)
	// Context timeout returns ctx.Err() — either nil result or timeout error
	if err == nil && result != nil && result.Success {
		t.Error("expected non-success for timed-out wait")
	}
}

// --- PrintResults ---

func TestPrintResults_EmptyFormat(t *testing.T) {
	executor, cleanup := newWaiterTestExecutor(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()

	waiter := NewQueryWaiter(executor, WaitConfig{OutputFormat: ""})
	// Should return nil for empty format
	err := waiter.PrintResults(&Result{Records: []map[string]any{{"key": "val"}}})
	if err != nil {
		t.Fatalf("PrintResults() with empty format error = %v", err)
	}
}

func TestPrintResults_NilRecords(t *testing.T) {
	executor, cleanup := newWaiterTestExecutor(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()

	waiter := NewQueryWaiter(executor, WaitConfig{OutputFormat: "json"})
	// Should return nil for nil records
	err := waiter.PrintResults(&Result{Records: nil})
	if err != nil {
		t.Fatalf("PrintResults() with nil records error = %v", err)
	}
}

func TestPrintResults_JSONFormat(t *testing.T) {
	executor, cleanup := newWaiterTestExecutor(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()

	waiter := NewQueryWaiter(executor, WaitConfig{OutputFormat: "json"})
	err := waiter.PrintResults(&Result{
		Records: []map[string]any{{"host": "server-01", "status": "ok"}},
	})
	if err != nil {
		t.Fatalf("PrintResults() JSON error = %v", err)
	}
}

// Test verbose mode path
func TestWait_VerboseOutput(t *testing.T) {
	executor, cleanup := newWaiterTestExecutor(t, func(w http.ResponseWriter, r *http.Request) {
		response := exec.DQLQueryResponse{
			State:  "SUCCEEDED",
			Result: &exec.DQLResult{Records: []map[string]interface{}{{"x": 1}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
	defer cleanup()

	buf := &bytes.Buffer{}
	config := WaitConfig{
		Query:       "fetch logs",
		Condition:   Condition{Type: ConditionTypeAny, Operator: OpGreater, Value: 0},
		MaxAttempts: 1,
		Quiet:       false,
		Verbose:     true,
		Timeout:     5000000000, // 5 seconds
		ProgressOut: buf,
		Backoff:     BackoffConfig{MinInterval: 0, MaxInterval: 0},
	}
	waiter := NewQueryWaiter(executor, config)
	ctx := context.Background()
	_, err := waiter.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait() verbose error = %v", err)
	}
	// Verbose mode should have written something to ProgressOut
	output := buf.String()
	if len(output) == 0 {
		t.Error("expected verbose output, got nothing")
	}
	_ = fmt.Sprintf("verbose output: %q", output) // suppress unused
}
