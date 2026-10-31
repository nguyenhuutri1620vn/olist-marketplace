package migrations_test

import (
	"io/fs"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/migrations"
)

func TestFSEmbedsTheIdentityMigration(t *testing.T) {
	for _, name := range []string{"000001_identity.up.sql", "000001_identity.down.sql"} {
		if _, err := fs.Stat(migrations.FS, name); err != nil {
			t.Errorf("embedded %s: %v", name, err)
		}
	}
}
