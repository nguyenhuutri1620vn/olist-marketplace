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
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/bootstrap"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/inbox"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/logger"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/ops"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/outbox"
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
	// Outbox runs the outbox relay (AD-6) on the service's database and
	// exposes outbox_unsent_rows. It needs Migrations (with the outbox
	// tables) and UsesKafka; OUTBOX_POLL_INTERVAL sets the poll (≤ 200ms).
	Outbox bool
	// Consumers run idempotent Kafka consumers (AD-7) on the service's
	// database, with inbox metrics and DLQ depth. They need Migrations (with
	// processed_events) and UsesKafka; INBOX_MAX_ATTEMPTS bounds retries.
	Consumers []*inbox.Consumer
	// GRPC serves a gRPC server with health on GRPC_ADDR (default :9090).
	GRPC bool
	// Init, when set, runs once before RegisterGRPC and API, so both can share
	// what it builds. pool is nil without Migrations. An error stops startup.
	Init func(ctx context.Context, pool *pgxpool.Pool) error
	// Policy says who may call each RPC on the gRPC server; tokens are verified
	// against JWKS_URL (default identity's JWKS). RPCs it does not declare are
	// refused, so every service must list its public and role-gated RPCs.
	Policy auth.Policy
	// RegisterGRPC, when set with GRPC, registers the service's handlers. pool
	// is the database pool, nil without Migrations.
	RegisterGRPC func(s *grpc.Server, pool *pgxpool.Pool)
	// API, when set, builds the HTTP API served on HTTP_ADDR (default :8080)
	// and a function that releases its resources at shutdown.
	API func() (http.Handler, func(), error)
}

const shutdownTimeout = 10 * time.Second

// dlqDepthInterval is how often inbox_dlq_depth is refreshed.
const dlqDepthInterval = 15 * time.Second

// DefaultJWKSURL is identity-service's internal JWKS endpoint.
const DefaultJWKSURL = "http://identity-service:8080/.well-known/jwks.json"

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
	var pollInterval time.Duration
	if s.Outbox {
		if s.Migrations == nil || !s.UsesKafka {
			return errors.New("Outbox needs Migrations and UsesKafka")
		}
		var err error
		if pollInterval, err = outbox.PollInterval(); err != nil {
			return err
		}
	}
	maxAttempts := 0
	if len(s.Consumers) > 0 {
		if s.Migrations == nil || !s.UsesKafka {
			return errors.New("Consumers need Migrations and UsesKafka")
		}
		groups := map[string]bool{}
		for _, c := range s.Consumers {
			if c.Group() == "" || len(c.Topics()) == 0 || groups[c.Group()] {
				return fmt.Errorf("consumer %q needs a group of its own and at least one topic", c.Group())
			}
			groups[c.Group()] = true
		}
		var err error
		if maxAttempts, err = inbox.MaxAttempts(); err != nil {
			return err
		}
	}
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

	if s.Init != nil {
		if err := s.Init(ctx, pool); err != nil {
			return fmt.Errorf("init: %w", err)
		}
	}

	reg := ops.NewRegistry()
	var relay *outbox.Relay
	if s.Outbox {
		brokers, err := bootstrap.KafkaBrokers()
		if err != nil {
			return err
		}
		producer, err := eventbus.NewProducer(brokers)
		if err != nil {
			return err
		}
		defer producer.Close()
		relay = outbox.NewRelay(pool, producer, log, pollInterval)
		reg.MustRegister(relay.Collector())
	}
	var consumerDeps inbox.Deps
	if len(s.Consumers) > 0 {
		brokers, err := bootstrap.KafkaBrokers()
		if err != nil {
			return err
		}
		metrics := inbox.NewMetrics()
		reg.MustRegister(metrics.Collectors()...)
		consumerDeps = inbox.Deps{Pool: pool, Brokers: brokers, Log: log, Metrics: metrics, MaxAttempts: maxAttempts}
	}
	var servers []server
	servers = append(servers, httpServer("ops", addr("OPS_ADDR", ":8090"), ops.Handler(reg, checks...)))
	if s.GRPC {
		verifier := auth.NewVerifier(auth.NewKeySet(addr("JWKS_URL", DefaultJWKSURL)))
		gs := grpcx.NewServer(reg,
			grpc.ChainUnaryInterceptor(s.Policy.UnaryInterceptor(verifier)),
			grpc.ChainStreamInterceptor(s.Policy.StreamInterceptor(verifier)))
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

	// The relay and the consumers outlive the servers' drain, so outbox rows
	// committed by in-flight requests are still published before exit.
	background, stopBackground := context.WithCancel(context.WithoutCancel(ctx))
	var workers sync.WaitGroup
	if relay != nil {
		workers.Go(func() { relay.Run(background) })
	}
	var topics []string
	for _, c := range s.Consumers {
		topics = append(topics, c.Topics()...)
		workers.Go(func() {
			if err := c.Run(background, consumerDeps); err != nil {
				log.Error("consumer stopped", "consumer", c.Group(), "err", err)
			}
		})
	}
	if len(topics) > 0 {
		workers.Go(func() {
			inbox.WatchDLQDepth(background, consumerDeps.Brokers, topics, consumerDeps.Metrics, log, dlqDepthInterval)
		})
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
	stopBackground()
	workers.Wait()
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
