package inbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
)

// DLQ naming, the headers a parked record carries, and the group whose
// committed offsets mark what was redriven.
const (
	DLQSuffix         = ".dlq"
	headerDLQReason   = "dlq-reason"
	headerDLQAttempts = "dlq-attempts"
	headerDLQConsumer = "dlq-consumer"
	RedriveGroup      = "dlq-redrive"
)

// dlqRetry is how soon WatchDLQDepth retries a failed poll.
const dlqRetry = time.Second

// redrivePoll bounds one wait for DLQ records; the caller's ctx bounds the
// whole redrive.
const redrivePoll = 10 * time.Second

func admin(brokers []string) (*kadm.Client, *kgo.Client, error) {
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		return nil, nil, fmt.Errorf("kafka admin client: %w", err)
	}
	return kadm.NewClient(cl), cl, nil
}

// depths returns, per partition of the dlq topics, the records not yet
// redriven: end offset − max(redrive group's commit, start offset). DLQ
// topics are never truncated by redrive, so the commit marks progress.
func depths(ctx context.Context, adm *kadm.Client, dlqs ...string) (map[string]int64, error) {
	out := map[string]int64{}
	ends, err := adm.ListEndOffsets(ctx, dlqs...)
	if err != nil {
		return nil, fmt.Errorf("list DLQ end offsets: %w", err)
	}
	starts, err := adm.ListStartOffsets(ctx, dlqs...)
	if err != nil {
		return nil, fmt.Errorf("list DLQ start offsets: %w", err)
	}
	committed, err := adm.FetchOffsets(ctx, RedriveGroup)
	if err != nil {
		return nil, fmt.Errorf("fetch %s offsets: %w", RedriveGroup, err)
	}
	var errs []error
	ends.Each(func(o kadm.ListedOffset) {
		if errors.Is(o.Err, kerr.UnknownTopicOrPartition) {
			return // nothing was ever parked
		}
		if o.Err != nil {
			errs = append(errs, o.Err)
			return
		}
		from := int64(0)
		if s, ok := starts.Lookup(o.Topic, o.Partition); ok && s.Err == nil {
			from = s.Offset
		}
		if c, ok := committed.Lookup(o.Topic, o.Partition); ok && c.Err == nil && c.At > from {
			from = c.At
		}
		out[o.Topic] += max(o.Offset-from, 0)
	})
	return out, errors.Join(errs...)
}

// DLQDepth is the number of records on topic's DLQ not yet redriven.
func DLQDepth(ctx context.Context, brokers []string, topic string) (int64, error) {
	adm, cl, err := admin(brokers)
	if err != nil {
		return 0, err
	}
	defer cl.Close()
	d, err := depths(ctx, adm, topic+DLQSuffix)
	return d[topic+DLQSuffix], err
}

// NonEmptyDLQs lists every *.dlq topic with records not yet redriven.
func NonEmptyDLQs(ctx context.Context, brokers []string) (map[string]int64, error) {
	adm, cl, err := admin(brokers)
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	topics, err := adm.ListTopics(ctx)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	var dlqs []string
	for _, name := range topics.Names() {
		if strings.HasSuffix(name, DLQSuffix) {
			dlqs = append(dlqs, name)
		}
	}
	if len(dlqs) == 0 {
		return map[string]int64{}, nil
	}
	all, err := depths(ctx, adm, dlqs...)
	for topic, d := range all {
		if d == 0 {
			delete(all, topic)
		}
	}
	return all, err
}

// WatchDLQDepth refreshes inbox_dlq_depth for topics every interval until
// ctx ends.
func WatchDLQDepth(ctx context.Context, brokers, topics []string, m *Metrics, log *slog.Logger, every time.Duration) {
	adm, cl, err := admin(brokers)
	if err != nil {
		log.ErrorContext(ctx, "DLQ depth watcher", "err", err)
		return
	}
	defer cl.Close()
	dlqs := make([]string, len(topics))
	for i, t := range topics {
		dlqs[i] = t + DLQSuffix
	}
	for {
		d, err := depths(ctx, adm, dlqs...)
		if err != nil && ctx.Err() == nil {
			log.WarnContext(ctx, "DLQ depth", "err", err)
		}
		// A failing partition leaves only its own topic's gauge stale.
		for i, topic := range topics {
			if v, ok := d[dlqs[i]]; ok || err == nil {
				m.dlqDepth.WithLabelValues(topic).Set(float64(v))
			}
		}
		// A failed poll (a broker still electing its group coordinator, say)
		// is retried soon, so the gauge is not missing for a whole interval.
		wait := every
		if err != nil {
			wait = min(every, dlqRetry)
		}
		if !sleep(ctx, wait) {
			return
		}
	}
}

// Redrive copies every record on topic's DLQ that was not yet redriven back
// to topic (without the dlq-* headers), committing the redrive group after
// each batch. Consumers are idempotent, so a record redriven twice is a
// duplicate; but it lands after newer events for the same order, so inspect
// the DLQ (dlq-reason, dlq-consumer) before redriving. It returns how many
// records it copied.
func Redrive(ctx context.Context, brokers []string, topic string) (int, error) {
	dlq := topic + DLQSuffix
	pending, err := DLQDepth(ctx, brokers, topic)
	if err != nil || pending == 0 {
		return 0, err
	}
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(RedriveGroup),
		kgo.ConsumeTopics(dlq),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return 0, fmt.Errorf("redrive consumer: %w", err)
	}
	defer consumer.Close()
	producer, err := eventbus.NewProducer(brokers)
	if err != nil {
		return 0, err
	}
	defer producer.Close()

	copied := 0
	for int64(copied) < pending {
		pctx, cancel := context.WithTimeout(ctx, redrivePoll)
		fetches := consumer.PollFetches(pctx)
		cancel()
		if err := ctx.Err(); err != nil {
			return copied, err
		}
		if errs := fetches.Errors(); len(errs) > 0 && fetches.NumRecords() == 0 {
			return copied, fmt.Errorf("read %s: %w", dlq, errs[0].Err)
		}
		recs := fetches.Records()
		if len(recs) == 0 {
			continue // joining the group can take a while; ctx bounds the wait
		}
		back := make([]*kgo.Record, len(recs))
		for i, r := range recs {
			back[i] = &kgo.Record{Topic: topic, Key: r.Key, Value: r.Value, Headers: withoutDLQHeaders(r.Headers)}
		}
		if err := producer.ProduceSync(ctx, back...).FirstErr(); err != nil {
			return copied, fmt.Errorf("republish to %s: %w", topic, err)
		}
		if err := consumer.CommitRecords(ctx, recs...); err != nil {
			return copied, fmt.Errorf("commit %s: %w", RedriveGroup, err)
		}
		copied += len(recs)
	}
	return copied, nil
}

func withoutDLQHeaders(hs []kgo.RecordHeader) []kgo.RecordHeader {
	var out []kgo.RecordHeader
	for _, h := range hs {
		if h.Key != headerDLQReason && h.Key != headerDLQAttempts && h.Key != headerDLQConsumer {
			out = append(out, h)
		}
	}
	return out
}
