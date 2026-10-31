package grpcx_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
)

// start serves a grpcx server on an in-memory listener and returns a client
// connection built with grpcx.Dial.
func start(t *testing.T, reg prometheus.Registerer) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpcx.NewServer(reg)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	conn, err := grpcx.Dial("passthrough:///bufnet", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestServerReportsServingHealth(t *testing.T) {
	conn := start(t, prometheus.NewRegistry())

	resp, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("health = %v, want SERVING", resp.GetStatus())
	}
}

func TestServerCountsAndTimesCallsByMethodAndCode(t *testing.T) {
	reg := prometheus.NewRegistry()
	conn := start(t, reg)

	for range 2 {
		if _, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{}); err != nil {
			t.Fatal(err)
		}
	}

	want := `
# HELP grpc_server_handled_total gRPC calls handled, by full method and status code.
# TYPE grpc_server_handled_total counter
grpc_server_handled_total{code="OK",method="/grpc.health.v1.Health/Check"} 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "grpc_server_handled_total"); err != nil {
		t.Error(err)
	}
	if n, _ := testutil.GatherAndCount(reg, "grpc_server_handling_seconds"); n == 0 {
		t.Error("grpc_server_handling_seconds not recorded")
	}
}

func TestClientAndServerSpansShareOneTrace(t *testing.T) {
	spans := tracetest.NewInMemoryExporter()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans)))
	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		otel.SetTextMapPropagator(prevProp)
	})
	conn := start(t, prometheus.NewRegistry())

	if _, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}

	// The server span can end just after the client sees the response.
	for deadline := time.Now().Add(2 * time.Second); len(spans.GetSpans()) < 2 && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	var client, server *tracetest.SpanStub
	for i, s := range spans.GetSpans() {
		switch s.SpanKind {
		case trace.SpanKindClient:
			client = &spans.GetSpans()[i]
		case trace.SpanKindServer:
			server = &spans.GetSpans()[i]
		}
	}
	if client == nil || server == nil {
		t.Fatalf("spans = %d, want a client and a server span", len(spans.GetSpans()))
	}
	if server.Parent.SpanID() != client.SpanContext.SpanID() || server.SpanContext.TraceID() != client.SpanContext.TraceID() {
		t.Errorf("server span is not a child of the client span in the same trace")
	}
}

func TestServerCountsStreamingCalls(t *testing.T) {
	reg := prometheus.NewRegistry()
	conn := start(t, reg)
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := healthpb.NewHealthClient(conn).Watch(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	cancel()

	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if n, _ := testutil.GatherAndCount(reg, "grpc_server_handled_total"); n > 0 {
			return
		}
	}
	t.Error("streaming Health/Watch call not counted in grpc_server_handled_total")
}
