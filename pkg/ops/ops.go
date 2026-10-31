// Package ops serves a service's operational endpoints on its ops port:
// /healthz (process alive), /readyz (dependencies reachable) and /metrics
// (Prometheus). Ops handlers are deliberately not traced.
package ops

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// checkTimeout bounds each readiness check.
const checkTimeout = 2 * time.Second

// Check is one readiness dependency, such as the database or Kafka.
type Check struct {
	Name string
	Fn   func(context.Context) error
}

// NewRegistry returns a registry with Go runtime and process collectors.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return reg
}

// Handler serves /healthz, /readyz and /metrics.
func Handler(reg *prometheus.Registry, checks ...Check) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		// Checks run concurrently so the response stays within one checkTimeout.
		errs := make([]error, len(checks))
		var wg sync.WaitGroup
		for i, c := range checks {
			wg.Go(func() {
				ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
				defer cancel()
				errs[i] = c.Fn(ctx)
			})
		}
		wg.Wait()
		var failed []string
		for i, err := range errs {
			if err != nil {
				slog.WarnContext(r.Context(), "readiness check failed", "check", checks[i].Name, "err", err)
				failed = append(failed, checks[i].Name)
			}
		}
		if len(failed) > 0 {
			http.Error(w, "not ready: "+strings.Join(failed, ", "), http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ready")
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	return mux
}

// HTTPMetrics returns middleware that counts and times requests by method,
// matched ServeMux route pattern and status code.
func HTTPMetrics(reg prometheus.Registerer) func(http.Handler) http.Handler {
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests handled, by method, route pattern and status code.",
	}, []string{"method", "route", "code"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency, by method and route pattern.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})
	reg.MustRegister(requests, duration)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(rec, r)
			// ServeMux records the matched pattern on r; unmatched paths share one label.
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			requests.WithLabelValues(r.Method, route, strconv.Itoa(rec.code)).Inc()
			duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Probe returns nil when url answers 200. The `healthcheck` subcommand of each
// service binary uses it, since distroless images have no curl.
func Probe(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return nil
}
