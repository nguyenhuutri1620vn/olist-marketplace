// Package schemagen derives the JSON Schema of the envelope and of every
// catalog type from the Go structs, so the schemas cannot drift from code.
package schemagen

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/invopop/jsonschema"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
)

// Generate returns the schema files by name: envelope.json and
// <EventType>.v<major>.json for each catalog type.
func Generate() (map[string][]byte, error) {
	return generate(eventsv1.Catalog())
}

func generate(types []eventsv1.Event) (map[string][]byte, error) {
	out := map[string][]byte{}
	b, err := schema(&eventsv1.Envelope{}, "Envelope")
	if err != nil {
		return nil, err
	}
	out["envelope.json"] = b
	for _, ev := range types {
		if err := eventsv1.Validate(ev); err != nil {
			return nil, err
		}
		major, _, _ := strings.Cut(ev.EventVersion(), ".")
		name := fmt.Sprintf("%s.v%s.json", ev.EventType(), major)
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("%s is listed twice", name)
		}
		if out[name], err = schema(ev, ev.EventType()); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func schema(v any, title string) ([]byte, error) {
	r := &jsonschema.Reflector{ExpandedStruct: true, DoNotReference: true}
	s := r.Reflect(v)
	s.Title = title
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("schema for %s: %w", title, err)
	}
	return append(b, '\n'), nil
}
