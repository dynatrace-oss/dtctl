package engine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dynatrace-oss/dtctl/pkg/engine"
)

// TestConcurrentDeadlineEndsAStalledRequest holds the design's single deadline:
// the request's context. Nothing preempts in-process code, so a command whose
// upstream never answers stays in its slot until its HTTP call ends, and the
// SDK's own ceiling on that call is six minutes. The context must therefore
// reach every request an invocation sends — including those from handlers that
// call the SDK with context.Background(), which is nearly all of them (`get
// workflows` and `get buckets`). The query execute is the exception, on purpose:
// the SDK detaches it from cancellation for a short grace so that a query it
// started can still be cancelled on the backend (dtctl #621).
func TestConcurrentDeadlineEndsAStalledRequest(t *testing.T) {
	stalled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stalled:
		case <-time.After(time.Minute):
		}
	}))
	t.Cleanup(func() {
		close(stalled)
		upstream.Close()
	})

	const budget = 400 * time.Millisecond
	eng := engine.New(engine.Limits{MaxQueued: 8, MaxDuration: time.Minute}, engine.WithConcurrentExecution(4))

	for _, command := range []string{
		"get workflows",       // the handler calls the SDK with context.Background()
		"get buckets",         // likewise
		"get documents",       // a second background-context handler
		"describe workflow x", // a request that is not a plain list
	} {
		t.Run(command, func(t *testing.T) {
			done := make(chan struct{})
			var (
				res *engine.Result
				err error
			)
			start := time.Now()
			go func() {
				defer close(done)
				res, err = eng.ExecuteWithLimits(context.Background(),
					tenantRequest(upstream.URL, "tok", command), engine.Limits{MaxDuration: budget})
			}()

			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatalf("%q was still running 10s after a %s budget: the deadline does not reach its request", command, budget)
			}

			elapsed := time.Since(start)
			require.Less(t, elapsed, 5*time.Second, "the command outlived its budget by far")
			require.Nil(t, res)
			require.ErrorIs(t, err, context.DeadlineExceeded, "the deadline is reported as the context's error")
		})
	}
}
