// Package grpcx builds gRPC servers and clients the same way in every
// service: OpenTelemetry tracing, Prometheus metrics and grpc.health.v1.
package grpcx

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// NewServer returns a traced, metered gRPC server with the standard health
// service registered and reporting SERVING. Metrics are registered on reg.
func NewServer(reg prometheus.Registerer, opts ...grpc.ServerOption) *grpc.Server {
	handled := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_handled_total",
		Help: "gRPC calls handled, by full method and status code.",
	}, []string{"method", "code"})
	latency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "grpc_server_handling_seconds",
		Help:    "gRPC call latency, by full method.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})
	reg.MustRegister(handled, latency)

	observe := func(method string, start time.Time, err error) {
		handled.WithLabelValues(method, status.Code(err).String()).Inc()
		latency.WithLabelValues(method).Observe(time.Since(start).Seconds())
	}
	unary := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := next(ctx, req)
		observe(info.FullMethod, start, err)
		return resp, err
	}
	stream := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
		start := time.Now()
		err := next(srv, ss)
		observe(info.FullMethod, start, err)
		return err
	}

	srv := grpc.NewServer(append([]grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(unary),
		grpc.ChainStreamInterceptor(stream),
	}, opts...)...)
	healthpb.RegisterHealthServer(srv, health.NewServer())
	return srv
}

// Dial returns a traced client connection to target. Connections stay inside
// the compose network, so transport security is off.
func Dial(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return grpc.NewClient(target, append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}, opts...)...)
}
