// Package tracing sets up the OpenTelemetry SDK for a service: spans are
// exported over OTLP/HTTP to the endpoint in OTEL_EXPORTER_OTLP_ENDPOINT
// (Jaeger) and W3C trace context is propagated.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// Init installs a global tracer provider named after service and returns a
// function that flushes and stops it. Extra options are appended (tests use
// them to add an in-memory exporter).
func Init(ctx context.Context, service string, opts ...sdktrace.TracerProviderOption) (func(context.Context) error, error) {
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}
	res := resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(service))
	tp := sdktrace.NewTracerProvider(append([]sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	}, opts...)...)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
}
