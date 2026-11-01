// Package eventbus turns catalog events (contracts/events/v1) into envelopes
// and Kafka records (AD-8, AD-9). Use cases never publish: they hand the
// Message to pkg/outbox, whose relay calls Record and the Producer.
package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
)

// TraceparentHeader is the Kafka header carrying W3C trace context.
const TraceparentHeader = "traceparent"

// Meta is what the writer knows about an event beyond its payload.
type Meta struct {
	// OccurredAt is business time (required): the dataset timestamp for
	// Historical events.
	OccurredAt time.Time
	Historical bool
	// EventID reuses an incoming id when republishing; empty means a new UUIDv7.
	EventID string
	// OriginPublishedAt is copied from the incoming event when republishing a
	// fact derived from it; zero means a first publish.
	OriginPublishedAt time.Time
}

// Message is an envelope waiting for the relay, which sets PublishedAt.
type Message struct {
	Topic    string
	Envelope eventsv1.Envelope
}

// New builds the Message for a catalog event, taking traceparent from the
// span in ctx. Types outside the catalog are eventsv1.ErrUnknownType.
func New(ctx context.Context, ev eventsv1.Event, meta Meta) (Message, error) {
	if err := eventsv1.Check(ev); err != nil {
		return Message{}, err
	}
	if meta.OccurredAt.IsZero() {
		return Message{}, fmt.Errorf("%s: occurred_at is required", ev.EventType())
	}
	if ev.AggregateID() == "" {
		return Message{}, fmt.Errorf("%s: aggregate id is empty", ev.EventType())
	}
	id := meta.EventID
	if id == "" {
		v7, err := uuid.NewV7()
		if err != nil {
			return Message{}, fmt.Errorf("new event id: %w", err)
		}
		id = v7.String()
	} else if u, err := uuid.Parse(id); err != nil {
		return Message{}, fmt.Errorf("%s: event id %q is not a uuid", ev.EventType(), id)
	} else {
		id = u.String()
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return Message{}, fmt.Errorf("%s payload: %w", ev.EventType(), err)
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return Message{
		Topic: ev.Topic(),
		Envelope: eventsv1.Envelope{
			EventID:           id,
			EventType:         ev.EventType(),
			EventVersion:      ev.EventVersion(),
			AggregateID:       ev.AggregateID(),
			Historical:        meta.Historical,
			OccurredAt:        meta.OccurredAt.UTC(),
			OriginPublishedAt: meta.OriginPublishedAt.UTC(),
			Traceparent:       carrier.Get(TraceparentHeader),
			Payload:           payload,
		},
	}, nil
}

// Record is the Kafka record for msg published at publishedAt: keyed by the
// aggregate (order_id), with the traceparent header. On a first publish
// origin_published_at becomes publishedAt.
func Record(msg Message, publishedAt time.Time) (*kgo.Record, error) {
	e := msg.Envelope
	e.PublishedAt = publishedAt.UTC()
	if e.OriginPublishedAt.IsZero() {
		e.OriginPublishedAt = e.PublishedAt
	}
	value, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("encode envelope %s: %w", e.EventID, err)
	}
	rec := &kgo.Record{Topic: msg.Topic, Key: []byte(e.AggregateID), Value: value}
	if e.Traceparent != "" {
		rec.Headers = []kgo.RecordHeader{{Key: TraceparentHeader, Value: []byte(e.Traceparent)}}
	}
	return rec, nil
}

// NewProducer returns the idempotent producer the relay uses: acks=all, so
// franz-go's own retries neither duplicate nor reorder records of one
// partition (the relay may still resend a batch: see pkg/outbox). Topics are
// created on first use.
func NewProducer(brokers []string) (*kgo.Client, error) {
	if len(brokers) == 0 {
		return nil, errors.New("no kafka brokers")
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}
	return client, nil
}
