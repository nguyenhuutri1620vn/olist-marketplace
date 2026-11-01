package eventsv1

import (
	"encoding/json"
	"time"
)

// Topics (AD-8): facts on <producer>.events, commands on <target>.commands,
// Replay input on replay.orders. A topic carries one direction only.
const (
	TopicOrderEvents     = "order.events"
	TopicPaymentEvents   = "payment.events"
	TopicShippingEvents  = "shipping.events"
	TopicPaymentCommands = "payment.commands"
	TopicReplayOrders    = "replay.orders"
)

// Topics lists every topic an event may be published on.
var Topics = []string{TopicOrderEvents, TopicPaymentEvents, TopicShippingEvents, TopicPaymentCommands, TopicReplayOrders}

// Envelope wraps every message on Kafka (AD-8).
type Envelope struct {
	// EventID is a uuid; republishing a derived fact may reuse the incoming one.
	EventID      string `json:"event_id" jsonschema:"format=uuid"`
	EventType    string `json:"event_type" jsonschema:"minLength=1"`
	EventVersion string `json:"event_version" jsonschema:"pattern=^[0-9]+\\.[0-9]+$"`
	// AggregateID is the order_id, also the Kafka partition key.
	AggregateID string `json:"aggregate_id" jsonschema:"minLength=1"`
	Historical  bool   `json:"historical"`
	// OccurredAt is business time: the dataset timestamp for Historical events.
	OccurredAt time.Time `json:"occurred_at"`
	// PublishedAt is set by the outbox relay.
	PublishedAt time.Time `json:"published_at"`
	// OriginPublishedAt equals PublishedAt on a first publish and is copied
	// unchanged when a fact derived from an incoming event is republished (AD-17).
	OriginPublishedAt time.Time `json:"origin_published_at"`
	// Traceparent is W3C trace context, also sent as a Kafka header.
	Traceparent string          `json:"traceparent"`
	Payload     json.RawMessage `json:"payload" jsonschema:"type=object"`
}
