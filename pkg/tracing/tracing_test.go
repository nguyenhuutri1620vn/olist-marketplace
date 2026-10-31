package tracing_test

import (
	"context"
	"slices"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/tracing"
)

func TestInitNamesTheServiceAndPropagatesTraceContext(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1") // unreachable is fine: export is async
	spans := tracetest.NewInMemoryExporter()

	shutdown, err := tracing.Init(context.Background(), "catalog-service", sdktrace.WithSyncer(spans))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shutdown(context.Background()) })

	_, span := otel.Tracer("test").Start(context.Background(), "op")
	span.End()

	got := spans.GetSpans()
	if len(got) != 1 {
		t.Fatalf("exported %d spans, want 1", len(got))
	}
	name, _ := got[0].Resource.Set().Value(semconv.ServiceNameKey)
	if name.AsString() != "catalog-service" {
		t.Errorf("service.name = %q, want catalog-service", name.AsString())
	}
	if !slices.Contains(otel.GetTextMapPropagator().Fields(), "traceparent") {
		t.Errorf("propagator fields = %v, want traceparent", otel.GetTextMapPropagator().Fields())
	}
}
