// Package inbox runs idempotent Kafka consumers (AD-7). Each handler applies
// its change in one DB transaction with a processed_events row, so a
// redelivered event is skipped. Every message ends in one outcome: applied,
// duplicate, rejected (domain-invalid, recorded and acked) or retryable
// (retried in place with backoff, then parked on <topic>.dlq).
package inbox

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
)

// SchemaSQL creates processed_events; consuming services copy it into a
// migration.
//
//go:embed schema.sql
var SchemaSQL string

// Outcome is how a message ended (AD-7).
type Outcome string

const (
	Applied   Outcome = "applied"
	Duplicate Outcome = "duplicate"
	Rejected  Outcome = "rejected"
	Retryable Outcome = "retryable"
)

// DefaultMaxAttempts is used when INBOX_MAX_ATTEMPTS is unset; maxMaxAttempts
// keeps one record's retries well inside a rebalance timeout.
const (
	DefaultMaxAttempts = 5
	maxMaxAttempts     = 20
)

// Backoff between retryable attempts: 100ms doubling, at most 5s.
const (
	firstBackoff = 100 * time.Millisecond
	maxBackoff   = 5 * time.Second
)

// MaxAttempts reads INBOX_MAX_ATTEMPTS: how many retryable attempts before a
// message is parked on the DLQ (default 5).
func MaxAttempts() (int, error) {
	v := os.Getenv("INBOX_MAX_ATTEMPTS")
	if v == "" {
		return DefaultMaxAttempts, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxMaxAttempts {
		return 0, fmt.Errorf("INBOX_MAX_ATTEMPTS=%q: want an integer in [1, %d]", v, maxMaxAttempts)
	}
	return n, nil
}

func backoff(attempt int) time.Duration {
	d := firstBackoff
	for i := 1; i < attempt && d < maxBackoff; i++ {
		d *= 2
	}
	return min(d, maxBackoff)
}

// rejection is a domain-invalid outcome chosen by a handler.
type rejection struct{ reason string }

func (r rejection) Error() string { return "rejected: " + r.reason }

// Reject is what a handler returns for a domain-invalid event: its changes
// are rolled back, the event is recorded with reason and acked, never retried.
func Reject(reason string) error { return rejection{reason} }

// handler decodes the payload for one event type and applies it in tx.
type handler struct {
	major string
	apply func(ctx context.Context, tx pgx.Tx, env eventsv1.Envelope) (decodeErr, err error)
}

// Consumer is one consumer group: its topics and a handler per event type.
// The group id is also the processed_events consumer name.
type Consumer struct {
	group    string
	topics   []string
	reactor  bool
	handlers map[string]handler
}

// NewConsumer returns a consumer for group reading topics.
func NewConsumer(group string, topics ...string) *Consumer {
	return &Consumer{group: group, topics: topics, handlers: map[string]handler{}}
}

// Group is the consumer group id.
func (c *Consumer) Group() string { return c.group }

// Topics are the topics the consumer reads.
func (c *Consumer) Topics() []string { return c.topics }

// DropHistorical makes c a process reactor: historical=true envelopes are
// acked before decoding (AD-12). Projectors keep them.
func (c *Consumer) DropHistorical() *Consumer {
	c.reactor = true
	return c
}

// Handle registers fn for T's event type. T must be in the catalog. fn's
// change and the inbox row commit together; it returns Reject(reason) for a
// domain-invalid event and any other error for a retryable failure.
func Handle[T eventsv1.Event](c *Consumer, fn func(ctx context.Context, tx pgx.Tx, env eventsv1.Envelope, ev T) error) {
	var zero T
	if err := eventsv1.Check(zero); err != nil {
		panic(fmt.Sprintf("inbox.Handle: %v", err))
	}
	major, _, _ := strings.Cut(zero.EventVersion(), ".")
	c.handlers[zero.EventType()] = handler{
		major: major,
		apply: func(ctx context.Context, tx pgx.Tx, env eventsv1.Envelope) (error, error) {
			var ev T
			if err := json.Unmarshal(env.Payload, &ev); err != nil {
				return fmt.Errorf("decode %s payload: %w", env.EventType, err), nil
			}
			return nil, fn(ctx, tx, env, ev)
		},
	}
}

// safeApply turns a handler panic into a retryable error, so one bad
// handler path parks its message instead of crash-looping the service.
func (h handler) safeApply(ctx context.Context, tx pgx.Tx, env eventsv1.Envelope) (decodeErr, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panic: %v", p)
		}
	}()
	return h.apply(ctx, tx, env)
}

// Producer sends records synchronously (the DLQ).
type Producer interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
}

// runtime is what processing needs besides the consumer's own definition.
type runtime struct {
	pool        *pgxpool.Pool
	log         *slog.Logger
	metrics     *Metrics
	maxAttempts int
	dlq         Producer
	// sleep waits d or until ctx ends; false means ctx ended.
	sleep func(ctx context.Context, d time.Duration) bool
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

const tracerName = "github.com/nguyenhuutri1620vn/olist-marketplace/pkg/inbox"

// process handles one record to a final outcome, retrying in place and then
// parking it on the DLQ. ok is false when ctx ended first: the offset must
// not be committed.
func (c *Consumer) process(ctx context.Context, rt *runtime, rec *kgo.Record) (out Outcome, ok bool) {
	// The span comes first, so a message no handler acts on is still traced.
	carrier := headerCarrier(rec.Headers)
	ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	ctx, span := otel.Tracer(tracerName).Start(ctx, "consume "+rec.Topic, trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attribute.String("messaging.consumer.group.name", c.group),
			attribute.String("messaging.destination.name", rec.Topic)))
	defer span.End()

	for attempt := 1; ; attempt++ {
		out, reason := c.attempt(ctx, rt, rec)
		rt.metrics.messages.WithLabelValues(c.group, rec.Topic, string(out)).Inc()
		span.SetAttributes(attribute.String("inbox.outcome", string(out)))
		if out != Retryable {
			if out == Rejected {
				rt.log.WarnContext(ctx, "message rejected", "consumer", c.group, "topic", rec.Topic, "reason", reason)
			}
			return out, true
		}
		rt.log.WarnContext(ctx, "message failed", "consumer", c.group, "topic", rec.Topic, "attempt", attempt, "err", reason)
		if attempt >= rt.maxAttempts {
			span.SetStatus(codes.Error, reason)
			return Retryable, c.park(ctx, rt, rec, reason, attempt)
		}
		if !rt.sleep(ctx, backoff(attempt)) {
			return Retryable, false
		}
	}
}

// park copies rec to <topic>.dlq, retrying until it is stored or ctx ends.
func (c *Consumer) park(ctx context.Context, rt *runtime, rec *kgo.Record, reason string, attempts int) bool {
	parked := &kgo.Record{
		Topic: rec.Topic + DLQSuffix,
		Key:   rec.Key,
		Value: rec.Value,
		Headers: append(withoutDLQHeaders(rec.Headers),
			kgo.RecordHeader{Key: headerDLQReason, Value: []byte(reason)},
			kgo.RecordHeader{Key: headerDLQAttempts, Value: []byte(strconv.Itoa(attempts))},
			kgo.RecordHeader{Key: headerDLQConsumer, Value: []byte(c.group)}),
	}
	for try := 1; ; try++ {
		err := rt.dlq.ProduceSync(ctx, parked).FirstErr()
		if err == nil {
			rt.metrics.parked.WithLabelValues(rec.Topic).Inc()
			rt.log.ErrorContext(ctx, "message parked on DLQ", "consumer", c.group, "topic", rec.Topic, "reason", reason)
			return true
		}
		rt.log.ErrorContext(ctx, "DLQ produce failed; retrying", "topic", parked.Topic, "err", err)
		if !rt.sleep(ctx, backoff(try)) {
			return false
		}
	}
}

// attempt runs one try. reason explains rejected and retryable outcomes.
func (c *Consumer) attempt(ctx context.Context, rt *runtime, rec *kgo.Record) (Outcome, string) {
	// Without a usable event_id there is nothing to record: only counted and logged.
	var env eventsv1.Envelope
	if err := json.Unmarshal(rec.Value, &env); err != nil {
		return Rejected, fmt.Sprintf("undecodable envelope: %v", err)
	}
	if _, err := uuid.Parse(env.EventID); err != nil {
		return Rejected, fmt.Sprintf("envelope event_id %q is not a uuid", env.EventID)
	}
	if env.EventType == "" {
		return Rejected, "envelope has no event_type"
	}
	if c.reactor && env.Historical {
		return Applied, ""
	}
	h, known := c.handlers[env.EventType]
	if !known {
		return Applied, ""
	}
	if major, _, _ := strings.Cut(env.EventVersion, "."); major != h.major {
		return c.reject(ctx, rt, env, fmt.Sprintf("unsupported %s version %s (handler is v%s)", env.EventType, env.EventVersion, h.major))
	}

	var decodeErr, handlerErr error
	duplicate := false
	err := pgx.BeginFunc(ctx, rt.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO processed_events (consumer, event_id, event_type, outcome)
			VALUES ($1, $2, $3, 'applied') ON CONFLICT DO NOTHING`, c.group, env.EventID, env.EventType)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			duplicate = true
			return nil
		}
		decodeErr, handlerErr = h.safeApply(ctx, tx, env)
		if decodeErr != nil {
			return decodeErr
		}
		return handlerErr
	})
	switch {
	case duplicate && err == nil:
		return Duplicate, ""
	case decodeErr != nil:
		return c.reject(ctx, rt, env, decodeErr.Error())
	case err == nil:
		return Applied, ""
	}
	var r rejection
	if errors.As(handlerErr, &r) {
		return c.reject(ctx, rt, env, r.reason)
	}
	return Retryable, err.Error()
}

// reject records env as rejected with reason in its own transaction; if
// that write fails the message is retryable instead.
func (c *Consumer) reject(ctx context.Context, rt *runtime, env eventsv1.Envelope, reason string) (Outcome, string) {
	tag, err := rt.pool.Exec(ctx, `
		INSERT INTO processed_events (consumer, event_id, event_type, outcome, reason)
		VALUES ($1, $2, $3, 'rejected', $4) ON CONFLICT DO NOTHING`, c.group, env.EventID, env.EventType, reason)
	if err != nil {
		return Retryable, fmt.Sprintf("record rejection: %v", err)
	}
	if tag.RowsAffected() == 0 {
		return Duplicate, ""
	}
	return Rejected, reason
}

// headerCarrier lets the propagator read Kafka headers.
type headerCarrier []kgo.RecordHeader

func (h headerCarrier) Get(key string) string {
	for _, x := range h {
		if x.Key == key {
			return string(x.Value)
		}
	}
	return ""
}
func (h headerCarrier) Set(string, string) {}
func (h headerCarrier) Keys() []string {
	keys := make([]string, len(h))
	for i, x := range h {
		keys[i] = x.Key
	}
	return keys
}
