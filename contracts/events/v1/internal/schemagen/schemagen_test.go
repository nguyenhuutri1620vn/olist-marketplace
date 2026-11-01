package schemagen

import (
	"encoding/json"
	"slices"
	"testing"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
)

type probe struct {
	OrderID string  `json:"order_id"`
	Note    *string `json:"note,omitempty"`
}

func (probe) EventType() string     { return "ProbeRecorded" }
func (probe) EventVersion() string  { return "2.3" }
func (probe) Topic() string         { return eventsv1.TopicOrderEvents }
func (p probe) AggregateID() string { return p.OrderID }

type badProbe struct{ probe }

func (badProbe) Topic() string { return "nowhere" }

func TestEachTypeGetsASchemaNamedByTypeAndMajorVersion(t *testing.T) {
	files, err := generate([]eventsv1.Event{probe{}})
	if err != nil {
		t.Fatal(err)
	}
	b, ok := files["ProbeRecorded.v2.json"]
	if !ok || len(files) != 2 {
		t.Fatalf("files = %v, want envelope.json and ProbeRecorded.v2.json", keys(files))
	}
	var s struct {
		Title    string   `json:"title"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	// Optional fields are omitempty pointers.
	if s.Title != "ProbeRecorded" || !slices.Equal(s.Required, []string{"order_id"}) {
		t.Errorf("schema title %q required %v", s.Title, s.Required)
	}
}

func TestMalformedTypesFailGeneration(t *testing.T) {
	if _, err := generate([]eventsv1.Event{badProbe{}}); err == nil {
		t.Error("generate accepted a type with an unknown topic")
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestDuplicateTypesFailGeneration(t *testing.T) {
	if _, err := generate([]eventsv1.Event{probe{}, probe{}}); err == nil {
		t.Error("generate accepted the same type twice")
	}
}
