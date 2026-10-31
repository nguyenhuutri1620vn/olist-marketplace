package migrations_test

import (
	"io/fs"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/migrations"
)

func TestFSIsReadableWithoutSQLFiles(t *testing.T) {
	if _, err := fs.ReadDir(migrations.FS, "."); err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
}
