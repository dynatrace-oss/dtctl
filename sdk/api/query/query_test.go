package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestExecute_Synchronous(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req ExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Query != "fetch logs | limit 1" {
			t.Errorf("unexpected query: %s", req.Query)
		}

		resp := Response{
			State: "SUCCEEDED",
			Result: &Result{
				Records: []map[string]interface{}{{"message": "hello"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.Execute(context.Background(), ExecuteRequest{Query: "fetch logs | limit 1"})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.State != "SUCCEEDED" {
		t.Errorf("State = %q, want SUCCEEDED", result.State)
	}
	if len(result.GetRecords()) != 1 {
		t.Errorf("got %d records, want 1", len(result.GetRecords()))
	}
}

func TestExecute_AsyncWithPoll(t *testing.T) {
	var pollCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{State: "RUNNING", RequestToken: "tok-123"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("request-token") != "tok-123" {
			t.Errorf("unexpected request-token: %s", r.URL.Query().Get("request-token"))
		}

		n := pollCount.Add(1)
		if n < 2 {
			resp := Response{State: "RUNNING", RequestToken: "tok-123", Progress: 50}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		resp := Response{
			State: "SUCCEEDED",
			Result: &Result{
				Records: []map[string]interface{}{{"key": "value"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs"}, nil)
	if err != nil {
		t.Fatalf("ExecuteAndPoll() error: %v", err)
	}
	if result.State != "SUCCEEDED" {
		t.Errorf("State = %q, want SUCCEEDED", result.State)
	}
	if pollCount.Load() < 2 {
		t.Errorf("expected at least 2 polls, got %d", pollCount.Load())
	}
}

func TestPoll_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := Response{
			State: "SUCCEEDED",
			Result: &Result{
				Records: []map[string]interface{}{{"a": 1}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.Poll(context.Background(), "tok-abc", 5000)
	if err != nil {
		t.Fatalf("Poll() error: %v", err)
	}
	if result.State != "SUCCEEDED" {
		t.Errorf("State = %q, want SUCCEEDED", result.State)
	}
}

func TestPoll_ErrorReturnsAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"jwt expired"}}`))
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Poll(context.Background(), "tok-abc", 5000)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *httpclient.APIError
	if !errorAsAPIError(err, &apiErr) {
		t.Fatalf("expected *httpclient.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
}

func TestCancel(t *testing.T) {
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:cancel", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("request-token") != "tok-cancel" {
			t.Errorf("unexpected request-token: %s", r.URL.Query().Get("request-token"))
		}
		w.WriteHeader(http.StatusOK)
	})

	h := NewHandler(newTestClient(t, mux))
	err := h.Cancel(context.Background(), "tok-cancel")
	if err != nil {
		t.Fatalf("Cancel() error: %v", err)
	}
	if !called.Load() {
		t.Error("cancel endpoint was not called")
	}
}

func TestVerify(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:verify", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := VerifyResponse{
			Valid:          true,
			CanonicalQuery: "fetch logs",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.Verify(context.Background(), VerifyRequest{Query: "fetch logs", GenerateCanonicalQuery: true})
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if !result.Valid {
		t.Error("expected valid query")
	}
	if result.CanonicalQuery != "fetch logs" {
		t.Errorf("CanonicalQuery = %q, want %q", result.CanonicalQuery, "fetch logs")
	}
}

func TestExecuteAndPoll_Cancellation(t *testing.T) {
	var cancelCalled atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{State: "RUNNING", RequestToken: "tok-cancel-test"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response — give the test time to cancel
		time.Sleep(200 * time.Millisecond)
		resp := Response{State: "RUNNING", RequestToken: "tok-cancel-test"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/platform/storage/query/v1/query:cancel", func(w http.ResponseWriter, r *http.Request) {
		cancelCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	})

	h := NewHandler(newTestClient(t, mux))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := h.ExecuteAndPoll(ctx, ExecuteRequest{Query: "fetch logs"}, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	// Give best-effort cancel a moment to complete
	time.Sleep(100 * time.Millisecond)
	if !cancelCalled.Load() {
		t.Error("expected cancel endpoint to be called")
	}
}

func TestExecuteAndPoll_QueryFailed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{State: "RUNNING", RequestToken: "tok-fail"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{State: "FAILED"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "bad query"}, nil)
	if err == nil {
		t.Fatal("expected error for FAILED query")
	}
}

func TestExecute_ErrorResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"invalid query","details":{"errorType":"SYNTAX_ERROR","errorMessage":"parse error","arguments":[]}}}`))
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Execute(context.Background(), ExecuteRequest{Query: "bad"})
	if err == nil {
		t.Fatal("expected error")
	}

	var qErr *QueryError
	if !errorAsQueryError(err, &qErr) {
		t.Fatalf("expected *QueryError, got %T: %v", err, err)
	}
	if qErr.ErrorType != "SYNTAX_ERROR" {
		t.Errorf("ErrorType = %q, want SYNTAX_ERROR", qErr.ErrorType)
	}
}

func TestWithHeaders(t *testing.T) {
	var gotHeader string
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("dt-client-context")
		resp := Response{State: "SUCCEEDED"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux)).WithHeaders(map[string]string{
		"dt-client-context": `{"app":"test"}`,
	})
	_, err := h.Execute(context.Background(), ExecuteRequest{Query: "fetch logs"})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotHeader != `{"app":"test"}` {
		t.Errorf("dt-client-context = %q, want %q", gotHeader, `{"app":"test"}`)
	}
}

func TestResponse_GetRecords(t *testing.T) {
	t.Run("from Result", func(t *testing.T) {
		r := &Response{
			Result: &Result{Records: []map[string]interface{}{{"a": 1}}},
		}
		if len(r.GetRecords()) != 1 {
			t.Errorf("got %d records, want 1", len(r.GetRecords()))
		}
	})

	t.Run("from top-level Records", func(t *testing.T) {
		r := &Response{
			Records: []map[string]interface{}{{"b": 2}},
		}
		if len(r.GetRecords()) != 1 {
			t.Errorf("got %d records, want 1", len(r.GetRecords()))
		}
	})

	t.Run("empty", func(t *testing.T) {
		r := &Response{}
		if len(r.GetRecords()) != 0 {
			t.Errorf("got %d records, want 0", len(r.GetRecords()))
		}
	})
}

func TestResponse_GetNotifications(t *testing.T) {
	t.Run("from top-level metadata", func(t *testing.T) {
		r := &Response{
			Metadata: &Metadata{
				Grail: &GrailMetadata{
					Notifications: []Notification{{Severity: "WARNING", Message: "scan limit"}},
				},
			},
		}
		if len(r.GetNotifications()) != 1 {
			t.Errorf("got %d notifications, want 1", len(r.GetNotifications()))
		}
	})

	t.Run("from result metadata", func(t *testing.T) {
		r := &Response{
			Result: &Result{
				Metadata: &Metadata{
					Grail: &GrailMetadata{
						Notifications: []Notification{{Severity: "INFO", Message: "ok"}},
					},
				},
			},
		}
		if len(r.GetNotifications()) != 1 {
			t.Errorf("got %d notifications, want 1", len(r.GetNotifications()))
		}
	})
}

func TestResponse_GetMetadata(t *testing.T) {
	r := &Response{
		Metadata: &Metadata{
			Grail: &GrailMetadata{
				QueryID:    "q-123",
				DQLVersion: "1.0",
			},
		},
	}
	m := r.GetMetadata()
	if m == nil {
		t.Fatal("expected metadata")
	}
	if m.QueryID != "q-123" {
		t.Errorf("QueryID = %q, want q-123", m.QueryID)
	}
}

// helpers to avoid importing errors in test
func errorAsAPIError(err error, target **httpclient.APIError) bool {
	for err != nil {
		if e, ok := err.(*httpclient.APIError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func errorAsQueryError(err error, target **QueryError) bool {
	for err != nil {
		if e, ok := err.(*QueryError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// TestExecuteAndPoll_OnUnauthorizedRetry verifies that a 401 during polling triggers
// the onUnauthorized callback and retries the poll successfully.
func TestExecuteAndPoll_OnUnauthorizedRetry(t *testing.T) {
	var pollCount atomic.Int32
	var refreshCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{State: "RUNNING", RequestToken: "tok-auth"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		n := pollCount.Add(1)
		if n == 1 {
			// First poll: simulate expired token
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"jwt expired"}}`))
			return
		}
		// Second poll: verify the refreshed token was applied to the client
		if got := r.Header.Get("Authorization"); got != "Bearer new-token" {
			t.Errorf("retry poll Authorization = %q, want %q", got, "Bearer new-token")
		}
		// Second poll: succeed
		resp := Response{
			State:  "SUCCEEDED",
			Result: &Result{Records: []map[string]interface{}{{"refreshed": true}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs"}, func() (string, error) {
		refreshCount.Add(1)
		return "new-token", nil
	})
	if err != nil {
		t.Fatalf("ExecuteAndPoll() error: %v", err)
	}
	if result.State != "SUCCEEDED" {
		t.Errorf("State = %q, want SUCCEEDED", result.State)
	}
	if refreshCount.Load() != 1 {
		t.Errorf("expected 1 token refresh, got %d", refreshCount.Load())
	}
	if pollCount.Load() != 2 {
		t.Errorf("expected 2 polls (1 failed + 1 retry), got %d", pollCount.Load())
	}
}

// TestExecuteAndPoll_OnUnauthorizedAbortOnConsecutive401 verifies that two consecutive
// 401 responses (even after a successful refresh) cause the poll to fail instead of
// looping infinitely.
func TestExecuteAndPoll_OnUnauthorizedAbortOnConsecutive401(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{State: "RUNNING", RequestToken: "tok-double-401"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		// Always return 401 — credentials are fundamentally broken
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"jwt expired"}}`))
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs"}, func() (string, error) {
		return "still-bad-token", nil
	})
	if err == nil {
		t.Fatal("expected error after consecutive 401s")
	}
}

// TestExecuteAndPoll_ImmediateSuccess verifies that ExecuteAndPoll returns immediately
// when the execute response already contains the completed result (no polling needed).
func TestExecuteAndPoll_ImmediateSuccess(t *testing.T) {
	var pollCalled atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			State: "SUCCEEDED",
			Result: &Result{
				Records: []map[string]interface{}{{"instant": true}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/platform/storage/query/v1/query:poll", func(w http.ResponseWriter, r *http.Request) {
		pollCalled.Store(true)
		t.Error("poll should not be called for immediate success")
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs | limit 1"}, nil)
	if err != nil {
		t.Fatalf("ExecuteAndPoll() error: %v", err)
	}
	if result.State != "SUCCEEDED" {
		t.Errorf("State = %q, want SUCCEEDED", result.State)
	}
	if pollCalled.Load() {
		t.Error("poll endpoint should not have been called")
	}
}

// TestExecute_RequestBodySerialization verifies that all ExecuteRequest fields are
// correctly serialized in the HTTP request body.
func TestExecute_RequestBodySerialization(t *testing.T) {
	var gotReq ExecuteRequest

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		resp := Response{State: "SUCCEEDED"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	includeTypes := true
	includeContrib := true
	req := ExecuteRequest{
		Query:                        "fetch logs",
		RequestTimeoutMilliseconds:   5000,
		MaxResultRecords:             100,
		MaxResultBytes:               1024,
		DefaultScanLimitGbytes:       500,
		DefaultSamplingRatio:         0.1,
		FetchTimeoutSeconds:          30,
		EnablePreview:                true,
		EnforceQueryConsumptionLimit: true,
		IncludeTypes:                 &includeTypes,
		IncludeContributions:         &includeContrib,
		DefaultTimeframeStart:        "2024-01-01T00:00:00Z",
		DefaultTimeframeEnd:          "2024-01-02T00:00:00Z",
		Locale:                       "en_US",
		Timezone:                     "Europe/Vienna",
		FilterSegments: []FilterSegmentRef{
			{ID: "seg-1", Variables: []FilterSegmentVariable{{Name: "host", Values: []string{"a", "b"}}}},
		},
	}

	h := NewHandler(newTestClient(t, mux))
	_, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if gotReq.Query != "fetch logs" {
		t.Errorf("Query = %q", gotReq.Query)
	}
	if gotReq.MaxResultRecords != 100 {
		t.Errorf("MaxResultRecords = %d, want 100", gotReq.MaxResultRecords)
	}
	if gotReq.DefaultScanLimitGbytes != 500 {
		t.Errorf("DefaultScanLimitGbytes = %f, want 500", gotReq.DefaultScanLimitGbytes)
	}
	if gotReq.Timezone != "Europe/Vienna" {
		t.Errorf("Timezone = %q, want Europe/Vienna", gotReq.Timezone)
	}
	if gotReq.IncludeTypes == nil || !*gotReq.IncludeTypes {
		t.Error("IncludeTypes should be true")
	}
	if len(gotReq.FilterSegments) != 1 || gotReq.FilterSegments[0].ID != "seg-1" {
		t.Errorf("FilterSegments = %v, want [{seg-1 ...}]", gotReq.FilterSegments)
	}
	if len(gotReq.FilterSegments[0].Variables) != 1 || gotReq.FilterSegments[0].Variables[0].Name != "host" {
		t.Error("FilterSegments[0].Variables should contain host variable")
	}
}

// TestVerify_InvalidQuery verifies that a query with syntax errors returns proper
// notifications in the VerifyResponse.
func TestVerify_InvalidQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:verify", func(w http.ResponseWriter, r *http.Request) {
		resp := VerifyResponse{
			Valid: false,
			Notifications: []VerifyNotification{
				{
					Severity:         "ERROR",
					NotificationType: "SYNTAX_ERROR",
					Message:          "unexpected token 'bogus'",
					SyntaxPosition: &SyntaxPosition{
						Start: &Position{Line: 1, Column: 0},
						End:   &Position{Line: 1, Column: 5},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	result, err := h.Verify(context.Background(), VerifyRequest{Query: "bogus"})
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if result.Valid {
		t.Error("expected invalid query")
	}
	if len(result.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(result.Notifications))
	}
	n := result.Notifications[0]
	if n.NotificationType != "SYNTAX_ERROR" {
		t.Errorf("NotificationType = %q, want SYNTAX_ERROR", n.NotificationType)
	}
	if n.SyntaxPosition == nil || n.SyntaxPosition.Start.Line != 1 {
		t.Error("expected syntax position with line 1")
	}
}

// TestWithHeaders_PropagatedToAllEndpoints verifies that custom headers are sent
// on execute, poll, cancel, and verify requests.
func TestWithHeaders_PropagatedToAllEndpoints(t *testing.T) {
	var headers sync.Map // endpoint -> header value

	mux := http.NewServeMux()
	recordHeader := func(endpoint string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			headers.Store(endpoint, r.Header.Get("x-custom"))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Response{State: "SUCCEEDED"})
		}
	}
	mux.HandleFunc("/platform/storage/query/v1/query:execute", recordHeader("execute"))
	mux.HandleFunc("/platform/storage/query/v1/query:poll", recordHeader("poll"))
	mux.HandleFunc("/platform/storage/query/v1/query:cancel", recordHeader("cancel"))
	mux.HandleFunc("/platform/storage/query/v1/query:verify", recordHeader("verify"))

	h := NewHandler(newTestClient(t, mux)).WithHeaders(map[string]string{"x-custom": "test-value"})

	h.Execute(context.Background(), ExecuteRequest{Query: "q"})
	h.Poll(context.Background(), "tok", 1000)
	h.Cancel(context.Background(), "tok")
	h.Verify(context.Background(), VerifyRequest{Query: "q"})

	for _, ep := range []string{"execute", "poll", "cancel", "verify"} {
		val, ok := headers.Load(ep)
		if !ok || val.(string) != "test-value" {
			t.Errorf("endpoint %q: x-custom = %q, want %q", ep, val, "test-value")
		}
	}
}

// TestCancel_ServerError verifies that Cancel returns a structured error
// when the server returns a non-2xx status.
func TestCancel_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:cancel", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("oops"))
	})

	h := NewHandler(newTestClient(t, mux))
	err := h.Cancel(context.Background(), "tok")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500: %v", err)
	}
}

// TestQueryError_ErrorFormatting verifies the Error() method produces sensible messages.
func TestQueryError_ErrorFormatting(t *testing.T) {
	t.Run("with error type", func(t *testing.T) {
		e := &QueryError{StatusCode: 400, Message: "bad", ErrorType: "SYNTAX_ERROR"}
		want := "query failed (SYNTAX_ERROR): bad"
		if e.Error() != want {
			t.Errorf("Error() = %q, want %q", e.Error(), want)
		}
	})

	t.Run("without error type", func(t *testing.T) {
		e := &QueryError{StatusCode: 500, Message: "internal"}
		want := "query failed with status 500: internal"
		if e.Error() != want {
			t.Errorf("Error() = %q, want %q", e.Error(), want)
		}
	})
}

// TestExecuteAndPoll_SetsRequestTimeout verifies that ExecuteAndPoll sets the
// RequestTimeoutMilliseconds field if the caller omits it.
func TestExecuteAndPoll_SetsRequestTimeout(t *testing.T) {
	var gotTimeout int64

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		var req ExecuteRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotTimeout = req.RequestTimeoutMilliseconds

		resp := Response{State: "SUCCEEDED"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	// Don't set RequestTimeoutMilliseconds — ExecuteAndPoll should default it
	_, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs"}, nil)
	if err != nil {
		t.Fatalf("ExecuteAndPoll() error: %v", err)
	}
	if gotTimeout != pollRequestTimeoutMs {
		t.Errorf("RequestTimeoutMilliseconds = %d, want %d", gotTimeout, pollRequestTimeoutMs)
	}
}

// TestExecuteAndPoll_PreservesCallerTimeout verifies that ExecuteAndPoll does NOT
// overwrite a caller-supplied RequestTimeoutMilliseconds value.
func TestExecuteAndPoll_PreservesCallerTimeout(t *testing.T) {
	var gotTimeout int64

	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		var req ExecuteRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotTimeout = req.RequestTimeoutMilliseconds

		resp := Response{State: "SUCCEEDED"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{
		Query:                      "fetch logs",
		RequestTimeoutMilliseconds: 10000,
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAndPoll() error: %v", err)
	}
	if gotTimeout != 10000 {
		t.Errorf("RequestTimeoutMilliseconds = %d, want 10000 (caller value)", gotTimeout)
	}
}

// TestExecuteAndPoll_NoRequestToken verifies that ExecuteAndPoll returns an error
// when the server returns RUNNING state without a request token.
func TestExecuteAndPoll_NoRequestToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/platform/storage/query/v1/query:execute", func(w http.ResponseWriter, r *http.Request) {
		// RUNNING but no request token — broken server response
		resp := Response{State: "RUNNING"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
	})

	h := NewHandler(newTestClient(t, mux))
	_, err := h.ExecuteAndPoll(context.Background(), ExecuteRequest{Query: "fetch logs"}, nil)
	if err == nil {
		t.Fatal("expected error for missing request token")
	}
	if !strings.Contains(err.Error(), "no request token") {
		t.Errorf("error should mention missing token: %v", err)
	}
}
