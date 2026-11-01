package eventsv1_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
)

// The envelope's JSON fields are fixed by AD-8, in this order.
func TestEnvelopeJSONFields(t *testing.T) {
	ts := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	b, err := json.Marshal(eventsv1.Envelope{
		EventID: "e", EventType: "T", EventVersion: "1.0", AggregateID: "a", Historical: true,
		OccurredAt: ts, PublishedAt: ts, OriginPublishedAt: ts, Traceparent: "tp", Payload: json.RawMessage(`{"x":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"event_id":"e","event_type":"T","event_version":"1.0","aggregate_id":"a","historical":true,` +
		`"occurred_at":"2026-11-01T12:00:00Z","published_at":"2026-11-01T12:00:00Z","origin_published_at":"2026-11-01T12:00:00Z",` +
		`"traceparent":"tp","payload":{"x":1}}`
	if string(b) != want {
		t.Errorf("envelope JSON\n got %s\nwant %s", b, want)
	}
}

func TestTopicsAreTheAD8Set(t *testing.T) {
	want := []string{"order.events", "payment.events", "shipping.events", "payment.commands", "replay.orders"}
	if !slices.Equal(eventsv1.Topics, want) {
		t.Errorf("Topics = %v, want %v", eventsv1.Topics, want)
	}
}
