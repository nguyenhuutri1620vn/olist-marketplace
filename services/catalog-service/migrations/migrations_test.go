package migrations_test

import (
	"io/fs"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/migrations"
)

func TestFSEmbedsTheCatalogMigrations(t *testing.T) {
	for _, name := range []string{
		"000001_catalog.up.sql", "000001_catalog.down.sql",
		"000002_product_list.up.sql", "000002_product_list.down.sql",
	} {
		if _, err := fs.Stat(migrations.FS, name); err != nil {
			t.Errorf("embedded %s: %v", name, err)
		}
	}
}
