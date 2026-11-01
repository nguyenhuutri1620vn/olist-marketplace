package app_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/inbox"
)

func TestConsumersNeedADatabaseAndKafka(t *testing.T) {
	t.Setenv("OPS_ADDR", freeAddr(t))
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	c := []*inbox.Consumer{inbox.NewConsumer("g", "t")}
	for name, s := range map[string]app.Service{
		"no database": {Name: "payment-service", UsesKafka: true, Consumers: c},
		"no kafka":    {Name: "payment-service", Migrations: fstest.MapFS{}, Consumers: c},
	} {
		if err := app.Run(context.Background(), s); err == nil || !strings.Contains(err.Error(), "Consumers") {
			t.Errorf("%s: Run = %v, want a Consumers config error", name, err)
		}
	}
	t.Setenv("INBOX_MAX_ATTEMPTS", "0")
	err := app.Run(context.Background(), app.Service{Name: "payment-service", Migrations: fstest.MapFS{}, UsesKafka: true, Consumers: c})
	if err == nil || !strings.Contains(err.Error(), "INBOX_MAX_ATTEMPTS") {
		t.Errorf("Run = %v, want an INBOX_MAX_ATTEMPTS error", err)
	}
}

// A service with Consumers consumes: a published event is applied once and
// counted on /metrics, and the DLQ depth gauge is exported for its topic.
func TestConsumerServiceAppliesEvents(t *testing.T) {
	dsn, brokers := os.Getenv("DBMIGRATE_TEST_DSN"), os.Getenv("OUTBOX_TEST_KAFKA")
	if dsn == "" || brokers == "" {
		t.Skip("DBMIGRATE_TEST_DSN or OUTBOX_TEST_KAFKA not set")
	}
	eventsv1.RegisterForTest(t, probe{})
	db := isolatedSchema(t, dsn)
	t.Setenv("DATABASE_URL", db)
	t.Setenv("KAFKA_BROKERS", brokers)
	ops := freeAddr(t)
	t.Setenv("OPS_ADDR", ops)

	topic := "app-inbox-" + uuid.NewString()
	msg, err := eventbus.New(context.Background(), probe{OrderID: "0192f000-0000-7000-8000-0000000000ee"}, eventbus.Meta{OccurredAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := eventbus.Record(msg, time.Now())
	rec.Topic = topic
	p, err := eventbus.NewProducer(strings.Split(brokers, ","))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.ProduceSync(context.Background(), rec).FirstErr(); err != nil {
		t.Fatal(err)
	}
	p.Close()

	c := inbox.NewConsumer("app-inbox-"+uuid.NewString(), topic)
	inbox.Handle(c, func(ctx context.Context, tx pgx.Tx, _ eventsv1.Envelope, ev probe) error {
		_, err := tx.Exec(ctx, `INSERT INTO effects (order_id) VALUES ($1)`, ev.OrderID)
		return err
	})
	stop := run(t, app.Service{
		Name:       "payment-service",
		Migrations: fstest.MapFS{"000001_inbox.up.sql": {Data: []byte(inbox.SchemaSQL + "; CREATE TABLE effects (order_id text);")}},
		UsesKafka:  true,
		Consumers:  []*inbox.Consumer{c},
	})
	waitReady(t)
	pool, err := pgxpool.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var n int
	deadline := time.Now().Add(30 * time.Second)
	for n == 0 && time.Now().Before(deadline) {
		pool.QueryRow(context.Background(), `SELECT count(*) FROM effects`).Scan(&n)
		time.Sleep(50 * time.Millisecond)
	}
	if n != 1 {
		t.Fatalf("effects = %d, want the event applied once", n)
	}
	page := metricsPage(t, "http://"+ops+"/metrics")
	if !strings.Contains(page, `inbox_messages_total{consumer="`+c.Group()+`",outcome="applied",topic="`+topic+`"} 1`) ||
		!strings.Contains(page, `inbox_dlq_depth{topic="`+topic+`"} 0`) {
		t.Errorf("metrics lack the applied count or the DLQ depth:\n%s", grepLines(page, "inbox_"))
	}
	if err := stop(); err != nil {
		t.Errorf("Run = %v", err)
	}
}

func grepLines(page, prefix string) string {
	var out []string
	for _, l := range strings.Split(page, "\n") {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func TestConsumersNeedAGroupTopicsAndDistinctGroups(t *testing.T) {
	t.Setenv("OPS_ADDR", freeAddr(t))
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	for name, cs := range map[string][]*inbox.Consumer{
		"no group":   {inbox.NewConsumer("", "t")},
		"no topics":  {inbox.NewConsumer("g")},
		"same group": {inbox.NewConsumer("g", "a"), inbox.NewConsumer("g", "b")},
	} {
		err := app.Run(context.Background(), app.Service{Name: "payment-service", Migrations: fstest.MapFS{}, UsesKafka: true, Consumers: cs})
		if err == nil || !strings.Contains(err.Error(), "consumer") {
			t.Errorf("%s: Run = %v, want a consumer config error", name, err)
		}
	}
}
