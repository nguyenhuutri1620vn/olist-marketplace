// Package outbox is the transactional outbox (AD-6): a use case writes its
// events in its own DB transaction with Write, and one Relay per service
// publishes them to Kafka in seq order. There is no other way to publish.
//
// Delivery is at least once: a batch whose produce or mark-sent step fails
// is sent again, in the same order, so consumers dedupe on event_id.
package outbox

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kgo"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
)

// SchemaSQL creates the outbox tables; publishing services copy it into a
// migration.
//
//go:embed schema.sql
var SchemaSQL string

// batchSize bounds the rows one tick publishes.
const batchSize = 100

// Poll interval bounds (AD-6: every ≤ 200 ms; the floor stops a hot loop).
const (
	DefaultPollInterval = 200 * time.Millisecond
	minPollInterval     = 10 * time.Millisecond
	maxPollInterval     = 200 * time.Millisecond
)

// Bounds on one batch's Kafka produce, and on the last flush when stopping.
// franz-go retries a down broker forever; the timeout ends the tick instead.
const (
	publishTimeout = 10 * time.Second
	flushTimeout   = 5 * time.Second
)

// Write adds ev to the outbox inside tx, the use case's own transaction, so
// the event exists exactly when the state change commits. Types outside the
// catalog are eventsv1.ErrUnknownType. Writers that touch an Order lock its
// row first, so one Order's rows commit in seq order.
func Write(ctx context.Context, tx pgx.Tx, ev eventsv1.Event, meta eventbus.Meta) error {
	msg, err := eventbus.New(ctx, ev, meta)
	if err != nil {
		return err
	}
	env, err := json.Marshal(msg.Envelope)
	if err != nil {
		return fmt.Errorf("encode envelope: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox (event_id, topic, envelope, traceparent) VALUES ($1, $2, $3, $4)`,
		msg.Envelope.EventID, msg.Topic, env, msg.Envelope.Traceparent)
	if err != nil {
		return fmt.Errorf("write outbox %s: %w", msg.Envelope.EventType, err)
	}
	return nil
}

// PollInterval reads OUTBOX_POLL_INTERVAL (a Go duration, default 200ms);
// values outside [10ms, 200ms] are rejected.
func PollInterval() (time.Duration, error) {
	v := os.Getenv("OUTBOX_POLL_INTERVAL")
	if v == "" {
		return DefaultPollInterval, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < minPollInterval || d > maxPollInterval {
		return 0, fmt.Errorf("OUTBOX_POLL_INTERVAL=%q: want a duration in [%v, %v]", v, minPollInterval, maxPollInterval)
	}
	return d, nil
}

// Producer sends records synchronously; *kgo.Client from
// eventbus.NewProducer is one.
type Producer interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
}

// Relay publishes unsent outbox rows in seq order. Run it in one goroutine.
type Relay struct {
	pool           *pgxpool.Pool
	producer       Producer
	log            *slog.Logger
	interval       time.Duration
	publishTimeout time.Duration
	unsent         prometheus.Gauge
}

// NewRelay returns a relay polling pool every interval (the default when
// interval is not positive).
func NewRelay(pool *pgxpool.Pool, producer Producer, log *slog.Logger, interval time.Duration) *Relay {
	r := newRelay(pool, producer, log)
	if interval > 0 {
		r.interval = interval
	}
	return r
}

func newRelay(pool *pgxpool.Pool, producer Producer, log *slog.Logger) *Relay {
	unsent := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "outbox_unsent_rows",
		Help: "Outbox rows not yet published to Kafka (NaN until first counted).",
	})
	unsent.Set(math.NaN())
	return &Relay{
		pool:           pool,
		producer:       producer,
		log:            log,
		interval:       DefaultPollInterval,
		publishTimeout: publishTimeout,
		unsent:         unsent,
	}
}

// Collector exposes outbox_unsent_rows.
func (r *Relay) Collector() prometheus.Collector { return r.unsent }

// Run polls until ctx ends, then flushes once more so rows committed before
// the stop are published. A failed tick is logged and retried on the next
// one; it never skips a row.
func (r *Relay) Run(ctx context.Context) {
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		if _, err := r.tick(ctx); err != nil && ctx.Err() == nil {
			r.log.ErrorContext(ctx, "outbox relay tick failed", "err", err)
		}
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushTimeout)
			defer cancel()
			if _, err := r.tick(fctx); err != nil {
				r.log.ErrorContext(fctx, "outbox relay final flush failed", "err", err)
			}
			return
		case <-t.C:
		}
	}
}

// tick publishes up to batchSize unsent rows in seq order and marks them sent
// in the same transaction. It returns how many it published. If another
// instance holds the relay lock it publishes nothing. The gauge is refreshed
// before and after, so it shows the backlog while Kafka is down.
func (r *Relay) tick(ctx context.Context) (int, error) {
	if err := r.count(ctx); err != nil {
		return 0, err
	}
	n, err := r.publish(ctx)
	if cerr := r.count(ctx); err == nil {
		err = cerr
	}
	return n, err
}

func (r *Relay) count(ctx context.Context) error {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE sent_at IS NULL`).Scan(&n); err != nil {
		return fmt.Errorf("count unsent: %w", err)
	}
	r.unsent.Set(float64(n))
	return nil
}

func (r *Relay) publish(ctx context.Context) (int, error) {
	var n int
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		// SKIP LOCKED only guards against a second instance: it skips the
		// whole tick, never a row.
		var lock int
		err := tx.QueryRow(ctx, `SELECT id FROM outbox_relay_lock FOR UPDATE SKIP LOCKED`).Scan(&lock)
		if errors.Is(err, pgx.ErrNoRows) {
			// Held by another instance, or missing: only the latter is an error.
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM outbox_relay_lock)`).Scan(&exists); err != nil {
				return fmt.Errorf("check relay lock: %w", err)
			}
			if !exists {
				return errors.New("outbox_relay_lock has no row; apply outbox.SchemaSQL")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("take relay lock: %w", err)
		}
		// No high-water mark: a lower seq committed late is still unsent here.
		rows, err := tx.Query(ctx, `SELECT seq, topic, envelope FROM outbox WHERE sent_at IS NULL ORDER BY seq LIMIT $1`, batchSize)
		if err != nil {
			return fmt.Errorf("read outbox: %w", err)
		}
		type row struct {
			Seq      int64
			Topic    string
			Envelope eventsv1.Envelope
		}
		pending, err := pgx.CollectRows(rows, pgx.RowToStructByPos[row])
		if err != nil {
			return fmt.Errorf("read outbox: %w", err)
		}
		if len(pending) == 0 {
			return nil
		}
		now := time.Now()
		recs := make([]*kgo.Record, len(pending))
		seqs := make([]int64, len(pending))
		for i, p := range pending {
			if recs[i], err = eventbus.Record(eventbus.Message{Topic: p.Topic, Envelope: p.Envelope}, now); err != nil {
				return err
			}
			seqs[i] = p.Seq
		}
		pctx, cancel := context.WithTimeout(ctx, r.publishTimeout)
		defer cancel()
		if err := r.producer.ProduceSync(pctx, recs...).FirstErr(); err != nil {
			return fmt.Errorf("publish outbox: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE outbox SET sent_at = now() WHERE seq = ANY($1)`, seqs); err != nil {
			return fmt.Errorf("mark outbox sent: %w", err)
		}
		n = len(pending)
		return nil
	})
	return n, err
}
