package handler_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/api-gateway/internal/handler"
)

func healthyConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpcx.NewServer(prometheus.NewRegistry())
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	return dial(t, func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) })
}

func deadConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	return dial(t, func(context.Context, string) (net.Conn, error) { return nil, net.ErrClosed })
}

func dial(t *testing.T, d func(context.Context, string) (net.Conn, error)) *grpc.ClientConn {
	t.Helper()
	conn, err := grpcx.Dial("passthrough:///svc", grpc.WithContextDialer(d))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func serve(t *testing.T, conns map[string]grpc.ClientConnInterface, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	handler.RegisterGRPCHealth(mux, conns)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestGRPCHealthReportsServingService(t *testing.T) {
	rec, body := serve(t, map[string]grpc.ClientConnInterface{"catalog-service": healthyConn(t)}, "/debug/grpc-health/catalog-service")

	if rec.Code != http.StatusOK || body["status"] != "SERVING" || body["service"] != "catalog-service" {
		t.Errorf("response = %d %v, want 200 SERVING for catalog-service", rec.Code, body)
	}
}

func TestGRPCHealthUnknownServiceIs404(t *testing.T) {
	rec, body := serve(t, map[string]grpc.ClientConnInterface{}, "/debug/grpc-health/nope")

	if errBody, _ := body["error"].(map[string]any); rec.Code != http.StatusNotFound || errBody["code"] != "NOT_FOUND" {
		t.Errorf("response = %d %v, want 404 with error code NOT_FOUND", rec.Code, body)
	}
}

func TestGRPCHealthUnreachableServiceIs502(t *testing.T) {
	rec, body := serve(t, map[string]grpc.ClientConnInterface{"order-service": deadConn(t)}, "/debug/grpc-health/order-service")

	errBody, _ := body["error"].(map[string]any)
	if rec.Code != http.StatusBadGateway || errBody["code"] != "UNAVAILABLE" || errBody["message"] == "" {
		t.Errorf("response = %d %v, want 502 with error code UNAVAILABLE", rec.Code, body)
	}
}

func TestGRPCHealthNotServingServiceIs503(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	healthpb.RegisterHealthServer(srv, hs)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn := dial(t, func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) })

	rec, body := serve(t, map[string]grpc.ClientConnInterface{"payment-service": conn}, "/debug/grpc-health/payment-service")

	if rec.Code != http.StatusServiceUnavailable || body["status"] != "NOT_SERVING" {
		t.Errorf("response = %d %v, want 503 NOT_SERVING", rec.Code, body)
	}
}
