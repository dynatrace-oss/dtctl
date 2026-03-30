package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/dynatrace-oss/dtctl/pkg/version"
)

// Init initializes OpenTelemetry tracing with a Dynatrace OTLP HTTP exporter.
// The endpoint should be the Dynatrace environment URL (e.g. "https://abc12345.live.dynatrace.com").
// The token is the API token with openTelemetryTrace.ingest scope.
// Returns a shutdown function that flushes pending spans; callers must call it before exit.
func Init(ctx context.Context, endpoint, token string) (shutdown func(context.Context) error, err error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("dtctl"),
			semconv.ServiceVersionKey.String(version.Version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating tracing resource: %w", err)
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithURLPath("/api/v2/otlp/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Api-Token " + token,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}
