package eventsv1_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1/internal/schemagen"
)

// The committed schemas must be what go generate writes from the Go structs.
func TestSchemasAreUpToDate(t *testing.T) {
	want, err := schemagen.Generate()
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("schema/*.json")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	var wantNames []string
	for name := range want {
		wantNames = append(wantNames, name)
	}
	slices.Sort(names)
	slices.Sort(wantNames)
	if !slices.Equal(names, wantNames) {
		t.Fatalf("schema files %v, want %v (run go generate ./... in contracts)", names, wantNames)
	}
	for name, b := range want {
		got, err := os.ReadFile(filepath.Join("schema", name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(b) {
			t.Errorf("schema/%s is stale (run go generate ./... in contracts)", name)
		}
	}
}

func TestEnvelopeSchemaConstrainsIDs(t *testing.T) {
	all, err := schemagen.Generate()
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Properties map[string]struct {
			Format    string `json:"format"`
			MinLength *int   `json:"minLength"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(all["envelope.json"], &s); err != nil {
		t.Fatal(err)
	}
	if s.Properties["event_id"].Format != "uuid" {
		t.Error("event_id is not format uuid")
	}
	for _, f := range []string{"event_type", "aggregate_id"} {
		if m := s.Properties[f].MinLength; m == nil || *m != 1 {
			t.Errorf("%s has no minLength 1", f)
		}
	}
}

func TestEnvelopeSchemaRequiresEveryField(t *testing.T) {
	all, err := schemagen.Generate()
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Required             []string `json:"required"`
		AdditionalProperties *bool    `json:"additionalProperties"`
	}
	if err := json.Unmarshal(all["envelope.json"], &s); err != nil {
		t.Fatal(err)
	}
	want := []string{"event_id", "event_type", "event_version", "aggregate_id", "historical", "occurred_at",
		"published_at", "origin_published_at", "traceparent", "payload"}
	if !slices.Equal(s.Required, want) || s.AdditionalProperties == nil || *s.AdditionalProperties {
		t.Errorf("envelope schema required %v additionalProperties %v, want %v and false", s.Required, s.AdditionalProperties, want)
	}
}
