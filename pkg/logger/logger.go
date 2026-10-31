// Package logger builds the slog JSON logger every service uses. Log lines
// written with a context that holds a span carry its trace and span ids.
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// New returns a JSON logger to stdout tagged with the service name and makes
// it the slog default.
func New(service string) *slog.Logger {
	l := NewWriter(os.Stdout, service)
	slog.SetDefault(l)
	return l
}

// NewWriter is New writing to w, without changing the slog default.
func NewWriter(w io.Writer, service string) *slog.Logger {
	return slog.New(traceHandler{slog.NewJSONHandler(w, nil)}).With("service", service)
}

type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}
