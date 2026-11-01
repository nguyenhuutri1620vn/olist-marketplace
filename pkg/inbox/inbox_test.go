package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
)

type probe struct {
	OrderID string `json:"order_id"`
	N       int    `json:"n"`
}

func (probe) EventType() string     { return "ProbeRecorded" }
func (probe) EventVersion() string  { return "1.3" }
func (probe) Topic() string         { return eventsv1.TopicOrderEvents }
func (p probe) AggregateID() string { return p.OrderID }

const orderID = "0192f000-0000-7000-8000-0000000000dd"

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// newPool creates a fresh schema with processed_events and a test "effects"
// table (dropped after the test). It skips without DBMIGRATE_TEST_DSN.
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("DBMIGRATE_TEST_DSN")
	if base == "" {
		t.Skip("DBMIGRATE_TEST_DSN not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("inbox_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, SchemaSQL+`; CREATE TABLE effects (n int NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	eventsv1.RegisterForTest(t, probe{})
	return pool
}

// record builds the Kafka record the relay would publish for p.
func record(t *testing.T, ctx context.Context, p probe, meta eventbus.Meta) *kgo.Record {
	t.Helper()
	if meta.OccurredAt.IsZero() {
		meta.OccurredAt = time.Now()
	}
	msg, err := eventbus.New(ctx, p, meta)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := eventbus.Record(msg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

// fakeDLQ records parked records; fail makes the next n produces fail.
type fakeDLQ struct {
	mu   sync.Mutex
	recs []*kgo.Record
	fail int
}

func (f *fakeDLQ) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(kgo.ProduceResults, len(rs))
	for i, r := range rs {
		out[i].Record = r
		if f.fail > 0 {
			out[i].Err = errors.New("dlq broker down")
		}
	}
	if f.fail > 0 {
		f.fail--
		return out
	}
	f.recs = append(f.recs, rs...)
	return out
}

// harness is a consumer whose handler writes p.N into effects and whose
// sleeps are recorded instead of slept.
type harness struct {
	c      *Consumer
	rt     *runtime
	dlq    *fakeDLQ
	sleeps []time.Duration
	calls  int
}

func newHarness(t *testing.T, pool *pgxpool.Pool, fn func(h *harness, p probe) error) *harness {
	t.Helper()
	h := &harness{dlq: &fakeDLQ{}}
	h.c = NewConsumer("test-consumer", eventsv1.TopicOrderEvents)
	Handle(h.c, func(ctx context.Context, tx pgx.Tx, _ eventsv1.Envelope, p probe) error {
		h.calls++
		if _, err := tx.Exec(ctx, `INSERT INTO effects (n) VALUES ($1)`, p.N); err != nil {
			return err
		}
		if fn != nil {
			return fn(h, p)
		}
		return nil
	})
	h.rt = &runtime{
		pool: pool, log: quiet, metrics: NewMetrics(), maxAttempts: 3, dlq: h.dlq,
		sleep: func(_ context.Context, d time.Duration) bool { h.sleeps = append(h.sleeps, d); return true },
	}
	return h
}

func (h *harness) process(t *testing.T, rec *kgo.Record) Outcome {
	t.Helper()
	out, ok := h.c.process(context.Background(), h.rt, rec)
	if !ok {
		t.Fatal("process interrupted")
	}
	return out
}

func effects(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM effects`).Scan(&n)
	return n
}

func processed(t *testing.T, pool *pgxpool.Pool) (outcome, reason string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `SELECT outcome, reason FROM processed_events`).Scan(&outcome, &reason)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	return outcome, reason
}

func counted(h *harness, outcome Outcome) float64 {
	return testutil.ToFloat64(h.rt.metrics.messages.WithLabelValues("test-consumer", eventsv1.TopicOrderEvents, string(outcome)))
}

func TestFirstDeliveryAppliesWithTheInboxRow(t *testing.T) {
	pool := newPool(t)
	h := newHarness(t, pool, nil)
	if out := h.process(t, record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{})); out != Applied {
		t.Fatalf("outcome = %s", out)
	}
	if o, _ := processed(t, pool); effects(t, pool) != 1 || o != "applied" || counted(h, Applied) != 1 {
		t.Errorf("effects %d, processed %q, applied counter %v", effects(t, pool), o, counted(h, Applied))
	}
}

func TestRedeliveryIsADuplicate(t *testing.T) {
	pool := newPool(t)
	h := newHarness(t, pool, nil)
	rec := record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{})
	h.process(t, rec)
	if out := h.process(t, rec); out != Duplicate || h.calls != 1 || effects(t, pool) != 1 {
		t.Errorf("redelivery: %s, handler calls %d, effects %d", out, h.calls, effects(t, pool))
	}
}

func TestAnotherConsumerAppliesTheSameEventToo(t *testing.T) {
	pool := newPool(t)
	rec := record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{})
	newHarness(t, pool, nil).process(t, rec)
	other := newHarness(t, pool, nil)
	other.c.group = "other-consumer"
	if out := other.process(t, rec); out != Applied {
		t.Errorf("second consumer: %s, want applied", out)
	}
}

func TestRejectRollsBackAndRecordsTheReason(t *testing.T) {
	pool := newPool(t)
	h := newHarness(t, pool, func(*harness, probe) error { return Reject("order not created") })
	rec := record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{})
	if out := h.process(t, rec); out != Rejected {
		t.Fatalf("outcome = %s", out)
	}
	o, reason := processed(t, pool)
	if effects(t, pool) != 0 || o != "rejected" || reason != "order not created" || len(h.sleeps) != 0 {
		t.Errorf("effects %d, processed %q %q, retries %d", effects(t, pool), o, reason, len(h.sleeps))
	}
	if out := h.process(t, rec); out != Duplicate || h.calls != 1 {
		t.Errorf("redelivered reject: %s, handler calls %d", out, h.calls)
	}
}

func TestATransientFailureIsRetriedInPlace(t *testing.T) {
	pool := newPool(t)
	h := newHarness(t, pool, func(h *harness, _ probe) error {
		if h.calls < 3 {
			return errors.New("connection reset")
		}
		return nil
	})
	if out := h.process(t, record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{})); out != Applied {
		t.Fatalf("outcome = %s", out)
	}
	if h.calls != 3 || effects(t, pool) != 1 || len(h.dlq.recs) != 0 {
		t.Errorf("calls %d, effects %d, parked %d", h.calls, effects(t, pool), len(h.dlq.recs))
	}
	if len(h.sleeps) != 2 || h.sleeps[0] != 100*time.Millisecond || h.sleeps[1] != 200*time.Millisecond {
		t.Errorf("backoff = %v, want [100ms 200ms]", h.sleeps)
	}
	if counted(h, Retryable) != 2 || counted(h, Applied) != 1 {
		t.Errorf("counters retryable %v applied %v", counted(h, Retryable), counted(h, Applied))
	}
}

func TestBackoffIsCappedAt5s(t *testing.T) {
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	for i, w := range want {
		if got := backoff(i + 1); got != w {
			t.Errorf("backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
	if got := backoff(10); got != 5*time.Second {
		t.Errorf("backoff(10) = %v, want 5s", got)
	}
	if got := backoff(100); got != 5*time.Second {
		t.Errorf("backoff(100) = %v, want 5s", got)
	}
}

func TestAPersistentFailureIsParkedOnTheDLQ(t *testing.T) {
	pool := newPool(t)
	h := newHarness(t, pool, func(*harness, probe) error { return errors.New("deadlock detected") })
	h.dlq.fail = 2 // the DLQ itself fails twice: keep trying, never skip
	rec := record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{})
	rec.Headers = append(rec.Headers, kgo.RecordHeader{Key: "x-extra", Value: []byte("kept")})
	if out := h.process(t, rec); out != Retryable {
		t.Fatalf("outcome = %s", out)
	}
	if h.calls != 3 || effects(t, pool) != 0 || len(h.dlq.recs) != 1 {
		t.Fatalf("calls %d, effects %d, parked %d", h.calls, effects(t, pool), len(h.dlq.recs))
	}
	p := h.dlq.recs[0]
	hdr := map[string]string{}
	for _, x := range p.Headers {
		hdr[x.Key] = string(x.Value)
	}
	if p.Topic != "order.events.dlq" || string(p.Key) != string(rec.Key) || string(p.Value) != string(rec.Value) ||
		hdr["x-extra"] != "kept" || hdr["dlq-attempts"] != "3" || hdr["dlq-reason"] != "deadlock detected" {
		t.Errorf("parked record topic %s headers %v", p.Topic, hdr)
	}
	if o, _ := processed(t, pool); o != "" {
		t.Errorf("parked message recorded as %q; a redrive must apply it", o)
	}
}

func TestBadMessagesAreRejectedWithoutRetry(t *testing.T) {
	pool := newPool(t)
	h := newHarness(t, pool, nil)
	good := record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{})
	var env eventsv1.Envelope
	json.Unmarshal(good.Value, &env)

	v2 := env
	v2.EventID = uuid.NewString()
	v2.EventVersion = "2.0"
	badPayload := env
	badPayload.EventID = uuid.NewString()
	badPayload.Payload = json.RawMessage(`{"n":"one"}`)
	for name, value := range map[string][]byte{
		"not json":        []byte("{nope"),
		"major version 2": mustJSON(t, v2),
		"bad payload":     mustJSON(t, badPayload),
	} {
		rec := &kgo.Record{Topic: good.Topic, Key: good.Key, Value: value}
		if out := h.process(t, rec); out != Rejected {
			t.Errorf("%s: outcome %s, want rejected", name, out)
		}
	}
	if h.calls != 0 || len(h.sleeps) != 0 || effects(t, pool) != 0 {
		t.Errorf("handler calls %d, retries %d", h.calls, len(h.sleeps))
	}
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM processed_events WHERE outcome = 'rejected' AND reason <> ''`).Scan(&n)
	if n != 2 {
		t.Errorf("recorded rejects with a reason = %d, want 2 (an undecodable envelope has no event_id)", n)
	}
	// A newer minor version decodes (added fields are ignored).
	minor := env
	minor.EventID = uuid.NewString()
	minor.EventVersion = "1.9"
	if out := h.process(t, &kgo.Record{Topic: good.Topic, Value: mustJSON(t, minor)}); out != Applied {
		t.Errorf("minor version bump: %s, want applied", out)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type other struct {
	OrderID string `json:"order_id"`
}

func (other) EventType() string     { return "OtherRecorded" }
func (other) EventVersion() string  { return "1.0" }
func (other) Topic() string         { return eventsv1.TopicOrderEvents }
func (o other) AggregateID() string { return o.OrderID }

func TestAnUnknownTypeIsAnAppliedNoOp(t *testing.T) {
	pool := newPool(t)
	eventsv1.RegisterForTest(t, other{})
	h := newHarness(t, pool, nil)
	msg, _ := eventbus.New(context.Background(), other{OrderID: orderID}, eventbus.Meta{OccurredAt: time.Now()})
	rec, _ := eventbus.Record(msg, time.Now())
	if out := h.process(t, rec); out != Applied || h.calls != 0 {
		t.Errorf("unknown type: %s, handler calls %d", out, h.calls)
	}
}

// spans installs an in-memory tracer and returns its recorder.
func spans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTracerProvider(prevTP); otel.SetTextMapPropagator(prevProp) })
	return rec
}

func TestReactorsDropHistoricalEventsButStillTraceThem(t *testing.T) {
	pool := newPool(t)
	sr := spans(t)
	h := newHarness(t, pool, nil)
	h.c.DropHistorical()
	ctx, parent := otel.Tracer("producer").Start(context.Background(), "publish")
	rec := record(t, ctx, probe{OrderID: orderID, N: 1}, eventbus.Meta{Historical: true})
	parent.End()
	if out := h.process(t, rec); out != Applied || h.calls != 0 || effects(t, pool) != 0 {
		t.Errorf("historical on a reactor: %s, calls %d", out, h.calls)
	}
	ended := sr.Ended()
	if len(ended) < 2 || ended[len(ended)-1].Parent().TraceID() != parent.SpanContext().TraceID() {
		t.Errorf("no consumer span in the producer's trace: %d spans", len(ended))
	}
}

func TestTheHandlerRunsInAChildSpanOfTheProducer(t *testing.T) {
	pool := newPool(t)
	sr := spans(t)
	var handlerSpan trace.SpanContext
	h := newHarness(t, pool, nil)
	Handle(h.c, func(ctx context.Context, _ pgx.Tx, _ eventsv1.Envelope, _ probe) error {
		handlerSpan = trace.SpanContextFromContext(ctx)
		return nil
	})
	ctx, parent := otel.Tracer("producer").Start(context.Background(), "publish")
	rec := record(t, ctx, probe{OrderID: orderID, N: 1}, eventbus.Meta{})
	parent.End()
	h.process(t, rec)
	if handlerSpan.TraceID() != parent.SpanContext().TraceID() || handlerSpan.SpanID() == parent.SpanContext().SpanID() {
		t.Errorf("handler span %v, producer span %v: want a child in the same trace", handlerSpan, parent.SpanContext())
	}
	var consumer sdktrace.ReadOnlySpan
	for _, s := range sr.Ended() {
		if s.SpanKind() == trace.SpanKindConsumer {
			consumer = s
		}
	}
	if consumer == nil || consumer.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Error("no consumer span whose parent is the producer span")
	}
}

func TestMaxAttemptsFromEnv(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want int
		ok   bool
	}{{"", 5, true}, {"1", 1, true}, {"20", 20, true}, {"21", 0, false}, {"0", 0, false}, {"-1", 0, false}, {"x", 0, false}} {
		t.Setenv("INBOX_MAX_ATTEMPTS", tc.env)
		got, err := MaxAttempts()
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("INBOX_MAX_ATTEMPTS=%q → %d, %v", tc.env, got, err)
		}
	}
}

func TestMalformedEnvelopesAreRejectedOnceWithAClearReason(t *testing.T) {
	pool := newPool(t)
	h := newHarness(t, pool, nil)
	good := record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{})
	var env eventsv1.Envelope
	json.Unmarshal(good.Value, &env)
	notUUID := env
	notUUID.EventID = "abc"
	noType := env
	noType.EventType = ""
	for name, tc := range map[string]struct {
		value  []byte
		reason string
	}{
		"event_id not a uuid": {mustJSON(t, notUUID), "event_id"},
		"no event_type":       {mustJSON(t, noType), "event_type"},
	} {
		out, reason := h.c.attempt(context.Background(), h.rt, &kgo.Record{Topic: good.Topic, Value: tc.value})
		if out != Rejected || !strings.Contains(reason, tc.reason) || strings.Contains(reason, "<nil>") {
			t.Errorf("%s: %s %q", name, out, reason)
		}
	}
}

func TestAPanickingHandlerIsRetryableNotACrash(t *testing.T) {
	pool := newPool(t)
	h := newHarness(t, pool, func(*harness, probe) error { panic("nil map") })
	if out := h.process(t, record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{})); out != Retryable {
		t.Errorf("outcome = %s", out)
	}
	if len(h.dlq.recs) != 1 || effects(t, pool) != 0 {
		t.Errorf("parked %d, effects %d", len(h.dlq.recs), effects(t, pool))
	}
}

// Stopping mid-retry (backoff interrupted) or while the DLQ is down must not
// let the offset be committed.
func TestStoppingMidRetryCommitsNothing(t *testing.T) {
	pool := newPool(t)
	h := newHarness(t, pool, func(*harness, probe) error { return errors.New("down") })
	h.rt.sleep = func(context.Context, time.Duration) bool { return false }
	rec := record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{})
	if _, ok := h.c.process(context.Background(), h.rt, rec); ok {
		t.Error("process reported a final outcome after an interrupted backoff")
	}
	h.rt.maxAttempts = 1
	h.dlq.fail = 1
	if _, ok := h.c.process(context.Background(), h.rt, rec); ok {
		t.Error("process reported a final outcome while the DLQ was down")
	}
}

func TestParkedRecordsNameTheConsumerAndAreCounted(t *testing.T) {
	pool := newPool(t)
	h := newHarness(t, pool, func(*harness, probe) error { return errors.New("down") })
	h.dlq.fail = 1
	h.process(t, record(t, context.Background(), probe{OrderID: orderID, N: 1}, eventbus.Meta{}))
	var consumer string
	for _, x := range h.dlq.recs[0].Headers {
		if x.Key == "dlq-consumer" {
			consumer = string(x.Value)
		}
	}
	if consumer != "test-consumer" {
		t.Errorf("dlq-consumer header = %q", consumer)
	}
	if got := testutil.ToFloat64(h.rt.metrics.parked.WithLabelValues(eventsv1.TopicOrderEvents)); got != 1 {
		t.Errorf("parked counter = %v after one park and one failed DLQ produce, want 1", got)
	}
}

func TestRedriveDropsOnlyTheDLQHeaders(t *testing.T) {
	got := withoutDLQHeaders([]kgo.RecordHeader{
		{Key: "traceparent", Value: []byte("tp")}, {Key: "dlq-reason", Value: []byte("x")},
		{Key: "dlq-attempts", Value: []byte("5")}, {Key: "dlq-consumer", Value: []byte("g")}, {Key: "x-extra", Value: []byte("y")},
	})
	if len(got) != 2 || got[0].Key != "traceparent" || got[1].Key != "x-extra" {
		t.Errorf("headers after redrive = %v", got)
	}
}
