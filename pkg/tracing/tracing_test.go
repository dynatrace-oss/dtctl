package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestInit(t *testing.T) {
	// Save and restore the global TracerProvider so this test doesn't leak state.
	origTP := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(origTP) })

	// Use a non-routable endpoint so the exporter is created but never sends data.
	shutdown, err := Init(context.Background(), "localhost:0", "test-token")
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init() returned nil shutdown function")
	}

	// Verify that the global TracerProvider was set to an SDK provider
	tp := otel.GetTracerProvider()
	if _, ok := tp.(*sdktrace.TracerProvider); !ok {
		t.Fatalf("expected *sdktrace.TracerProvider, got %T", tp)
	}

	// Shutdown should complete without error
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
}
