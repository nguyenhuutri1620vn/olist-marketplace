// Package repository is catalog's Postgres adapter.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
)

// Postgres stores catalog reference data. Each upsert applies its whole batch
// in one statement, so a bad row rejects the batch.
type Postgres struct {
	pool *pgxpool.Pool
}

// New returns a repository on pool.
func New(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

// displayName is `<Category English name> #<first 4 chars of product id>` (FR13).
const displayName = `c.name_en || ' #' || left(p.id::text, 4)`

// UpsertCategories inserts or updates Categories and renames their Products.
func (r *Postgres) UpsertCategories(ctx context.Context, cats []domain.Category) (int, error) {
	pt := make([]string, len(cats))
	en := make([]string, len(cats))
	for i, c := range cats {
		pt[i], en[i] = c.NamePT, c.NameEN
	}
	var n int64
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO category (name_pt, name_en)
			SELECT * FROM unnest($1::text[], $2::text[])
			ON CONFLICT (name_pt) DO UPDATE SET name_en = EXCLUDED.name_en`, pt, en)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		_, err = tx.Exec(ctx, `
			UPDATE product p SET display_name = `+displayName+`
			FROM category c
			WHERE p.category = c.name_pt AND c.name_pt = ANY($1) AND p.display_name <> `+displayName, pt)
		return err
	})
	return int(n), classify("upsert categories", err)
}

// UpsertSellers inserts or updates Seller profiles.
func (r *Postgres) UpsertSellers(ctx context.Context, sellers []domain.Seller) (int, error) {
	ids := make([]string, len(sellers))
	cities := make([]string, len(sellers))
	states := make([]string, len(sellers))
	for i, s := range sellers {
		ids[i], cities[i], states[i] = s.ID, s.City, s.State
	}
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO seller (id, city, state)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[])
		ON CONFLICT (id) DO UPDATE SET city = EXCLUDED.city, state = EXCLUDED.state`, ids, cities, states)
	return int(tag.RowsAffected()), classify("upsert sellers", err)
}

// UpsertProducts inserts or updates Products, deriving each display name from
// its Category row.
func (r *Postgres) UpsertProducts(ctx context.Context, products []domain.Product) (int, error) {
	ids := make([]string, len(products))
	cats := make([]string, len(products))
	sellers := make([]string, len(products))
	prices := make([]int64, len(products))
	freights := make([]int64, len(products))
	for i, p := range products {
		ids[i], cats[i], sellers[i], prices[i], freights[i] = p.ID, p.Category, p.SellerID, p.Price, p.Freight
	}
	// LEFT JOIN so an unknown Category fails the foreign key instead of
	// silently dropping the row.
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO product (id, category, display_name, price, freight, seller_id)
		SELECT p.id, p.category, coalesce(`+displayName+`, ''), p.price, p.freight, p.seller_id
		FROM unnest($1::uuid[], $2::text[], $3::uuid[], $4::bigint[], $5::bigint[])
			AS p(id, category, seller_id, price, freight)
		LEFT JOIN category c ON c.name_pt = p.category
		ON CONFLICT (id) DO UPDATE SET
			category = EXCLUDED.category,
			display_name = EXCLUDED.display_name,
			price = EXCLUDED.price,
			freight = EXCLUDED.freight,
			seller_id = EXCLUDED.seller_id`, ids, cats, sellers, prices, freights)
	return int(tag.RowsAffected()), classify("upsert products", err)
}

// classify turns errors caused by the batch's content into domain.ErrInvalid.
func classify(op string, err error) error {
	if err == nil {
		return nil
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23503", // foreign_key_violation: unknown Category or Seller
			"23514", // check_violation
			"21000", // cardinality_violation: same id twice in one batch
			"22P02": // invalid_text_representation: bad uuid
			// Detail names the offending key, e.g. Key (seller_id)=(...) is not present.
			return fmt.Errorf("%s: %w: %s (%s)", op, domain.ErrInvalid, pg.Message, pg.Detail)
		}
	}
	return fmt.Errorf("%s: %w", op, err)
}
