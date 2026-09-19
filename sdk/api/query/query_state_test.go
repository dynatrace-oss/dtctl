package query

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
)

// stateServer answers the execute call with execute and every poll with the
// next entry of polls (the last one repeats).
func stateServer(t *testing.T, execute Response, polls ...Response) (*Handler, *atomic.Int32) {
	t.Helper()
	var pollCount atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(execute)
	})
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		n := int(pollCount.Add(1))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(polls[min(n, len(polls))-1])
	})
	return NewHandler(newTestClient(t, mux)), &pollCount
}

// TestExecuteAndPoll_FailureStatesAreErrors covers #495: a failure state in
// the execute response or in a poll must be an error, not a success with zero
// records.
func TestExecuteAndPoll_FailureStatesAreErrors(t *testing.T) {
	for _, state := range []string{StateFailed, StateCancelled, StateResultGone} {
		t.Run("execute "+state, func(t *testing.T) {
			h, polls := stateServer(t, Response{State: state, RequestToken: "tok"}, Response{State: StateSucceeded})

			_, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs"}, nil)

			assertStateError(t, err, state)
			if polls.Load() != 0 {
				t.Errorf("polled %d times, want 0 for a terminal execute state", polls.Load())
			}
		})
		t.Run("poll "+state, func(t *testing.T) {
			h, _ := stateServer(t, Response{State: StateRunning, RequestToken: "tok"}, Response{State: state})

			_, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs"}, nil)

			assertStateError(t, err, state)
		})
	}
}

func TestExecuteAndPoll_UnknownStateWithTokenIsPolled(t *testing.T) {
	h, polls := stateServer(t, Response{State: "QUEUED", RequestToken: "tok"},
		Response{State: "QUEUED", RequestToken: "tok"},
		Response{State: StateSucceeded, Result: &Result{Records: []map[string]interface{}{{"c": "1"}}}})

	result, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs"}, nil)
	if err != nil {
		t.Fatalf("ExecuteAndPoll() error: %v", err)
	}
	if result.State != StateSucceeded {
		t.Errorf("State = %q, want SUCCEEDED", result.State)
	}
	if polls.Load() < 2 {
		t.Errorf("polled %d times, want at least 2", polls.Load())
	}
}

func TestExecuteAndPoll_UnknownStateWithoutTokenIsAnError(t *testing.T) {
	h, _ := stateServer(t, Response{State: "QUEUED"})

	_, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs"}, nil)

	assertStateError(t, err, "QUEUED")
}

// TestExecuteAndPoll_CancelRacingCancelledPollIsCancellation makes sure a
// Ctrl-C that races a poll answering CANCELLED stays a context cancellation.
func TestExecuteAndPoll_CancelRacingCancelledPollIsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(Response{State: StateRunning, RequestToken: "tok"})
	})
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Response{State: StateCancelled})
	})
	mux.HandleFunc("/platform/storage/query/v1/query:cancel", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	h := NewHandler(newTestClient(t, mux))

	_, err := h.ExecuteAndPoll(ctx, ExecuteRequest{Query: "fetch logs"}, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func assertStateError(t *testing.T, err error, state string) {
	t.Helper()
	var stateErr *StateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("error = %v, want a *StateError", err)
	}
	if stateErr.State != state {
		t.Errorf("StateError.State = %q, want %q", stateErr.State, state)
	}
}
