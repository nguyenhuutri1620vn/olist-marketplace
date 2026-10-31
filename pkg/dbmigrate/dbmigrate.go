// Package dbmigrate applies a service's embedded SQL migrations at startup
// with golang-migrate (AD-2).
package dbmigrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// Up applies every pending migration in the root of fsys to the database at dsn.
// Non-.sql files are ignored, so an fsys with no .sql files is valid and leaves
// the database unchanged. A .sql file that is not named like a migration
// (NNNNNN_name.up.sql / .down.sql) is an error rather than silently skipped.
func Up(ctx context.Context, dsn string, fsys fs.FS) error {
	if err := checkNames(fsys); err != nil {
		return err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	src, err := iofs.New(fsys, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("init migrate driver: %w", err)
	}
	if _, err := src.First(); errors.Is(err, fs.ErrNotExist) {
		return nil // no migrations yet
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func checkNames(fsys fs.FS) error {
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	for _, n := range names {
		if _, err := source.DefaultParse(n); err != nil {
			return fmt.Errorf("migration file %s: name must look like 000001_name.up.sql or .down.sql", n)
		}
	}
	return nil
}
