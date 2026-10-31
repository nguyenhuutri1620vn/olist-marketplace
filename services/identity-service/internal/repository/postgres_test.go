package repository_test

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/dbmigrate"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/repository"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/token"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/migrations"
)

// newRepo migrates a fresh schema (dropped after the test) and returns a
// repository on it. It skips the test when DBMIGRATE_TEST_DSN is not set.
func newRepo(t *testing.T) (*repository.Postgres, *pgxpool.Pool) {
	t.Helper()
	base := os.Getenv("DBMIGRATE_TEST_DSN")
	if base == "" {
		t.Skip("DBMIGRATE_TEST_DSN not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("identity_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	if err := dbmigrate.Up(ctx, u.String(), migrations.FS); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return repository.New(pool), pool
}

func account(id, email string) domain.Account {
	return domain.Account{ID: id, Email: email, PasswordHash: "$2a$10$hash", Role: domain.RoleCustomer}
}

const (
	id1 = "0192f3a0-0000-7000-8000-000000000001"
	id2 = "0192f3a0-0000-7000-8000-000000000002"
)

func TestCreateAndFindAccount(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	if err := repo.CreateAccount(ctx, account(id1, "ana@example.com")); err != nil {
		t.Fatal(err)
	}
	got, err := repo.AccountByEmail(ctx, "ana@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id1 || got.Role != domain.RoleCustomer || got.PasswordHash != "$2a$10$hash" || got.CreatedAt.IsZero() {
		t.Errorf("AccountByEmail = %+v", got)
	}
	if _, err := repo.AccountByEmail(ctx, "nobody@example.com"); !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("unknown email err = %v, want ErrAccountNotFound", err)
	}
}

func TestCreateAccountRejectsTakenEmail(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	if err := repo.CreateAccount(ctx, account(id1, "ana@example.com")); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAccount(ctx, account(id2, "ana@example.com")); !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("duplicate email err = %v, want ErrEmailTaken", err)
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM account`).Scan(&n)
	if n != 1 {
		t.Errorf("accounts = %d, want 1", n)
	}
}

func TestSchemaRejectsUnnormalizedEmail(t *testing.T) {
	repo, _ := newRepo(t)
	if err := repo.CreateAccount(context.Background(), account(id1, "Ana@Example.com")); err == nil {
		t.Error("stored a mixed-case email, want the check constraint to refuse it")
	}
}

func TestSigningKeyIsCreatedOnceAndReloaded(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	generated := 0
	gen := func() (*rsa.PrivateKey, error) {
		generated++
		return token.GenerateKey()
	}
	first, err := repo.SigningKey(ctx, gen)
	if err != nil {
		t.Fatal(err)
	}
	again, err := repo.SigningKey(ctx, gen)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 1 {
		t.Errorf("generated %d keys, want 1", generated)
	}
	if again.ID != first.ID || !again.Private.Equal(first.Private) {
		t.Errorf("reloaded key %s differs from %s", again.ID, first.ID)
	}
}

func TestConcurrentStartupsShareOneKey(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make([]string, 4)
	for i := range ids {
		wg.Go(func() {
			k, err := repo.SigningKey(ctx, token.GenerateKey)
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = k.ID
		})
	}
	wg.Wait()
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM signing_key`).Scan(&n)
	if n != 1 || ids[0] != ids[1] || ids[1] != ids[2] || ids[2] != ids[3] {
		t.Errorf("keys stored %d, ids %v; want one shared key", n, ids)
	}
}
