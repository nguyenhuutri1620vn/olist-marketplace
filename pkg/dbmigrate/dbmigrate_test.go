package dbmigrate_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/dbmigrate"
)

func TestUpFailsWhenDatabaseUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := dbmigrate.Up(ctx, "postgres://nobody:x@127.0.0.1:1/none?sslmode=disable&connect_timeout=1", fstest.MapFS{})
	if err == nil {
		t.Fatal("Up succeeded against an unreachable database, want error")
	}
}

// testDSN returns a DSN whose search_path is a fresh schema dropped after the
// test, so tests in different packages can share one database. It skips the
// test when DBMIGRATE_TEST_DSN is not set.
func testDSN(t *testing.T) string {
	t.Helper()
	base := os.Getenv("DBMIGRATE_TEST_DSN")
	if base == "" {
		t.Skip("DBMIGRATE_TEST_DSN not set")
	}
	db, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	schema := fmt.Sprintf("dbmigrate_test_%d", time.Now().UnixNano())
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func TestUpSucceedsWithNoMigrations(t *testing.T) {
	dsn := testDSN(t)
	// Mirrors a service migrations package: the embedded FS holds only Go source.
	fsys := fstest.MapFS{"migrations.go": {Data: []byte("package migrations")}}

	if err := dbmigrate.Up(context.Background(), dsn, fsys); err != nil {
		t.Fatalf("Up with no migrations: %v", err)
	}
}

func TestUpAppliesMigrationsOnceAndIsIdempotent(t *testing.T) {
	dsn := testDSN(t)
	fsys := fstest.MapFS{
		"000001_widgets.up.sql":   {Data: []byte("CREATE TABLE widgets (id int PRIMARY KEY);")},
		"000001_widgets.down.sql": {Data: []byte("DROP TABLE widgets;")},
	}

	for i := range 2 {
		if err := dbmigrate.Up(context.Background(), dsn, fsys); err != nil {
			t.Fatalf("Up run %d: %v", i+1, err)
		}
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO widgets (id) VALUES (1)`); err != nil {
		t.Errorf("widgets table not usable after Up: %v", err)
	}
}

func TestUpFailsWhenDatabaseVersionIsMissingFromSource(t *testing.T) {
	dsn := testDSN(t)
	two := fstest.MapFS{
		"000001_widgets.up.sql": {Data: []byte("CREATE TABLE widgets (id int PRIMARY KEY);")},
		"000002_gadgets.up.sql": {Data: []byte("CREATE TABLE gadgets (id int PRIMARY KEY);")},
	}
	if err := dbmigrate.Up(context.Background(), dsn, two); err != nil {
		t.Fatal(err)
	}
	// An older binary that only knows migration 1 meets a database at version 2.
	one := fstest.MapFS{"000001_widgets.up.sql": two["000001_widgets.up.sql"]}

	if err := dbmigrate.Up(context.Background(), dsn, one); err == nil {
		t.Fatal("Up succeeded against a database version missing from the source, want error")
	}
}

func TestUpRejectsMisnamedSQLFiles(t *testing.T) {
	fsys := fstest.MapFS{"0001_init.sql": {Data: []byte("CREATE TABLE widgets (id int);")}}

	err := dbmigrate.Up(context.Background(), "postgres://unused", fsys)
	if err == nil || !strings.Contains(err.Error(), "0001_init.sql") {
		t.Fatalf("Up error = %v, want one naming 0001_init.sql", err)
	}
}
