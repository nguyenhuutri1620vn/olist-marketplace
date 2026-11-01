package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/outbox"
)

type probe struct {
	OrderID string `json:"order_id"`
}

func (probe) EventType() string     { return "ProbeRecorded" }
func (probe) EventVersion() string  { return "1.0" }
func (probe) Topic() string         { return eventsv1.TopicOrderEvents }
func (p probe) AggregateID() string { return p.OrderID }

func TestOutboxNeedsADatabaseAndKafka(t *testing.T) {
	t.Setenv("OPS_ADDR", freeAddr(t))
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	for name, s := range map[string]app.Service{
		"no database": {Name: "order-service", UsesKafka: true, Outbox: true},
		"no kafka":    {Name: "order-service", Migrations: fstest.MapFS{}, Outbox: true},
	} {
		err := app.Run(context.Background(), s)
		if err == nil || !strings.Contains(err.Error(), "Outbox") {
			t.Errorf("%s: Run = %v, want an Outbox config error", name, err)
		}
	}
}

func TestOutboxRejectsASlowPollInterval(t *testing.T) {
	t.Setenv("OPS_ADDR", freeAddr(t))
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("OUTBOX_POLL_INTERVAL", "1s")
	err := app.Run(context.Background(), app.Service{Name: "order-service", Migrations: fstest.MapFS{}, UsesKafka: true, Outbox: true})
	if err == nil || !strings.Contains(err.Error(), "OUTBOX_POLL_INTERVAL") {
		t.Errorf("Run = %v, want an OUTBOX_POLL_INTERVAL error", err)
	}
}

// A service with Outbox runs the relay: an event written at startup is
// published and outbox_unsent_rows drops to 0 on /metrics.
func TestOutboxServiceRunsTheRelay(t *testing.T) {
	dsn, brokers := os.Getenv("DBMIGRATE_TEST_DSN"), os.Getenv("OUTBOX_TEST_KAFKA")
	if dsn == "" || brokers == "" {
		t.Skip("DBMIGRATE_TEST_DSN or OUTBOX_TEST_KAFKA not set")
	}
	eventsv1.RegisterForTest(t, probe{})
	db := isolatedSchema(t, dsn)
	t.Setenv("DATABASE_URL", db)
	t.Setenv("KAFKA_BROKERS", brokers)
	t.Setenv("OUTBOX_POLL_INTERVAL", "20ms")
	ops := freeAddr(t)
	t.Setenv("OPS_ADDR", ops)
	stop := run(t, app.Service{
		Name:       "order-service",
		Migrations: fstest.MapFS{"000001_outbox.up.sql": {Data: []byte(outbox.SchemaSQL)}},
		UsesKafka:  true,
		Outbox:     true,
		Init: func(ctx context.Context, pool *pgxpool.Pool) error {
			return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
				return outbox.Write(ctx, tx, probe{OrderID: "0192f000-0000-7000-8000-0000000000bb"}, eventbus.Meta{OccurredAt: time.Now()})
			})
		},
	})
	waitReady(t)
	pool, err := pgxpool.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// The gauge starts at 0, so first wait for the row itself to be sent.
	deadline := time.Now().Add(10 * time.Second)
	sent := false
	for !sent && time.Now().Before(deadline) {
		pool.QueryRow(context.Background(), `SELECT count(*) = 1 FROM outbox WHERE sent_at IS NOT NULL`).Scan(&sent)
		time.Sleep(20 * time.Millisecond)
	}
	if !sent {
		t.Fatal("the relay never published the outbox row")
	}
	if got := metric(t, "http://"+ops+"/metrics", "outbox_unsent_rows"); got != "0" {
		t.Errorf("outbox_unsent_rows = %q, want 0", got)
	}
	if err := stop(); err != nil {
		t.Errorf("Run returned %v after cancel", err)
	}
}

// isolatedSchema creates a schema dropped after the test and returns dsn
// with it as search_path, so migrations here touch nothing else.
func isolatedSchema(t *testing.T, dsn string) string {
	t.Helper()
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("app_outbox_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

// metricsPage fetches a /metrics page ("" when unreachable).
func metricsPage(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// metric returns the value of an unlabelled metric on a /metrics page, or "".
func metric(t *testing.T, url, name string) string {
	t.Helper()
	for _, line := range strings.Split(metricsPage(t, url), "\n") {
		if v, ok := strings.CutPrefix(line, name+" "); ok {
			return v
		}
	}
	return ""
}

// A request still in flight at shutdown commits its outbox row during the
// drain; the relay outlives the drain and publishes it before Run returns.
func TestOutboxRowsCommittedDuringTheDrainArePublished(t *testing.T) {
	dsn, brokers := os.Getenv("DBMIGRATE_TEST_DSN"), os.Getenv("OUTBOX_TEST_KAFKA")
	if dsn == "" || brokers == "" {
		t.Skip("DBMIGRATE_TEST_DSN or OUTBOX_TEST_KAFKA not set")
	}
	eventsv1.RegisterForTest(t, probe{})
	db := isolatedSchema(t, dsn)
	t.Setenv("DATABASE_URL", db)
	t.Setenv("KAFKA_BROKERS", brokers)
	t.Setenv("OPS_ADDR", freeAddr(t))
	api := freeAddr(t)
	t.Setenv("HTTP_ADDR", api)
	started := make(chan struct{})
	stop := run(t, app.Service{
		Name:       "order-service",
		Migrations: fstest.MapFS{"000001_outbox.up.sql": {Data: []byte(outbox.SchemaSQL)}},
		UsesKafka:  true,
		Outbox:     true,
		API: func() (http.Handler, func(), error) {
			pool, err := pgxpool.New(context.Background(), db)
			if err != nil {
				return nil, nil, err
			}
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				time.Sleep(300 * time.Millisecond) // shutdown begins meanwhile
				err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
					return outbox.Write(context.Background(), tx, probe{OrderID: "0192f000-0000-7000-8000-0000000000cc"}, eventbus.Meta{OccurredAt: time.Now()})
				})
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
			})
			return h, pool.Close, nil
		},
	})
	waitReady(t)
	go http.Get("http://" + api + "/write")
	<-started
	if err := stop(); err != nil {
		t.Fatalf("Run = %v", err)
	}
	pool, err := pgxpool.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var unsent, total int
	pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE sent_at IS NULL), count(*) FROM outbox`).Scan(&unsent, &total)
	if total != 1 || unsent != 0 {
		t.Errorf("after Run returned: %d rows, %d unsent; want the drain's row published", total, unsent)
	}
}
