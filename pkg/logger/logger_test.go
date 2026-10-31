package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/logger"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line is not JSON: %v: %q", err, buf.String())
	}
	return line
}

func TestLogInsideSpanCarriesTraceAndSpanID(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWriter(&buf, "catalog-service")
	ctx, span := sdktrace.NewTracerProvider().Tracer("test").Start(context.Background(), "op")
	defer span.End()

	log.InfoContext(ctx, "hello")

	line := decode(t, &buf)
	sc := span.SpanContext()
	if line["trace_id"] != sc.TraceID().String() || line["span_id"] != sc.SpanID().String() {
		t.Errorf("trace fields = %v/%v, want %s/%s", line["trace_id"], line["span_id"], sc.TraceID(), sc.SpanID())
	}
	if line["service"] != "catalog-service" || line["msg"] != "hello" {
		t.Errorf("line = %v, want service and msg", line)
	}
}

func TestLogOutsideSpanHasNoTraceFields(t *testing.T) {
	var buf bytes.Buffer
	logger.NewWriter(&buf, "catalog-service").InfoContext(context.Background(), "hello")

	line := decode(t, &buf)
	if _, ok := line["trace_id"]; ok {
		t.Errorf("line without span has trace_id: %v", line)
	}
}

func TestTraceFieldsSurviveWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWriter(&buf, "svc").With("k", "v")
	ctx, span := sdktrace.NewTracerProvider().Tracer("test").Start(context.Background(), "op")
	defer span.End()

	log.InfoContext(ctx, "hello")

	if line := decode(t, &buf); line["trace_id"] != span.SpanContext().TraceID().String() || line["k"] != "v" {
		t.Errorf("line = %v, want trace_id and k=v", line)
	}
}
