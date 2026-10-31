package app_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// run starts app.Run in the background and returns a function that stops it
// and reports Run's result.
func run(t *testing.T, s app.Service) func() error {
	t.Helper()
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()
	t.Cleanup(cancel)
	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(10 * time.Second):
			return fmt.Errorf("Run did not return after cancel")
		}
	}
}

func waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if app.Healthcheck(context.Background()) == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("service never became ready")
}

func TestServiceWithAPIBecomesReadyAndStopsCleanly(t *testing.T) {
	t.Setenv("OPS_ADDR", freeAddr(t))
	api := freeAddr(t)
	t.Setenv("HTTP_ADDR", api)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {})
	stop := run(t, app.Service{Name: "api-gateway", API: func() (http.Handler, func(), error) { return mux, func() {}, nil }})

	waitReady(t)
	resp, err := http.Get("http://" + api + "/hello")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /hello = %d, want 200", resp.StatusCode)
	}
	if err := stop(); err != nil {
		t.Errorf("Run returned %v after cancel, want nil", err)
	}
}

func TestDataServiceMigratesServesGRPCAndBecomesReady(t *testing.T) {
	dsn := os.Getenv("DBMIGRATE_TEST_DSN")
	if dsn == "" {
		t.Skip("DBMIGRATE_TEST_DSN not set")
	}
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("OPS_ADDR", freeAddr(t))
	grpcAddr := freeAddr(t)
	t.Setenv("GRPC_ADDR", grpcAddr)
	stop := run(t, app.Service{Name: "catalog-service", Migrations: fstest.MapFS{}, GRPC: true})

	waitReady(t)
	conn, err := grpcx.Dial(grpcAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	resp, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("gRPC health = %v, %v; want SERVING", resp.GetStatus(), err)
	}
	if err := stop(); err != nil {
		t.Errorf("Run returned %v after cancel, want nil", err)
	}
}

func TestRunFailsWhenMigrationsCannotReachDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x:y@127.0.0.1:1/z?sslmode=disable")
	t.Setenv("OPS_ADDR", freeAddr(t))
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := app.Run(ctx, app.Service{Name: "catalog-service", Migrations: fstest.MapFS{}})
	if err == nil || !strings.Contains(err.Error(), "connect to database") {
		t.Errorf("Run with unreachable database = %v, want a connect error", err)
	}
}

func TestHealthcheckFailsWhenNothingListens(t *testing.T) {
	t.Setenv("OPS_ADDR", freeAddr(t))

	if err := app.Healthcheck(context.Background()); err == nil {
		t.Error("Healthcheck with no server = nil, want error")
	}
}
