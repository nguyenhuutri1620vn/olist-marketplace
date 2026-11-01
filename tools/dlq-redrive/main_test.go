package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
)

func TestFlagsChooseExactlyOneMode(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "127.0.0.1:1")
	for _, args := range [][]string{nil, {"-topic", "order.events", "-check"}, {"-bogus"}} {
		var out bytes.Buffer
		if code := run(context.Background(), args, &out); code != 2 {
			t.Errorf("run(%v) = %d, want 2 (usage)", args, code)
		}
	}
}

func TestTopicMustNotBeADLQ(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "127.0.0.1:1")
	var out bytes.Buffer
	if code := run(context.Background(), []string{"-topic", "order.events.dlq"}, &out); code != 2 {
		t.Errorf("redriving a DLQ name = %d, want 2", code)
	}
}

// With a real broker: -check fails while a DLQ holds a record, -topic
// redrives it, and -check then passes.
func TestCheckRedriveCheck(t *testing.T) {
	brokers := os.Getenv("OUTBOX_TEST_KAFKA")
	if brokers == "" {
		t.Skip("OUTBOX_TEST_KAFKA not set")
	}
	t.Setenv("KAFKA_BROKERS", brokers)
	topic := "redrive-test-" + uuid.NewString()
	p, err := eventbus.NewProducer(strings.Split(brokers, ","))
	if err != nil {
		t.Fatal(err)
	}
	err = p.ProduceSync(context.Background(), &kgo.Record{Topic: topic + ".dlq", Key: []byte("k"), Value: []byte(`{}`),
		Headers: []kgo.RecordHeader{{Key: "dlq-reason", Value: []byte("boom")}}}).FirstErr()
	p.Close()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var out bytes.Buffer
	if code := run(ctx, []string{"-check"}, &out); code != 1 || !strings.Contains(out.String(), topic+".dlq: 1") {
		t.Fatalf("check before redrive = %d %q, want 1 naming %s.dlq", code, out.String(), topic)
	}
	out.Reset()
	if code := run(ctx, []string{"-topic", topic}, &out); code != 0 || !strings.Contains(out.String(), "redrove 1 ") {
		t.Fatalf("redrive = %d %q", code, out.String())
	}
	// Other runs may leave DLQs on a shared broker: only this topic matters.
	out.Reset()
	run(ctx, []string{"-check"}, &out)
	if strings.Contains(out.String(), topic+".dlq") {
		t.Errorf("check after redrive still lists %s.dlq: %q", topic, out.String())
	}
}
