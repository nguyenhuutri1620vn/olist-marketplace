package migrations_test

import (
	"io/fs"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/outbox"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/migrations"
)

// The outbox migration is pkg/outbox's schema, unchanged.
func TestOutboxMigrationIsTheSharedSchema(t *testing.T) {
	up, err := fs.ReadFile(migrations.FS, "000001_outbox.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if string(up) != outbox.SchemaSQL {
		t.Error("000001_outbox.up.sql differs from pkg/outbox/schema.sql; copy it again")
	}
	if _, err := fs.ReadFile(migrations.FS, "000001_outbox.down.sql"); err != nil {
		t.Error(err)
	}
}
