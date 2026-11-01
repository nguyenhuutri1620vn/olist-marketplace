package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
)

// productOrder sorts by display name ignoring case, then by id, so pages never
// overlap. COLLATE "C" keeps the order the same under any database locale;
// without lower() the one capitalised name, "Uncategorized", would fill the
// first pages. migrations/000002 indexes this exact order.
const productOrder = `lower(p.display_name) COLLATE "C", p.display_name COLLATE "C", p.id`

// Categories lists every Category by English name, "Uncategorized" last.
func (r *Postgres) Categories(ctx context.Context) ([]domain.Category, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT name_pt, name_en FROM category
		ORDER BY name_pt = $1, lower(name_en) COLLATE "C", name_pt COLLATE "C"`, domain.Uncategorized)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	cats, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Category, error) {
		var c domain.Category
		err := row.Scan(&c.NamePT, &c.NameEN)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return cats, nil
}

// ListProducts returns one page of Products and the number matching the
// filter, both read from one snapshot. An unknown Category is ErrInvalid.
func (r *Postgres) ListProducts(ctx context.Context, q domain.ProductQuery) (domain.ProductPage, error) {
	page := domain.ProductPage{Products: []domain.ProductSummary{}}
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if q.Category != "" {
			var known bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM category WHERE name_pt = $1)`, q.Category).Scan(&known); err != nil {
				return err
			}
			if !known {
				return fmt.Errorf("%w: unknown category %q", domain.ErrInvalid, q.Category)
			}
		}
		// Separate statements with and without the Category: a cached generic
		// plan for "$1 = '' OR category = $1" cannot use the Category index.
		count, where, args := `SELECT count(*) FROM product`, "", []any{q.Limit(), q.Offset()}
		if q.Category != "" {
			count, where, args = count+` WHERE category = $1`, `WHERE p.category = $3`, append(args, q.Category)
		}
		if err := tx.QueryRow(ctx, count, args[2:]...).Scan(&page.Total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT replace(p.id::text, '-', ''), p.display_name, c.name_pt, c.name_en,
				replace(s.id::text, '-', ''), s.city, s.state, p.price
			FROM product p
			JOIN category c ON c.name_pt = p.category
			JOIN seller s ON s.id = p.seller_id
			`+where+`
			ORDER BY `+productOrder+`
			LIMIT $1 OFFSET $2`, args...)
		if err != nil {
			return err
		}
		products, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.ProductSummary, error) {
			var p domain.ProductSummary
			err := row.Scan(&p.ID, &p.DisplayName, &p.Category.NamePT, &p.Category.NameEN,
				&p.Seller.ID, &p.Seller.City, &p.Seller.State, &p.Price)
			return p, err
		})
		if err != nil {
			return err
		}
		page.Products = append(page.Products, products...)
		return nil
	})
	if err != nil {
		return domain.ProductPage{}, fmt.Errorf("list products: %w", err)
	}
	return page, nil
}
