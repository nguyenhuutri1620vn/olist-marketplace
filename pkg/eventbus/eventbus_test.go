package eventbus_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
)

type probe struct {
	OrderID string `json:"order_id"`
	Total   int64  `json:"total"`
}

func (probe) EventType() string     { return "ProbeRecorded" }
func (probe) EventVersion() string  { return "1.2" }
func (probe) Topic() string         { return eventsv1.TopicOrderEvents }
func (p probe) AggregateID() string { return p.OrderID }

const orderID = "0192f000-0000-7000-8000-000000000001"

var occurred = time.Date(2017, 10, 2, 10, 56, 33, 0, time.UTC)

// traced returns a context inside a sampled span and its traceparent.
func traced(t *testing.T) (context.Context, string) {
	t.Helper()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { tp.Shutdown(context.Background()) })
	ctx, span := tp.Tracer("test").Start(context.Background(), "use case")
	t.Cleanup(func() { span.End() })
	sc := span.SpanContext()
	return ctx, "00-" + sc.TraceID().String() + "-" + sc.SpanID().String() + "-01"
}

func TestNewBuildsTheEnvelope(t *testing.T) {
	eventsv1.RegisterForTest(t, probe{})
	ctx, tp := traced(t)
	msg, err := eventbus.New(ctx, probe{OrderID: orderID, Total: 12990}, eventbus.Meta{OccurredAt: occurred, Historical: true})
	if err != nil {
		t.Fatal(err)
	}
	e := msg.Envelope
	id, err := uuid.Parse(e.EventID)
	if err != nil || id.Version() != 7 {
		t.Errorf("event_id %q is not a UUIDv7", e.EventID)
	}
	if e.EventType != "ProbeRecorded" || e.EventVersion != "1.2" || e.AggregateID != orderID || !e.Historical ||
		!e.OccurredAt.Equal(occurred) || !e.PublishedAt.IsZero() || !e.OriginPublishedAt.IsZero() || e.Traceparent != tp {
		t.Errorf("envelope = %+v (traceparent want %s)", e, tp)
	}
	if string(e.Payload) != `{"order_id":"`+orderID+`","total":12990}` || msg.Topic != eventsv1.TopicOrderEvents {
		t.Errorf("payload %s topic %s", e.Payload, msg.Topic)
	}
}

func TestNewKeepsAGivenEventIDAndOrigin(t *testing.T) {
	eventsv1.RegisterForTest(t, probe{})
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("replay")).String()
	origin := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	msg, err := eventbus.New(context.Background(), probe{OrderID: orderID}, eventbus.Meta{OccurredAt: occurred, EventID: id, OriginPublishedAt: origin})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Envelope.EventID != id || !msg.Envelope.OriginPublishedAt.Equal(origin) || msg.Envelope.Traceparent != "" {
		t.Errorf("envelope = %+v", msg.Envelope)
	}
}

func TestNewRejects(t *testing.T) {
	eventsv1.RegisterForTest(t, probe{})
	ctx := context.Background()
	cases := map[string]struct {
		ev   eventsv1.Event
		meta eventbus.Meta
	}{
		"type not in the catalog": {unlisted{OrderID: orderID}, eventbus.Meta{OccurredAt: occurred}},
		"no occurred_at":          {probe{OrderID: orderID}, eventbus.Meta{}},
		"bad event_id":            {probe{OrderID: orderID}, eventbus.Meta{OccurredAt: occurred, EventID: "x"}},
		"no aggregate id":         {probe{}, eventbus.Meta{OccurredAt: occurred}},
	}
	for name, tc := range cases {
		if _, err := eventbus.New(ctx, tc.ev, tc.meta); err == nil {
			t.Errorf("%s: New accepted it", name)
		}
	}
	if _, err := eventbus.New(ctx, unlisted{OrderID: orderID}, eventbus.Meta{OccurredAt: occurred}); !errors.Is(err, eventsv1.ErrUnknownType) {
		t.Errorf("unlisted type: err = %v, want ErrUnknownType", err)
	}
}

type unlisted struct{ probe }

func (unlisted) EventType() string { return "NotListed" }

func TestRecordStampsPublishTimeAndKeysByAggregate(t *testing.T) {
	eventsv1.RegisterForTest(t, probe{})
	ctx, tp := traced(t)
	msg, err := eventbus.New(ctx, probe{OrderID: orderID}, eventbus.Meta{OccurredAt: occurred})
	if err != nil {
		t.Fatal(err)
	}
	published := time.Date(2026, 11, 1, 10, 0, 0, 123000000, time.UTC)
	rec, err := eventbus.Record(msg, published)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Topic != eventsv1.TopicOrderEvents || string(rec.Key) != orderID {
		t.Errorf("record topic %q key %q", rec.Topic, rec.Key)
	}
	if len(rec.Headers) != 1 || rec.Headers[0].Key != "traceparent" || string(rec.Headers[0].Value) != tp {
		t.Errorf("headers = %+v, want traceparent %s", rec.Headers, tp)
	}
	var e eventsv1.Envelope
	if err := json.Unmarshal(rec.Value, &e); err != nil {
		t.Fatal(err)
	}
	// First publish: origin is the publish time.
	if !e.PublishedAt.Equal(published) || !e.OriginPublishedAt.Equal(published) || e.EventID != msg.Envelope.EventID {
		t.Errorf("published envelope = %+v", e)
	}

	origin := published.Add(-time.Hour)
	msg.Envelope.OriginPublishedAt = origin
	rec, _ = eventbus.Record(msg, published)
	json.Unmarshal(rec.Value, &e)
	if !e.OriginPublishedAt.Equal(origin) {
		t.Errorf("republished origin = %v, want %v", e.OriginPublishedAt, origin)
	}
}

func TestRecordWithoutTraceHasNoHeader(t *testing.T) {
	eventsv1.RegisterForTest(t, probe{})
	msg, _ := eventbus.New(context.Background(), probe{OrderID: orderID}, eventbus.Meta{OccurredAt: occurred})
	rec, err := eventbus.Record(msg, time.Now())
	if err != nil || len(rec.Headers) != 0 {
		t.Errorf("headers = %+v, err %v", rec.Headers, err)
	}
}

func TestProducerIsIdempotentWithAcksAll(t *testing.T) {
	p, err := eventbus.NewProducer([]string{"127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if acks := p.OptValue(kgo.RequiredAcks); acks != kgo.AllISRAcks() {
		t.Errorf("acks = %v, want all", acks)
	}
	if p.OptValue(kgo.DisableIdempotentWrite) == true {
		t.Error("idempotent writes disabled")
	}
	if _, err := eventbus.NewProducer(nil); err == nil {
		t.Error("NewProducer accepted no brokers")
	}
}

// The envelope carries the canonical form, as the outbox's uuid column does.
func TestNewCanonicalisesAGivenEventID(t *testing.T) {
	eventsv1.RegisterForTest(t, probe{})
	msg, err := eventbus.New(context.Background(), probe{OrderID: orderID}, eventbus.Meta{OccurredAt: occurred, EventID: "{0192F000-0000-7000-8000-0000000000AB}"})
	if err != nil || msg.Envelope.EventID != "0192f000-0000-7000-8000-0000000000ab" {
		t.Errorf("event_id = %q, %v", msg.Envelope.EventID, err)
	}
}
