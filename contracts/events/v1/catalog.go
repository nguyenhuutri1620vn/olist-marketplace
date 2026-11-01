package eventsv1

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sync"
	"testing"
)

// Event is a payload type in the catalog: a Go struct whose JSON is the
// envelope's payload.
type Event interface {
	EventType() string
	// EventVersion is "MAJOR.MINOR": adding a field bumps the minor version,
	// renaming or removing one needs a new major version (AD-9).
	EventVersion() string
	Topic() string
	// AggregateID is the order_id the event is about.
	AggregateID() string
}

// ErrUnknownType marks an event that is not in the catalog.
var ErrUnknownType = errors.New("event type not in contracts/events/v1")

// catalog lists every publishable type. Later stories add theirs here.
var catalog = []Event{}

var (
	mu        sync.RWMutex
	testTypes = map[string]reflect.Type{}
)

var version = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// Catalog returns the publishable types.
func Catalog() []Event {
	return slices.Clone(catalog)
}

// Lookup returns the Go type registered for eventType.
func Lookup(eventType string) (reflect.Type, bool) {
	for _, ev := range catalog {
		if ev.EventType() == eventType {
			return reflect.TypeOf(ev), true
		}
	}
	mu.RLock()
	defer mu.RUnlock()
	typ, ok := testTypes[eventType]
	return typ, ok
}

// Check returns ErrUnknownType unless ev's Go type is the one the catalog
// lists for its event type.
func Check(ev Event) error {
	if typ, ok := Lookup(ev.EventType()); !ok || typ != reflect.TypeOf(ev) {
		return fmt.Errorf("%w: %T (%q)", ErrUnknownType, ev, ev.EventType())
	}
	return nil
}

// Validate checks an event type's metadata: a struct (its JSON is the
// payload object), a name, a MAJOR.MINOR version and a known topic.
func Validate(ev Event) error {
	switch {
	case reflect.TypeOf(ev).Kind() != reflect.Struct:
		return fmt.Errorf("%T is not a struct", ev)
	case ev.EventType() == "":
		return fmt.Errorf("%T has no event type", ev)
	case !version.MatchString(ev.EventVersion()):
		return fmt.Errorf("%T version %q is not MAJOR.MINOR", ev, ev.EventVersion())
	case !slices.Contains(Topics, ev.Topic()):
		return fmt.Errorf("%T topic %q is not one of %v", ev, ev.Topic(), Topics)
	}
	return nil
}

// RegisterForTest adds ev's type to the catalog until t ends, so shared
// packages can be tested before real types exist. It panics outside tests.
func RegisterForTest(t testing.TB, ev Event) {
	if !testing.Testing() {
		panic("eventsv1.RegisterForTest called outside a test")
	}
	if err := Validate(ev); err != nil {
		t.Fatal(err)
	}
	for _, real := range catalog {
		if real.EventType() == ev.EventType() {
			t.Fatalf("RegisterForTest(%T): %s is already in the catalog", ev, ev.EventType())
		}
	}
	name := ev.EventType()
	mu.Lock()
	prev, had := testTypes[name]
	testTypes[name] = reflect.TypeOf(ev)
	mu.Unlock()
	// Restore what was there, so nested registrations keep the outer one.
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if had {
			testTypes[name] = prev
		} else {
			delete(testTypes, name)
		}
	})
}
