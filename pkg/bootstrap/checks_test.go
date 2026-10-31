package bootstrap_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/bootstrap"
)

func TestOpenDBRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	if _, err := bootstrap.OpenDB(context.Background()); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("OpenDB error = %v, want one naming DATABASE_URL", err)
	}
}

func TestDBCheckFailsWhenDatabaseUnreachable(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x:y@127.0.0.1:1/z?sslmode=disable")
	pool, err := bootstrap.OpenDB(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := bootstrap.DBCheck(pool).Fn(ctx); err == nil {
		t.Error("DBCheck against unreachable database = nil, want error")
	}
}

func TestDBCheckPassesWhenDatabaseReachable(t *testing.T) {
	dsn := os.Getenv("DBMIGRATE_TEST_DSN")
	if dsn == "" {
		t.Skip("DBMIGRATE_TEST_DSN not set")
	}
	t.Setenv("DATABASE_URL", dsn)
	pool, err := bootstrap.OpenDB(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := bootstrap.DBCheck(pool).Fn(context.Background()); err != nil {
		t.Errorf("DBCheck = %v, want nil", err)
	}
}

func TestKafkaCheckRequiresBrokers(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "")

	if _, _, err := bootstrap.KafkaCheck(); err == nil || !strings.Contains(err.Error(), "KAFKA_BROKERS") {
		t.Fatalf("KafkaCheck error = %v, want one naming KAFKA_BROKERS", err)
	}
}

func TestKafkaCheckFailsWhenBrokerUnreachable(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "127.0.0.1:1")
	check, closeClient, err := bootstrap.KafkaCheck()
	if err != nil {
		t.Fatal(err)
	}
	defer closeClient()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := check.Fn(ctx); err == nil {
		t.Error("KafkaCheck against unreachable broker = nil, want error")
	}
}

func TestKafkaCheckRejectsBrokerListWithNoEntries(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", " , ")

	if _, _, err := bootstrap.KafkaCheck(); err == nil || !strings.Contains(err.Error(), "KAFKA_BROKERS") {
		t.Fatalf("KafkaCheck error = %v, want one naming KAFKA_BROKERS", err)
	}
}
