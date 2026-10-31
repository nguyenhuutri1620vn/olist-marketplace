// Package bootstrap holds the startup steps every data-owning service runs
// before serving.
package bootstrap

import (
	"context"
	"io/fs"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/config"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/dbmigrate"
)

// Migrate applies the service's embedded migrations to the database named by
// the DATABASE_URL environment variable.
func Migrate(ctx context.Context, migrations fs.FS) error {
	cfg, err := config.Require("DATABASE_URL")
	if err != nil {
		return err
	}
	return dbmigrate.Up(ctx, cfg["DATABASE_URL"], migrations)
}
