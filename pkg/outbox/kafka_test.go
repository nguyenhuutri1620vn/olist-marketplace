package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
)

// kafkaBrokers returns OUTBOX_TEST_KAFKA (comma-separated) or skips.
func kafkaBrokers(t *testing.T) []string {
	t.Helper()
	v := os.Getenv("OUTBOX_TEST_KAFKA")
	if v == "" {
		t.Skip("OUTBOX_TEST_KAFKA not set")
	}
	return strings.Split(v, ",")
}

// With real Postgres and Kafka: events committed while the relay is down
// (killed after the commit, before publishing) are published once it
// restarts, in seq order, with the envelope and the traceparent header; a
// rolled-back write publishes nothing.
func TestRelayPublishesToKafkaAfterARestart(t *testing.T) {
	brokers := kafkaBrokers(t)
	pool := newPool(t)
	ctx := context.Background()

	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider()
	defer tp.Shutdown(ctx)
	spanCtx, span := tp.Tracer("test").Start(ctx, "use case")
	traceID := span.SpanContext().TraceID().String()

	// A unique aggregate keeps this run's records apart on the shared topic.
	order := uuid.Must(uuid.NewV7()).String()
	writeN := func(c context.Context, first, n int, rollback bool) {
		err := pgx.BeginFunc(c, pool, func(tx pgx.Tx) error {
			for i := first; i < first+n; i++ {
				if err := Write(c, tx, probe{OrderID: order, N: i}, meta); err != nil {
					return err
				}
			}
			if rollback {
				return errors.New("rolled back")
			}
			return nil
		})
		if err != nil && !rollback {
			t.Fatal(err)
		}
	}
	writeN(spanCtx, 1, 3, false)
	writeN(spanCtx, 99, 1, true)
	writeN(spanCtx, 4, 2, false)
	span.End()

	producer, err := eventbus.NewProducer(brokers)
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	relay := NewRelay(pool, producer, quiet, 20*time.Millisecond)
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { relay.Run(runCtx); done <- nil }()
	defer func() { stop(); <-done }()

	consumer, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.ConsumeTopics(eventsv1.TopicOrderEvents),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	var got []*kgo.Record
	deadline := time.Now().Add(20 * time.Second)
	for len(got) < 5 && time.Now().Before(deadline) {
		pctx, cancel := context.WithTimeout(ctx, time.Second)
		fs := consumer.PollFetches(pctx)
		cancel()
		fs.EachRecord(func(r *kgo.Record) {
			if string(r.Key) == order {
				got = append(got, r)
			}
		})
	}
	if nums := probeNumbers(t, got); !slices.Equal(nums, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("consumed %v, want [1 2 3 4 5] (rolled-back 99 must not appear)", nums)
	}
	for _, r := range got {
		var e eventsv1.Envelope
		if err := json.Unmarshal(r.Value, &e); err != nil {
			t.Fatal(err)
		}
		var header string
		for _, h := range r.Headers {
			if h.Key == "traceparent" {
				header = string(h.Value)
			}
		}
		if header == "" || header != e.Traceparent || !strings.Contains(header, traceID) {
			t.Errorf("traceparent header %q, envelope %q, want the writer's trace %s", header, e.Traceparent, traceID)
		}
		if e.PublishedAt.IsZero() || !e.OriginPublishedAt.Equal(e.PublishedAt) || e.AggregateID != order {
			t.Errorf("envelope %+v", e)
		}
	}
	if unsent(t, pool) != 0 {
		t.Errorf("unsent = %d", unsent(t, pool))
	}
}
