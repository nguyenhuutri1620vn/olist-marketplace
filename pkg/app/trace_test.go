package app_test

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	collectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/proto"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
)

// otlpReceiver is a fake OTLP/HTTP endpoint that keeps every exported span.
type otlpReceiver struct {
	mu    sync.Mutex
	spans []*tracepb.Span
}

func (o *otlpReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req collectorpb.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &req); err == nil {
		o.mu.Lock()
		for _, rs := range req.GetResourceSpans() {
			for _, ss := range rs.GetScopeSpans() {
				o.spans = append(o.spans, ss.GetSpans()...)
			}
		}
		o.mu.Unlock()
	}
	w.WriteHeader(http.StatusOK)
}

func TestAPIRequestTracesIntoGRPCCallAndIsMetered(t *testing.T) {
	recv := &otlpReceiver{}
	collector := httptest.NewServer(recv)
	defer collector.Close()

	upstream := grpcx.NewServer(prometheus.NewRegistry())
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	lis := l.Addr().String()
	go upstream.Serve(l)
	defer upstream.Stop()

	opsAddr, apiAddr := freeAddr(t), freeAddr(t)
	t.Setenv("OPS_ADDR", opsAddr)
	t.Setenv("HTTP_ADDR", apiAddr)
	api := func() (http.Handler, func(), error) {
		conn, err := grpcx.Dial(lis)
		if err != nil {
			return nil, nil, err
		}
		mux := http.NewServeMux()
		mux.HandleFunc("GET /check/{name}", func(w http.ResponseWriter, r *http.Request) {
			if _, err := healthpb.NewHealthClient(conn).Check(r.Context(), &healthpb.HealthCheckRequest{}); err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
			}
		})
		return mux, func() { conn.Close() }, nil
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	stop := run(t, app.Service{Name: "api-gateway", API: api})
	waitReady(t)

	resp, err := http.Get("http://" + apiAddr + "/check/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /check/x = %d, want 200", resp.StatusCode)
	}
	metrics, err := http.Get("http://" + opsAddr + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(metrics.Body)
	metrics.Body.Close()
	if !strings.Contains(string(body), `http_requests_total{code="200",method="GET",route="GET /check/{name}"} 1`) {
		t.Error("ops /metrics lacks the API request count for GET /check/{name}")
	}
	if err := stop(); err != nil { // flushes spans to the receiver
		t.Fatal(err)
	}

	recv.mu.Lock()
	defer recv.mu.Unlock()
	var server, client *tracepb.Span
	for _, s := range recv.spans {
		switch s.GetKind() {
		case tracepb.Span_SPAN_KIND_SERVER:
			server = s
		case tracepb.Span_SPAN_KIND_CLIENT:
			client = s
		}
	}
	if server == nil || client == nil {
		t.Fatalf("exported %d spans, want an HTTP server span and a gRPC client span", len(recv.spans))
	}
	if server.GetName() != "GET /check/{name}" {
		t.Errorf("server span name = %q, want the route pattern", server.GetName())
	}
	if string(client.GetParentSpanId()) != string(server.GetSpanId()) || string(client.GetTraceId()) != string(server.GetTraceId()) {
		t.Error("gRPC client span is not a child of the HTTP server span in the same trace")
	}
}
