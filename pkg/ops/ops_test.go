package ops_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/ops"
)

func ok(context.Context) error { return nil }

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestHealthzIsOKWhileChecksFail(t *testing.T) {
	h := ops.Handler(ops.NewRegistry(), ops.Check{Name: "db", Fn: func(context.Context) error { return errors.New("down") }})

	if code, _ := get(t, h, "/healthz"); code != http.StatusOK {
		t.Errorf("/healthz = %d, want 200", code)
	}
}

func TestReadyzIsOKWhenAllChecksPass(t *testing.T) {
	h := ops.Handler(ops.NewRegistry(), ops.Check{Name: "db", Fn: ok}, ops.Check{Name: "kafka", Fn: ok})

	if code, body := get(t, h, "/readyz"); code != http.StatusOK {
		t.Errorf("/readyz = %d %q, want 200", code, body)
	}
}

func TestReadyzNamesEveryFailingCheck(t *testing.T) {
	fail := func(context.Context) error { return errors.New("unreachable") }
	h := ops.Handler(ops.NewRegistry(), ops.Check{Name: "db", Fn: fail}, ops.Check{Name: "kafka", Fn: fail}, ops.Check{Name: "fine", Fn: ok})

	code, body := get(t, h, "/readyz")
	if code != http.StatusServiceUnavailable {
		t.Errorf("/readyz = %d, want 503", code)
	}
	if !strings.Contains(body, "db") || !strings.Contains(body, "kafka") || strings.Contains(body, "fine") {
		t.Errorf("/readyz body = %q, want db and kafka named, fine not", body)
	}
}

func TestReadyzBoundsEachCheckByTwoSeconds(t *testing.T) {
	var deadline time.Time
	h := ops.Handler(ops.NewRegistry(), ops.Check{Name: "db", Fn: func(ctx context.Context) error {
		deadline, _ = ctx.Deadline()
		return nil
	}})
	start := time.Now()

	get(t, h, "/readyz")

	if deadline.IsZero() || deadline.Sub(start) > 2*time.Second+100*time.Millisecond {
		t.Errorf("check deadline = %v after start, want ≤ 2s", deadline.Sub(start))
	}
}

func TestMetricsCountsAndTimesHTTPRequestsByRoute(t *testing.T) {
	reg := ops.NewRegistry()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	api := ops.HTTPMetrics(reg)(mux)

	get(t, api, "/items/1")
	get(t, api, "/items/2")

	want := `
# HELP http_requests_total HTTP requests handled, by method, route pattern and status code.
# TYPE http_requests_total counter
http_requests_total{code="418",method="GET",route="GET /items/{id}"} 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "http_requests_total"); err != nil {
		t.Error(err)
	}
	_, body := get(t, ops.Handler(reg), "/metrics")
	for _, want := range []string{"http_requests_total", "http_request_duration_seconds_bucket", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics lacks %s", want)
		}
	}
}

func TestProbeReportsReadiness(t *testing.T) {
	ready := httptest.NewServer(ops.Handler(ops.NewRegistry(), ops.Check{Name: "db", Fn: ok}))
	defer ready.Close()
	notReady := httptest.NewServer(ops.Handler(ops.NewRegistry(), ops.Check{Name: "db", Fn: func(context.Context) error { return io.EOF }}))
	defer notReady.Close()

	if err := ops.Probe(context.Background(), ready.URL+"/readyz"); err != nil {
		t.Errorf("Probe(ready) = %v, want nil", err)
	}
	if err := ops.Probe(context.Background(), notReady.URL+"/readyz"); err == nil {
		t.Error("Probe(not ready) = nil, want error")
	}
}

func TestReadyzRunsChecksConcurrently(t *testing.T) {
	slow := func(context.Context) error { time.Sleep(time.Second); return nil }
	h := ops.Handler(ops.NewRegistry(), ops.Check{Name: "db", Fn: slow}, ops.Check{Name: "kafka", Fn: slow}, ops.Check{Name: "x", Fn: slow})
	start := time.Now()

	get(t, h, "/readyz")

	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("/readyz took %v with three 1s checks, want them run concurrently", d)
	}
}
