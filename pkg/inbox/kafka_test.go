package inbox

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/twmb/franz-go/pkg/kgo"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
)

func kafkaBrokers(t *testing.T) []string {
	t.Helper()
	v := os.Getenv("OUTBOX_TEST_KAFKA")
	if v == "" {
		t.Skip("OUTBOX_TEST_KAFKA not set")
	}
	return strings.Split(v, ",")
}

// produce publishes probes to topic as the relay would (keyed by order).
func produce(t *testing.T, brokers []string, topic string, recs ...*kgo.Record) {
	t.Helper()
	p, err := eventbus.NewProducer(brokers)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, r := range recs {
		r.Topic = topic
	}
	if err := p.ProduceSync(context.Background(), recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

// start runs c until the test ends.
func start(t *testing.T, c *Consumer, pool *pgxpool.Pool, brokers []string, maxAttempts int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- c.Run(ctx, Deps{Pool: pool, Brokers: brokers, Log: quiet, Metrics: NewMetrics(), MaxAttempts: maxAttempts})
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run = %v", err)
		}
	})
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func appliedNs(t *testing.T, pool *pgxpool.Pool) []int {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT n FROM effects ORDER BY ctid`)
	if err != nil {
		t.Fatal(err)
	}
	ns, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		t.Fatal(err)
	}
	return ns
}

// With real Postgres and Kafka: an event delivered twice is applied once, a
// handler failing transiently is retried without skipping the message, and
// a new group reads what was published before it first started.
func TestConsumerAppliesOnceAndRetriesWithoutSkipping(t *testing.T) {
	brokers := kafkaBrokers(t)
	pool := newPool(t)
	topic := "inbox-test-" + uuid.NewString()
	ctx := context.Background()
	first := record(t, ctx, probe{OrderID: orderID, N: 1}, eventbus.Meta{})
	dup := &kgo.Record{Key: first.Key, Value: first.Value, Headers: first.Headers}
	produce(t, brokers, topic, first, dup,
		record(t, ctx, probe{OrderID: orderID, N: 2}, eventbus.Meta{}),
		record(t, ctx, probe{OrderID: orderID, N: 3}, eventbus.Meta{}))

	var mu sync.Mutex
	failures := 0
	c := NewConsumer("inbox-test-"+uuid.NewString(), topic)
	Handle(c, func(ctx context.Context, tx pgx.Tx, _ eventsv1.Envelope, p probe) error {
		mu.Lock()
		defer mu.Unlock()
		if p.N == 2 && failures < 2 {
			failures++
			return errors.New("connection reset")
		}
		_, err := tx.Exec(ctx, `INSERT INTO effects (n) VALUES ($1)`, p.N)
		return err
	})
	start(t, c, pool, brokers, 5)
	waitFor(t, "three effects", func() bool { return len(appliedNs(t, pool)) >= 3 })
	time.Sleep(300 * time.Millisecond) // a wrongly re-applied duplicate would show up now
	if got := appliedNs(t, pool); !slices.Equal(got, []int{1, 2, 3}) || failures != 2 {
		t.Errorf("effects %v after %d transient failures, want [1 2 3] after 2", got, failures)
	}
}

// A message that keeps failing is parked on <topic>.dlq and the partition
// moves on; redriving it brings it back, it is applied, and the depth is 0.
func TestParkThenRedrive(t *testing.T) {
	brokers := kafkaBrokers(t)
	pool := newPool(t)
	topic := "inbox-test-" + uuid.NewString()
	ctx := context.Background()
	produce(t, brokers, topic,
		record(t, ctx, probe{OrderID: orderID, N: 1}, eventbus.Meta{}),
		record(t, ctx, probe{OrderID: orderID, N: 2}, eventbus.Meta{}))

	var mu sync.Mutex
	broken := true
	c := NewConsumer("inbox-test-"+uuid.NewString(), topic)
	Handle(c, func(ctx context.Context, tx pgx.Tx, _ eventsv1.Envelope, p probe) error {
		mu.Lock()
		defer mu.Unlock()
		if p.N == 1 && broken {
			return errors.New("deadlock detected")
		}
		_, err := tx.Exec(ctx, `INSERT INTO effects (n) VALUES ($1)`, p.N)
		return err
	})
	start(t, c, pool, brokers, 2)
	waitFor(t, "the second message", func() bool { return slices.Equal(appliedNs(t, pool), []int{2}) })

	depth := func() int64 {
		d, err := DLQDepth(ctx, brokers, topic)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	waitFor(t, "a parked message", func() bool { return depth() == 1 })
	m := NewMetrics()
	wctx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	go WatchDLQDepth(wctx, brokers, []string{topic}, m, quiet, 20*time.Millisecond)
	waitFor(t, "the depth gauge at 1", func() bool { return testutil.ToFloat64(m.dlqDepth.WithLabelValues(topic)) == 1 })
	nonEmpty, err := NonEmptyDLQs(ctx, brokers)
	if err != nil || nonEmpty[topic+DLQSuffix] != 1 {
		t.Errorf("NonEmptyDLQs = %v, %v; want %s: 1", nonEmpty, err, topic+DLQSuffix)
	}

	mu.Lock()
	broken = false
	mu.Unlock()
	n, err := Redrive(ctx, brokers, topic)
	if err != nil || n != 1 {
		t.Fatalf("Redrive = %d, %v; want 1", n, err)
	}
	if d := depth(); d != 0 {
		t.Errorf("depth after redrive = %d, want 0", d)
	}
	waitFor(t, "the depth gauge at 0", func() bool { return testutil.ToFloat64(m.dlqDepth.WithLabelValues(topic)) == 0 })
	waitFor(t, "the redriven message", func() bool { return slices.Equal(appliedNs(t, pool), []int{2, 1}) })
	if n, err := Redrive(ctx, brokers, topic); n != 0 || err != nil {
		t.Errorf("second Redrive = %d, %v; want nothing left", n, err)
	}
}

func TestDLQDepthOfATopicWithoutADLQIsZero(t *testing.T) {
	brokers := kafkaBrokers(t)
	if d, err := DLQDepth(context.Background(), brokers, "inbox-test-"+uuid.NewString()); d != 0 || err != nil {
		t.Errorf("DLQDepth = %d, %v", d, err)
	}
}
