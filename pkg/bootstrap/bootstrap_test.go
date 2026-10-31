package bootstrap_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/bootstrap"
)

func TestMigrateRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	err := bootstrap.Migrate(context.Background(), fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("Migrate error = %v, want one naming DATABASE_URL", err)
	}
}

func TestMigrateAppliesToDatabaseURL(t *testing.T) {
	dsn := os.Getenv("DBMIGRATE_TEST_DSN")
	if dsn == "" {
		t.Skip("DBMIGRATE_TEST_DSN not set")
	}
	t.Setenv("DATABASE_URL", dsn)

	if err := bootstrap.Migrate(context.Background(), fstest.MapFS{}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
}
