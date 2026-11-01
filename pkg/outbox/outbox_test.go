package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/url"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/twmb/franz-go/pkg/kgo"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
)

type probe struct {
	OrderID string `json:"order_id"`
	N       int    `json:"n"`
}

func (probe) EventType() string     { return "ProbeRecorded" }
func (probe) EventVersion() string  { return "1.0" }
func (probe) Topic() string         { return eventsv1.TopicOrderEvents }
func (p probe) AggregateID() string { return p.OrderID }

type unlisted struct{ probe }

func (unlisted) EventType() string { return "NotListed" }

const orderID = "0192f000-0000-7000-8000-0000000000aa"

var meta = eventbus.Meta{OccurredAt: time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// newPool creates a fresh schema with the outbox tables (dropped after the
// test). It skips the test when DBMIGRATE_TEST_DSN is not set.
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
	schema := fmt.Sprintf("outbox_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, SchemaSQL); err != nil {
		t.Fatal(err)
	}
	eventsv1.RegisterForTest(t, probe{})
	return pool
}

// write commits n probe events, numbered from first.
func write(t *testing.T, pool *pgxpool.Pool, first, n int) {
	t.Helper()
	err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
		for i := first; i < first+n; i++ {
			if err := Write(context.Background(), tx, probe{OrderID: orderID, N: i}, meta); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// fakeProducer records produced records; fail, when set, fails the next call.
type fakeProducer struct {
	mu   sync.Mutex
	recs []*kgo.Record
	fail error
}

func (f *fakeProducer) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(kgo.ProduceResults, len(rs))
	for i, r := range rs {
		out[i] = kgo.ProduceResult{Record: r, Err: f.fail}
	}
	if f.fail == nil {
		f.recs = append(f.recs, rs...)
	}
	f.fail = nil
	return out
}

// numbers decodes the probe numbers of the produced records, in order.
func (f *fakeProducer) numbers(t *testing.T) []int {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	return probeNumbers(t, f.recs)
}

func probeNumbers(t *testing.T, recs []*kgo.Record) []int {
	t.Helper()
	var out []int
	for _, r := range recs {
		var e eventsv1.Envelope
		var p probe
		if err := json.Unmarshal(r.Value, &e); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p.N)
	}
	return out
}

func unsent(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox WHERE sent_at IS NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWriteStoresTheEnvelopeInTheCallersTransaction(t *testing.T) {
	pool := newPool(t)
	write(t, pool, 1, 2)
	rows, err := pool.Query(context.Background(), `SELECT seq, topic, envelope, traceparent, sent_at IS NULL FROM outbox ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Seq         int64
		Topic       string
		Envelope    eventsv1.Envelope
		Traceparent string
		Unsent      bool
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[row])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Seq >= got[1].Seq {
		t.Fatalf("rows = %+v, want two with increasing seq", got)
	}
	e := got[0].Envelope
	if got[0].Topic != eventsv1.TopicOrderEvents || !got[0].Unsent || e.EventType != "ProbeRecorded" || e.AggregateID != orderID ||
		!e.OccurredAt.Equal(meta.OccurredAt) || !e.PublishedAt.IsZero() {
		t.Errorf("row = %+v", got[0])
	}
}

func TestRolledBackWritesLeaveNothing(t *testing.T) {
	pool := newPool(t)
	rollback := errors.New("use case failed")
	err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
		if err := Write(context.Background(), tx, probe{OrderID: orderID, N: 1}, meta); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	p := &fakeProducer{}
	if n, err := newRelay(pool, p, quiet).tick(context.Background()); n != 0 || err != nil || unsent(t, pool) != 0 || len(p.numbers(t)) != 0 {
		t.Errorf("tick = %d, %v; unsent %d; produced %v", n, err, unsent(t, pool), p.numbers(t))
	}
}

func TestWriteRejectsTypesOutsideTheCatalog(t *testing.T) {
	pool := newPool(t)
	err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
		return Write(context.Background(), tx, unlisted{probe{OrderID: orderID}}, meta)
	})
	if !errors.Is(err, eventsv1.ErrUnknownType) || unsent(t, pool) != 0 {
		t.Errorf("err = %v, unsent %d", err, unsent(t, pool))
	}
}

func TestTickPublishesUnsentRowsInSeqOrderAndMarksThem(t *testing.T) {
	pool := newPool(t)
	write(t, pool, 1, 3)
	write(t, pool, 4, 2)
	p := &fakeProducer{}
	r := newRelay(pool, p, quiet)
	if n, err := r.tick(context.Background()); n != 5 || err != nil {
		t.Fatalf("tick = %d, %v", n, err)
	}
	if got := p.numbers(t); !slices.Equal(got, []int{1, 2, 3, 4, 5}) || unsent(t, pool) != 0 {
		t.Errorf("produced %v, unsent %d", got, unsent(t, pool))
	}
	if string(p.recs[0].Key) != orderID {
		t.Errorf("key = %q", p.recs[0].Key)
	}
	// Sent rows are not published again.
	if n, _ := r.tick(context.Background()); n != 0 || len(p.numbers(t)) != 5 {
		t.Errorf("second tick published %d", n)
	}
}

func TestTickPublishesInBatchesOfAtMost100(t *testing.T) {
	pool := newPool(t)
	write(t, pool, 1, 150)
	p := &fakeProducer{}
	r := newRelay(pool, p, quiet)
	if n, _ := r.tick(context.Background()); n != 100 {
		t.Errorf("first tick = %d, want 100", n)
	}
	if n, _ := r.tick(context.Background()); n != 50 {
		t.Errorf("second tick = %d, want 50", n)
	}
	want := make([]int, 150)
	for i := range want {
		want[i] = i + 1
	}
	if !slices.Equal(p.numbers(t), want) {
		t.Error("150 rows not published in order")
	}
}

func TestFailedProduceMarksNothingAndTheNextTickResendsInOrder(t *testing.T) {
	pool := newPool(t)
	write(t, pool, 1, 3)
	p := &fakeProducer{fail: errors.New("broker down")}
	r := newRelay(pool, p, quiet)
	if _, err := r.tick(context.Background()); err == nil {
		t.Fatal("tick ignored the produce error")
	}
	if unsent(t, pool) != 3 {
		t.Fatalf("unsent = %d after a failed produce, want 3", unsent(t, pool))
	}
	write(t, pool, 4, 1)
	if n, err := r.tick(context.Background()); n != 4 || err != nil {
		t.Fatalf("retry tick = %d, %v", n, err)
	}
	if got := p.numbers(t); !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Errorf("produced %v", got)
	}
}

// No high-water mark: a lower seq that commits after a higher one is sent.
func TestALowerSeqCommittedLaterIsStillPublished(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	slow, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Rollback(ctx)
	if err := Write(ctx, slow, probe{OrderID: orderID, N: 1}, meta); err != nil {
		t.Fatal(err)
	}
	write(t, pool, 2, 1)
	p := &fakeProducer{}
	r := newRelay(pool, p, quiet)
	r.tick(ctx)
	if err := slow.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	r.tick(ctx)
	if got := p.numbers(t); !slices.Equal(got, []int{2, 1}) || unsent(t, pool) != 0 {
		t.Errorf("produced %v, unsent %d", got, unsent(t, pool))
	}
}

// A second instance skips its tick while the first holds the relay lock.
func TestASecondInstanceSkipsWhileTheLockIsHeld(t *testing.T) {
	pool := newPool(t)
	write(t, pool, 1, 2)
	ctx := context.Background()
	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, `SELECT 1 FROM outbox_relay_lock FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	p := &fakeProducer{}
	r := newRelay(pool, p, quiet)
	if n, err := r.tick(ctx); n != 0 || err != nil || len(p.numbers(t)) != 0 {
		t.Errorf("tick while locked = %d, %v, produced %v", n, err, p.numbers(t))
	}
	holder.Rollback(ctx)
	if n, _ := r.tick(ctx); n != 2 {
		t.Errorf("tick after release = %d, want 2", n)
	}
}

func TestUnsentGaugeCountsUnsentRows(t *testing.T) {
	pool := newPool(t)
	write(t, pool, 1, 3)
	p := &fakeProducer{fail: errors.New("down")}
	r := newRelay(pool, p, quiet)
	r.tick(context.Background())
	if got := testutil.ToFloat64(r.unsent); got != 3 {
		t.Errorf("gauge = %v after a failed tick, want 3", got)
	}
	r.tick(context.Background())
	if got := testutil.ToFloat64(r.unsent); got != 0 {
		t.Errorf("gauge = %v after sending, want 0", got)
	}
	if n, err := testutil.GatherAndCount(registry(r), "outbox_unsent_rows"); n != 1 || err != nil {
		t.Errorf("collector exposes outbox_unsent_rows %d times, %v", n, err)
	}
}

func TestRunPollsUntilCanceled(t *testing.T) {
	pool := newPool(t)
	p := &fakeProducer{}
	r := newRelay(pool, p, quiet)
	r.interval = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { r.Run(ctx); done <- nil }()
	write(t, pool, 1, 2)
	deadline := time.Now().Add(2 * time.Second)
	for len(p.numbers(t)) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run = %v after cancel, want nil", err)
	}
	if got := p.numbers(t); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("produced %v", got)
	}
}

func TestPollIntervalFromEnv(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want time.Duration
		ok   bool
	}{
		{"", 200 * time.Millisecond, true},
		{"50ms", 50 * time.Millisecond, true},
		{"200ms", 200 * time.Millisecond, true},
		{"201ms", 0, false},
		{"1s", 0, false},
		{"0", 0, false},
		{"1ns", 0, false},
		{"9ms", 0, false},
		{"10ms", 10 * time.Millisecond, true},
		{"-5ms", 0, false},
		{"soon", 0, false},
	} {
		t.Setenv("OUTBOX_POLL_INTERVAL", tc.env)
		got, err := PollInterval()
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("OUTBOX_POLL_INTERVAL=%q → %v, %v; want %v ok=%v", tc.env, got, err, tc.want, tc.ok)
		}
	}
}

func registry(r *Relay) *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(r.Collector())
	return reg
}

// blockingProducer never answers until its context ends, like franz-go
// retrying against a broker that is down.
type blockingProducer struct{}

func (blockingProducer) ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	<-ctx.Done()
	out := make(kgo.ProduceResults, len(rs))
	for i, r := range rs {
		out[i] = kgo.ProduceResult{Record: r, Err: ctx.Err()}
	}
	return out
}

func TestATickGivesUpWhenKafkaDoesNotAnswer(t *testing.T) {
	pool := newPool(t)
	write(t, pool, 1, 2)
	r := newRelay(pool, blockingProducer{}, quiet)
	r.publishTimeout = 50 * time.Millisecond
	start := time.Now()
	if _, err := r.tick(context.Background()); err == nil {
		t.Error("tick reported success with no answer from Kafka")
	}
	if time.Since(start) > 2*time.Second || unsent(t, pool) != 2 {
		t.Errorf("tick took %v, unsent %d; want a bounded failure leaving 2 unsent", time.Since(start), unsent(t, pool))
	}
	if got := testutil.ToFloat64(r.unsent); got != 2 {
		t.Errorf("gauge = %v during the outage, want 2", got)
	}
}

func TestTheGaugeIsUnknownUntilTheFirstCount(t *testing.T) {
	r := newRelay(nil, &fakeProducer{}, quiet)
	if got := testutil.ToFloat64(r.unsent); !math.IsNaN(got) {
		t.Errorf("gauge before any tick = %v, want NaN", got)
	}
}

func TestAMissingLockRowIsAnError(t *testing.T) {
	pool := newPool(t)
	write(t, pool, 1, 1)
	if _, err := pool.Exec(context.Background(), `DELETE FROM outbox_relay_lock`); err != nil {
		t.Fatal(err)
	}
	if _, err := newRelay(pool, &fakeProducer{}, quiet).tick(context.Background()); err == nil {
		t.Error("tick without the relay lock row reported success")
	}
}

// Stopping the relay publishes what is already committed before Run returns.
func TestRunFlushesOnceMoreWhenStopped(t *testing.T) {
	pool := newPool(t)
	p := &fakeProducer{}
	r := newRelay(pool, p, quiet)
	r.interval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond) // the first tick ran on an empty outbox
	write(t, pool, 1, 2)
	cancel()
	<-done
	if got := p.numbers(t); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("produced %v after stop, want [1 2]", got)
	}
}

func TestNewRelayDefaultsANonPositiveInterval(t *testing.T) {
	if r := NewRelay(nil, &fakeProducer{}, quiet, 0); r.interval != DefaultPollInterval {
		t.Errorf("interval = %v, want the default", r.interval)
	}
}
