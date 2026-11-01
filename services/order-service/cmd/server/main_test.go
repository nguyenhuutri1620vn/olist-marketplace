package main

import "testing"

// order-service publishes through the outbox from Story 2.3 on; the relay
// runs now so the stack exercises it.
func TestOrderServiceRunsTheOutboxRelay(t *testing.T) {
	s := orderService()
	if !s.Outbox || !s.UsesKafka || s.Migrations == nil {
		t.Errorf("order-service: Outbox %v, UsesKafka %v, Migrations set %v", s.Outbox, s.UsesKafka, s.Migrations != nil)
	}
}
