package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestInjectTraceContext_PropagatesHeaders(t *testing.T) {
	// Set up OTel with a known TRACEPARENT so we can verify exact header values.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "")

	// Reset global OTel state after the test to avoid leaking into other tests.
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	})

	// Import the tracing package to initialise OTel with a known trace context.
	traceParent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	t.Setenv("TRACEPARENT", traceParent)
	t.Setenv("TRACESTATE", "vendor=test")

	// Manually set up a minimal OTel environment for the test.
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)

	// Extract the trace context from environment into a context.
	carrier := envMapCarrier{
		"traceparent": traceParent,
		"tracestate":  "vendor=test",
	}
	ctx := prop.Extract(context.Background(), carrier)

	// Capture HTTP headers from the request.
	var gotTraceparent, gotTracestate string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceparent = r.Header.Get("Traceparent")
		gotTracestate = r.Header.Get("Tracestate")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c, err := NewForTesting(server.URL, "test-token")
	if err != nil {
		t.Fatalf("NewForTesting() error = %v", err)
	}

	// Inject the trace context.
	InjectTraceContext(c, ctx)

	// Make a request.
	_, err = c.HTTP().R().Get("/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// Verify traceparent header is present and contains the expected trace ID.
	if gotTraceparent == "" {
		t.Fatal("expected traceparent header to be set, got empty")
	}
	if !strings.Contains(gotTraceparent, "4bf92f3577b34da6a3ce929d0e0e4736") {
		t.Errorf("traceparent = %q, want it to contain the inherited trace ID", gotTraceparent)
	}

	// Verify tracestate header is propagated.
	if !strings.Contains(gotTracestate, "vendor=test") {
		t.Errorf("tracestate = %q, want it to contain 'vendor=test'", gotTracestate)
	}
}

// envMapCarrier is a simple TextMapCarrier backed by a map, used in tests.
type envMapCarrier map[string]string

func (c envMapCarrier) Get(key string) string { return c[strings.ToLower(key)] }
func (c envMapCarrier) Set(key, value string) { c[strings.ToLower(key)] = value }
func (c envMapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
