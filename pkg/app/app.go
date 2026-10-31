// Package app runs a service with the shared observability baseline: slog
// JSON logs, OpenTelemetry tracing, an ops port (/healthz, /readyz, /metrics),
// migrations and readiness checks, an optional gRPC server and an optional
// HTTP API. Every service's main calls Main.
package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/bootstrap"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/logger"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/ops"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/tracing"
)

// Service describes what a service runs besides the ops port.
type Service struct {
	Name string
	// Migrations, when set, are applied to DATABASE_URL at startup and the
	// database becomes a readiness check.
	Migrations fs.FS
	// UsesKafka adds a readiness check for KAFKA_BROKERS.
	UsesKafka bool
	// GRPC serves a gRPC server with health on GRPC_ADDR (default :9090).
	GRPC bool
	// RegisterGRPC, when set with GRPC, registers the service's handlers. pool
	// is the database pool, nil without Migrations.
	RegisterGRPC func(s *grpc.Server, pool *pgxpool.Pool)
	// API, when set, builds the HTTP API served on HTTP_ADDR (default :8080)
	// and a function that releases its resources at shutdown.
	API func() (http.Handler, func(), error)
}

const shutdownTimeout = 10 * time.Second

func addr(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return fallback
}

// Main runs s until SIGINT/SIGTERM. Invoked as `<binary> healthcheck`, it
// instead probes the running service's /readyz and exits 0 or 1.
func Main(s Service) {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := Healthcheck(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "not ready:", err)
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := Run(ctx, s); err != nil {
		slog.Error("service stopped", "err", err)
		os.Exit(1)
	}
}

// Healthcheck probes /readyz on this process's ops address (OPS_ADDR).
func Healthcheck(ctx context.Context) error {
	host, port, err := net.SplitHostPort(addr("OPS_ADDR", ":8090"))
	if err != nil {
		return err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return ops.Probe(ctx, "http://"+net.JoinHostPort(host, port)+"/readyz")
}

// Run starts s and blocks until ctx is done, then shuts down gracefully.
func Run(ctx context.Context, s Service) error {
	log := logger.New(s.Name)
	shutdownTracing, err := tracing.Init(ctx, s.Name)
	if err != nil {
		return err
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := shutdownTracing(sctx); err != nil {
			log.Warn("tracing shutdown", "err", err)
		}
	}()

	var checks []ops.Check
	var pool *pgxpool.Pool
	if s.Migrations != nil {
		if err := bootstrap.Migrate(ctx, s.Migrations); err != nil {
			return err
		}
		log.InfoContext(ctx, "migrations applied")
		pool, err = bootstrap.OpenDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()
		checks = append(checks, bootstrap.DBCheck(pool))
	}
	if s.UsesKafka {
		check, closeKafka, err := bootstrap.KafkaCheck()
		if err != nil {
			return err
		}
		defer closeKafka()
		checks = append(checks, check)
	}

	reg := ops.NewRegistry()
	var servers []server
	servers = append(servers, httpServer("ops", addr("OPS_ADDR", ":8090"), ops.Handler(reg, checks...)))
	if s.GRPC {
		gs := grpcx.NewServer(reg)
		if s.RegisterGRPC != nil {
			s.RegisterGRPC(gs, pool)
		}
		servers = append(servers, grpcServer{gs, addr("GRPC_ADDR", ":9090")})
	}
	if s.API != nil {
		h, closeAPI, err := s.API()
		if err != nil {
			return err
		}
		defer closeAPI()
		h = otelhttp.NewHandler(ops.HTTPMetrics(reg)(h), s.Name,
			otelhttp.WithSpanNameFormatter(spanName))
		servers = append(servers, httpServer("api", addr("HTTP_ADDR", ":8080"), h))
	}

	// Bind every port before serving any, so a failed bind leaves nothing running.
	listeners := make([]net.Listener, 0, len(servers))
	for _, srv := range servers {
		lis, err := net.Listen("tcp", srv.address())
		if err != nil {
			for _, l := range listeners {
				l.Close()
			}
			return fmt.Errorf("listen %s: %w", srv.address(), err)
		}
		listeners = append(listeners, lis)
	}
	errs := make(chan error, len(servers))
	for i, srv := range servers {
		log.InfoContext(ctx, "listening", "server", srv.name(), "addr", srv.address())
		go func() { errs <- srv.serve(listeners[i]) }()
	}

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err = <-errs:
		log.Error("server failed", "err", err)
	}
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	for _, srv := range servers {
		srv.shutdown(sctx)
	}
	return err
}

// spanName names an API span after its ServeMux route. otelhttp calls it before
// routing (no pattern yet) and again after, when r.Pattern is set.
func spanName(_ string, r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return r.Method + " " + r.URL.Path
}

type server interface {
	name() string
	address() string
	serve(net.Listener) error
	shutdown(context.Context)
}

type httpSrv struct {
	label string
	srv   *http.Server
}

func httpServer(label, address string, h http.Handler) server {
	return httpSrv{label, &http.Server{Addr: address, Handler: h, ReadHeaderTimeout: 5 * time.Second}}
}

func (h httpSrv) name() string    { return h.label }
func (h httpSrv) address() string { return h.srv.Addr }
func (h httpSrv) serve(l net.Listener) error {
	if err := h.srv.Serve(l); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%s server: %w", h.label, err)
	}
	return nil
}
func (h httpSrv) shutdown(ctx context.Context) {
	if err := h.srv.Shutdown(ctx); err != nil {
		slog.Warn("shutdown", "server", h.label, "err", err)
	}
}

type grpcServer struct {
	srv interface {
		Serve(net.Listener) error
		GracefulStop()
		Stop()
	}
	addr string
}

func (g grpcServer) name() string               { return "grpc" }
func (g grpcServer) address() string            { return g.addr }
func (g grpcServer) serve(l net.Listener) error { return g.srv.Serve(l) }
func (g grpcServer) shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() { g.srv.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		g.srv.Stop()
	}
}
