package httpclient

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

// testRetries is the retry budget the tests below configure. A request that is
// retried therefore reaches the server testRetries+1 times; one that is not,
// exactly once.
const testRetries = 3

func newRetryTestClient(t *testing.T, baseURL string, opts ...Option) *Client {
	t.Helper()
	opts = append([]Option{
		WithToken("dt0c01.test"),
		WithRetry(testRetries, time.Millisecond, time.Millisecond),
	}, opts...)
	c, err := New(baseURL, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestClient_RetryOnlyResendsSafeRequests pins which failed requests the
// default retry condition resends. A 5xx on a POST or PATCH may arrive after
// the server already acted on the request, so resending it could duplicate
// the work; a 429 (or a 503 with Retry-After) says it was turned away.
func TestClient_RetryOnlyResendsSafeRequests(t *testing.T) {
	tests := []struct {
		method     string
		status     int
		retryAfter string
		retried    bool
	}{
		// Non-idempotent methods: a server error is final.
		{method: http.MethodPost, status: http.StatusInternalServerError},
		{method: http.MethodPost, status: http.StatusBadGateway},
		{method: http.MethodPost, status: http.StatusServiceUnavailable},
		{method: http.MethodPost, status: http.StatusGatewayTimeout},
		{method: http.MethodPatch, status: http.StatusInternalServerError},
		{method: http.MethodPatch, status: http.StatusBadGateway},

		// ...unless the server says it did not process the request.
		{method: http.MethodPost, status: http.StatusTooManyRequests, retried: true},
		{method: http.MethodPatch, status: http.StatusTooManyRequests, retried: true},
		{method: http.MethodPost, status: http.StatusServiceUnavailable, retryAfter: "1", retried: true},

		// Idempotent methods: any 5xx is retried.
		{method: http.MethodGet, status: http.StatusInternalServerError, retried: true},
		{method: http.MethodGet, status: http.StatusBadGateway, retried: true},
		{method: http.MethodGet, status: http.StatusServiceUnavailable, retried: true},
		{method: http.MethodGet, status: http.StatusTooManyRequests, retried: true},
		{method: http.MethodHead, status: http.StatusBadGateway, retried: true},
		{method: http.MethodPut, status: http.StatusInternalServerError, retried: true},
		{method: http.MethodDelete, status: http.StatusGatewayTimeout, retried: true},

		// Client errors are never retried, whatever the method.
		{method: http.MethodGet, status: http.StatusNotFound},
		{method: http.MethodPost, status: http.StatusBadRequest},
	}

	for _, tt := range tests {
		name := tt.method + " " + http.StatusText(tt.status)
		if tt.retryAfter != "" {
			name += " with Retry-After"
		}
		t.Run(name, func(t *testing.T) {
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := newRetryTestClient(t, srv.URL)
			resp, err := c.HTTP().R().Execute(tt.method, "/platform/x/v1/y")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode() != tt.status {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.status)
			}

			want := int32(1)
			if tt.retried {
				want = testRetries + 1
			}
			if got := attempts.Load(); got != want {
				t.Errorf("attempts = %d, want %d", got, want)
			}
		})
	}
}

// TestClient_RetryOnDroppedConnection: the request reached the server, which
// then dropped the connection without answering. Whether it acted on the
// request first is unknowable, so only an idempotent request is resent.
func TestClient_RetryOnDroppedConnection(t *testing.T) {
	tests := []struct {
		method  string
		retried bool
	}{
		{method: http.MethodPost, retried: false},
		{method: http.MethodPatch, retried: false},
		{method: http.MethodGet, retried: true},
		{method: http.MethodPut, retried: true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Errorf("hijack: %v", err)
					return
				}
				_ = conn.Close()
			}))
			defer srv.Close()

			c := newRetryTestClient(t, srv.URL)
			if _, err := c.HTTP().R().SetBody(`{}`).Execute(tt.method, "/platform/x/v1/y"); err == nil {
				t.Fatal("expected a transport error")
			}

			want := int32(1)
			if tt.retried {
				want = testRetries + 1
			}
			if got := attempts.Load(); got != want {
				t.Errorf("attempts = %d, want %d", got, want)
			}
		})
	}
}

// countingTransport counts the round trips it is asked to make.
type countingTransport struct {
	next  http.RoundTripper
	count atomic.Int32
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.count.Add(1)
	return c.next.RoundTrip(req)
}

// TestClient_RetryPostWhenConnectionRefused: a dial failure means no byte of
// the request was written, so even a POST is safe to send again.
func TestClient_RetryPostWhenConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listens here any more: every dial is refused

	transport := &countingTransport{next: &http.Transport{}}
	c := newRetryTestClient(t, "http://"+addr, WithTransport(transport))
	if _, err := c.HTTP().R().SetBody(`{}`).Post("/platform/x/v1/y"); err == nil {
		t.Fatal("expected a connection error")
	}

	if got := transport.count.Load(); got != testRetries+1 {
		t.Errorf("attempts = %d, want %d", got, testRetries+1)
	}
}

// TestClient_RequestLevelConditionOptsIntoRetry pins the documented escape
// hatch: a request-level retry condition is consulted before the client's, so
// a caller that knows one POST is safe to resend can say so for that request.
func TestClient_RequestLevelConditionOptsIntoRetry(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newRetryTestClient(t, srv.URL)
	resp, err := c.HTTP().R().
		AddRetryCondition(func(r *resty.Response, err error) bool {
			return err == nil && r.StatusCode() >= 500
		}).
		Post("/platform/x/v1/y:verify")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode())
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

// TestIsRetryable_NoResponse covers the inputs no live request produces
// deterministically: a request rejected before it was sent, and a cancelled
// context.
func TestIsRetryable_NoResponse(t *testing.T) {
	if IsRetryable(nil, http.ErrServerClosed) {
		t.Error("a request that was never sent must not be retried")
	}
	if IsRetryable(nil, nil) {
		t.Error("a nil response without an error must not be retried")
	}
	req := &resty.Request{Method: http.MethodGet}
	if IsRetryable(&resty.Response{Request: req}, &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}) {
		t.Error("a dial that hit the context deadline must not be retried")
	}
}
