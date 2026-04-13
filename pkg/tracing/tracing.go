// Package tracing configures OpenTelemetry for dtctl using the OTLP HTTP exporter.
//
// OneAgent does not reliably instrument short-lived processes; dtctl therefore uses the
// OpenTelemetry SDK directly so spans are exported before the process exits.
//
// # Span lifecycle
//
// Init creates a single root span covering the entire CLI invocation and registers a
// BatchSpanProcessor backed by an OTLP HTTP exporter (when configured). The caller
// MUST defer the returned shutdown function so spans are force-flushed before exit.
//
// # W3C Trace Context propagation
//
// TRACEPARENT and TRACESTATE environment variables are read on startup. When a valid
// TRACEPARENT is present, the CLI span becomes a child of the caller's span, allowing
// dtctl to participate in an existing distributed trace (e.g. from a CI pipeline or
// orchestrator). Invalid values are silently ignored per the W3C spec: a new root span
// is created instead.
//
// # OTLP export
//
// An OTLP HTTP exporter is initialised only when OTEL_EXPORTER_OTLP_ENDPOINT is set.
// All standard OTEL_* env vars (OTEL_EXPORTER_OTLP_HEADERS, OTEL_SERVICE_NAME, etc.)
// are forwarded to the SDK automatically. When no endpoint is configured the SDK still
// generates valid span contexts that are propagated to Dynatrace API requests via the
// traceparent header, but no spans are exported.
package tracing

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/dynatrace-oss/dtctl/pkg/version"
)

const tracerName = "dtctl"

// Init initialises OpenTelemetry for the current dtctl invocation and returns:
//   - a context carrying the root CLI span (use as parent for all further work)
//   - a shutdown function that MUST be deferred to flush spans before process exit
//   - a non-nil error only when OTEL_EXPORTER_OTLP_ENDPOINT is set but the exporter
//     cannot be created; the returned context and span are always valid
func Init(ctx context.Context, spanName string) (context.Context, func(context.Context), error) {
	svcName := os.Getenv("OTEL_SERVICE_NAME")
	if svcName == "" {
		svcName = tracerName
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(svcName),
			semconv.ServiceVersion(version.Version),
		),
	)
	if err != nil {
		res = resource.Default()
	}

	// Route SDK-internal errors (e.g. failed span exports) to stderr so they
	// are visible when debugging. Only active when OTEL_EXPORTER_OTLP_ENDPOINT
	// is set and the BatchSpanProcessor encounters a delivery failure.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		fmt.Fprintf(os.Stderr, "dtctl: otel: %v\n", err)
	}))

	// W3C TraceContext + Baggage propagators — registered globally so the OTel
	// HTTP instrumentation middleware and manual carrier injections both work.
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	}

	// Set up OTLP HTTP exporter only when the endpoint is explicitly configured.
	// All OTEL_EXPORTER_OTLP_* env vars are picked up by the SDK automatically.
	var exportErr error
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		exp, expErr := otlptracehttp.New(ctx)
		if expErr != nil {
			exportErr = fmt.Errorf("OTLP exporter: %w", expErr)
		} else {
			// BatchSpanProcessor is async — low overhead during CLI execution;
			// ForceFlush in shutdown ensures delivery before process exit.
			tpOpts = append(tpOpts, sdktrace.WithBatcher(exp))
		}
	}

	tp := sdktrace.NewTracerProvider(tpOpts...)
	otel.SetTracerProvider(tp)

	// Inherit caller's trace context from TRACEPARENT / TRACESTATE env vars.
	// The W3C propagator silently ignores invalid values → new root span.
	parentCtx := prop.Extract(ctx, envCarrier{})

	// Root span covers the entire CLI invocation.
	spanCtx, span := tp.Tracer(tracerName).Start(parentCtx, spanName)

	shutdown := func(ctx context.Context) {
		span.End()
		if err := tp.ForceFlush(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "dtctl: otel: flush: %v\n", err)
		}
		if err := tp.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "dtctl: otel: shutdown: %v\n", err)
		}
	}

	return spanCtx, shutdown, exportErr
}

// envCarrier implements propagation.TextMapCarrier backed by environment variables,
// mapping the canonical W3C header names to TRACEPARENT / TRACESTATE.
type envCarrier struct{}

func (envCarrier) Get(key string) string {
	switch strings.ToLower(key) {
	case "traceparent":
		return os.Getenv("TRACEPARENT")
	case "tracestate":
		return os.Getenv("TRACESTATE")
	}
	return ""
}

func (envCarrier) Set(string, string) {}
func (envCarrier) Keys() []string     { return []string{"traceparent", "tracestate"} }
