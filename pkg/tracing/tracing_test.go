package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// Tests set global OTel state via Init — do not run in parallel.

func TestInit_NoParent(t *testing.T) {
	t.Setenv("TRACEPARENT", "")
	t.Setenv("TRACESTATE", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	ctx, shutdown, err := Init(context.Background(), "dtctl test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer shutdown(context.Background())

	span := trace.SpanFromContext(ctx)
	sc := span.SpanContext()
	if !sc.IsValid() {
		t.Error("expected a valid span context")
	}
	if !sc.TraceID().IsValid() {
		t.Error("trace ID must not be zero")
	}
	if !sc.SpanID().IsValid() {
		t.Error("span ID must not be zero")
	}
	if !sc.IsSampled() {
		t.Error("span must be sampled (AlwaysSample)")
	}
}

func TestInit_InheritsTraceID(t *testing.T) {
	t.Setenv("TRACEPARENT", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	t.Setenv("TRACESTATE", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	ctx, shutdown, err := Init(context.Background(), "dtctl test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer shutdown(context.Background())

	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		t.Fatal("expected valid span context")
	}

	want := "4bf92f3577b34da6a3ce929d0e0e4736"
	if got := sc.TraceID().String(); got != want {
		t.Errorf("TraceID = %q, want %q (inherited from TRACEPARENT)", got, want)
	}
	// This span is a child — its span ID must differ from the parent's.
	if sc.SpanID().String() == "00f067aa0ba902b7" {
		t.Error("SpanID must be a new child span, not the parent's")
	}
}

func TestInit_InvalidParent_Graceful(t *testing.T) {
	// The W3C propagator ignores invalid traceparent values per spec.
	t.Setenv("TRACEPARENT", "not-a-valid-value")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	ctx, shutdown, err := Init(context.Background(), "dtctl test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer shutdown(context.Background())

	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		t.Error("expected valid span context after graceful fallback for invalid TRACEPARENT")
	}
}

func TestInit_TraceStatePropagated(t *testing.T) {
	t.Setenv("TRACEPARENT", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	t.Setenv("TRACESTATE", "vendor=value")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	ctx, shutdown, err := Init(context.Background(), "dtctl test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer shutdown(context.Background())

	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.TraceState().Len() == 0 {
		t.Error("expected TraceState to be propagated from TRACESTATE env var")
	}
}

func TestInit_NoOTLPEndpoint_NoError(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	_, shutdown, err := Init(context.Background(), "dtctl test")
	defer shutdown(context.Background())
	if err != nil {
		t.Errorf("unexpected error when OTLP endpoint is not set: %v", err)
	}
}
