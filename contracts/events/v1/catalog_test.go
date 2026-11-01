package eventsv1_test

import (
	"errors"
	"reflect"
	"testing"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
)

type probe struct {
	OrderID string `json:"order_id"`
}

func (probe) EventType() string     { return "ProbeRecorded" }
func (probe) EventVersion() string  { return "1.0" }
func (probe) Topic() string         { return eventsv1.TopicOrderEvents }
func (p probe) AggregateID() string { return p.OrderID }

// impostor claims a registered type's name with a different Go type.
type impostor struct{ probe }

func TestUnknownTypesAreRejected(t *testing.T) {
	if err := eventsv1.Check(probe{}); !errors.Is(err, eventsv1.ErrUnknownType) {
		t.Errorf("Check(unregistered) = %v, want ErrUnknownType", err)
	}
	if _, ok := eventsv1.Lookup("ProbeRecorded"); ok {
		t.Error("Lookup found an unregistered type")
	}
}

func TestRegisterForTestAddsATypeForOneTest(t *testing.T) {
	t.Run("registered", func(t *testing.T) {
		eventsv1.RegisterForTest(t, probe{})
		if err := eventsv1.Check(probe{OrderID: "o"}); err != nil {
			t.Errorf("Check(registered) = %v", err)
		}
		if typ, ok := eventsv1.Lookup("ProbeRecorded"); !ok || typ != reflect.TypeFor[probe]() {
			t.Errorf("Lookup = %v, %v", typ, ok)
		}
		// Same event_type, different Go type: not the catalog's type.
		if err := eventsv1.Check(impostor{}); !errors.Is(err, eventsv1.ErrUnknownType) {
			t.Errorf("Check(impostor) = %v, want ErrUnknownType", err)
		}
	})
	if _, ok := eventsv1.Lookup("ProbeRecorded"); ok {
		t.Error("test type still registered after its test")
	}
}

func TestCatalogTypesAreWellFormed(t *testing.T) {
	for _, ev := range eventsv1.Catalog() {
		if err := eventsv1.Validate(ev); err != nil {
			t.Errorf("%T: %v", ev, err)
		}
	}
}

func TestValidateChecksVersionAndTopic(t *testing.T) {
	for _, ev := range []eventsv1.Event{badVersion{}, badTopic{}, emptyType{}} {
		if err := eventsv1.Validate(ev); err == nil {
			t.Errorf("Validate(%T) = nil, want an error", ev)
		}
	}
	if err := eventsv1.Validate(probe{}); err != nil {
		t.Errorf("Validate(probe) = %v", err)
	}
}

type badVersion struct{ probe }

func (badVersion) EventVersion() string { return "1" }

type badTopic struct{ probe }

func (badTopic) Topic() string { return "orders" }

type emptyType struct{ probe }

func (emptyType) EventType() string { return "" }

func TestNestedRegistrationsRestoreTheOuterOne(t *testing.T) {
	eventsv1.RegisterForTest(t, probe{})
	t.Run("inner", func(t *testing.T) { eventsv1.RegisterForTest(t, probe{}) })
	if err := eventsv1.Check(probe{}); err != nil {
		t.Errorf("outer registration lost after the inner test: %v", err)
	}
}

func TestValidateRejectsNonStructTypes(t *testing.T) {
	if err := eventsv1.Validate(probeID("o")); err == nil {
		t.Error("Validate accepted a string type, whose JSON is not an object")
	}
}

type probeID string

func (probeID) EventType() string     { return "ProbeID" }
func (probeID) EventVersion() string  { return "1.0" }
func (probeID) Topic() string         { return eventsv1.TopicOrderEvents }
func (p probeID) AggregateID() string { return string(p) }

func TestCatalogHasOneTypePerName(t *testing.T) {
	seen := map[string]bool{}
	for _, ev := range eventsv1.Catalog() {
		if seen[ev.EventType()] {
			t.Errorf("%s listed twice", ev.EventType())
		}
		seen[ev.EventType()] = true
	}
}
