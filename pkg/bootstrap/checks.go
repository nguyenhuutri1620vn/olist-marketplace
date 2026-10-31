package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/config"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/ops"
)

// OpenDB returns a connection pool for DATABASE_URL. It does not connect until
// first use; DBCheck reports whether the database is reachable.
func OpenDB(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := config.Require("DATABASE_URL")
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(ctx, cfg["DATABASE_URL"])
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return pool, nil
}

// DBCheck is the readiness check for the service's database.
func DBCheck(pool *pgxpool.Pool) ops.Check {
	return ops.Check{Name: "database", Fn: pool.Ping}
}

// KafkaCheck returns the readiness check for the brokers in KAFKA_BROKERS
// (comma-separated) and a function that closes its client.
func KafkaCheck() (ops.Check, func(), error) {
	cfg, err := config.Require("KAFKA_BROKERS")
	if err != nil {
		return ops.Check{}, nil, err
	}
	var brokers []string
	for _, b := range strings.Split(cfg["KAFKA_BROKERS"], ",") {
		if b = strings.TrimSpace(b); b != "" {
			brokers = append(brokers, b)
		}
	}
	if len(brokers) == 0 {
		return ops.Check{}, nil, fmt.Errorf("KAFKA_BROKERS lists no brokers")
	}
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		return ops.Check{}, nil, fmt.Errorf("create kafka client: %w", err)
	}
	return ops.Check{Name: "kafka", Fn: client.Ping}, client.Close, nil
}
