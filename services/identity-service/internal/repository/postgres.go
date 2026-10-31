// Package repository is identity's Postgres adapter.
package repository

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/token"
)

// Postgres stores Accounts and the signing key.
type Postgres struct {
	pool *pgxpool.Pool
}

// New returns a repository on pool.
func New(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

// CreateAccount inserts a; an email already in use returns domain.ErrEmailTaken.
func (r *Postgres) CreateAccount(ctx context.Context, a domain.Account) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO account (id, email, password_hash, role) VALUES ($1, $2, $3, $4)`,
		a.ID, a.Email, a.PasswordHash, string(a.Role))
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "account_email_key" {
		return domain.ErrEmailTaken
	}
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

// AccountByEmail finds the Account with a normalized email, or returns
// domain.ErrAccountNotFound.
func (r *Postgres) AccountByEmail(ctx context.Context, email string) (domain.Account, error) {
	var a domain.Account
	var role string
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, email, password_hash, role, created_at FROM account WHERE email = $1`, email).
		Scan(&a.ID, &a.Email, &a.PasswordHash, &role, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("find account: %w", err)
	}
	a.Role = domain.Role(role)
	return a, nil
}

// SigningKey returns the newest stored signing key, creating one with
// generate when none exists. An advisory lock makes concurrent startups agree
// on a single key.
func (r *Postgres) SigningKey(ctx context.Context, generate func() (*rsa.PrivateKey, error)) (token.Key, error) {
	var key token.Key
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('identity.signing_key'))`); err != nil {
			return err
		}
		var der []byte
		err := tx.QueryRow(ctx, `SELECT private_key FROM signing_key ORDER BY created_at DESC, kid LIMIT 1`).Scan(&der)
		if err == nil {
			parsed, err := x509.ParsePKCS8PrivateKey(der)
			if err != nil {
				return fmt.Errorf("parse stored key: %w", err)
			}
			priv, ok := parsed.(*rsa.PrivateKey)
			if !ok {
				return fmt.Errorf("stored key is %T, want RSA", parsed)
			}
			key = token.NewKey(priv)
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		priv, err := generate()
		if err != nil {
			return fmt.Errorf("generate key: %w", err)
		}
		if der, err = x509.MarshalPKCS8PrivateKey(priv); err != nil {
			return err
		}
		key = token.NewKey(priv)
		_, err = tx.Exec(ctx, `INSERT INTO signing_key (kid, private_key) VALUES ($1, $2)`, key.ID, der)
		return err
	})
	if err != nil {
		return token.Key{}, fmt.Errorf("signing key: %w", err)
	}
	return key, nil
}

// UpsertAccounts inserts or updates seeded Accounts by id, all with hash, in
// one transaction. Only Accounts already in the reserved domain are updated,
// so an id that belongs to a registered Account cannot take it over. Such an
// id, an email owned by another Account, or the same id twice rejects the
// whole batch with domain.ErrInvalid.
func (r *Postgres) UpsertAccounts(ctx context.Context, seeds []domain.Seed, hash string) (int, error) {
	ids := make([]string, len(seeds))
	emails := make([]string, len(seeds))
	roles := make([]string, len(seeds))
	for i, s := range seeds {
		ids[i], emails[i], roles[i] = s.ID, s.Email, string(s.Role)
	}
	var n int64
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO account (id, email, password_hash, role)
			SELECT id, email, $4, role FROM unnest($1::uuid[], $2::text[], $3::text[]) AS s(id, email, role)
			ON CONFLICT (id) DO UPDATE SET
				email = EXCLUDED.email,
				password_hash = EXCLUDED.password_hash,
				role = EXCLUDED.role
			WHERE split_part(account.email, '@', 2) = $5`, ids, emails, roles, hash, domain.ReservedDomain)
		if err != nil {
			return err
		}
		if n = tag.RowsAffected(); n != int64(len(seeds)) {
			return fmt.Errorf("%w: %d of %d ids belong to Accounts outside @%s", domain.ErrInvalid,
				int64(len(seeds))-n, len(seeds), domain.ReservedDomain)
		}
		return nil
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505", // unique_violation: email owned by another Account
			"21000": // cardinality_violation: same id twice in one batch
			return 0, fmt.Errorf("upsert accounts: %w: %s (%s)", domain.ErrInvalid, pg.Message, pg.Detail)
		}
	}
	if err != nil {
		return 0, fmt.Errorf("upsert accounts: %w", err)
	}
	return int(n), nil
}
