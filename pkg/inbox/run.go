package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
)

// commitTimeout bounds an offset commit, which runs even while stopping.
const commitTimeout = 5 * time.Second

// Deps are what a running consumer needs from its service.
type Deps struct {
	Pool        *pgxpool.Pool
	Brokers     []string
	Log         *slog.Logger
	Metrics     *Metrics
	MaxAttempts int
}

// Run consumes until ctx ends: one record at a time, its offset committed
// only after it reached a final outcome. A new group starts at the earliest
// offset, so nothing published before its first start is missed.
func (c *Consumer) Run(ctx context.Context, d Deps) error {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(d.Brokers...),
		kgo.ConsumerGroup(c.group),
		kgo.ConsumeTopics(c.topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
		// No rebalance between processing a record and committing it.
		kgo.BlockRebalanceOnPoll(),
	)
	if err != nil {
		return fmt.Errorf("consumer %s: %w", c.group, err)
	}
	defer cl.Close()
	dlq, err := eventbus.NewProducer(d.Brokers)
	if err != nil {
		return err
	}
	defer dlq.Close()
	rt := &runtime{pool: d.Pool, log: d.Log, metrics: d.Metrics, maxAttempts: d.MaxAttempts, dlq: dlq, sleep: sleep}

	for {
		// One record per poll: a rebalance waits for at most one record's
		// retries, never a whole batch's.
		fetches := cl.PollRecords(ctx, 1)
		if ctx.Err() != nil {
			cl.AllowRebalance()
			return nil
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			d.Log.ErrorContext(ctx, "fetch failed", "consumer", c.group, "topic", topic, "partition", partition, "err", err)
		})
		for _, rec := range fetches.Records() {
			if _, ok := c.process(ctx, rt, rec); !ok {
				cl.AllowRebalance()
				return nil // stopped mid-retry: not committed, redelivered later
			}
			cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), commitTimeout)
			if err := cl.CommitRecords(cctx, rec); err != nil {
				// The record comes again after a restart and is then a duplicate.
				d.Log.ErrorContext(ctx, "commit failed", "consumer", c.group, "topic", rec.Topic, "err", err)
			}
			cancel()
		}
		cl.AllowRebalance()
	}
}
