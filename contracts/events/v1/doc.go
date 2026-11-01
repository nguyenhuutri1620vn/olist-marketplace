// Package eventsv1 is the only definition of event and command types
// published on Kafka (AD-9): the envelope, the topics and a closed catalog of
// payload types. Each story adds the types it needs to the catalog; JSON
// Schema for the envelope and every catalog type is generated from the Go
// structs into schema/ (go generate).
package eventsv1

//go:generate go run ./internal/schemagen/cmd -out schema
