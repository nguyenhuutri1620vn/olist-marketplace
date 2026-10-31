// Package service holds catalog's use cases and the ports they depend on.
package service

import (
	"context"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
)

// Repository stores reference data. Each call applies its batch whole or not
// at all and returns the number of rows upserted.
type Repository interface {
	UpsertCategories(context.Context, []domain.Category) (int, error)
	UpsertSellers(context.Context, []domain.Seller) (int, error)
	UpsertProducts(context.Context, []domain.Product) (int, error)
}

// Importer loads Olist reference data (FR17) as idempotent upserts.
type Importer struct {
	repo Repository
}

// NewImporter returns an Importer storing into repo.
func NewImporter(repo Repository) *Importer {
	return &Importer{repo: repo}
}

// ImportCategories upserts Categories; an empty batch is a no-op.
func (i *Importer) ImportCategories(ctx context.Context, cats []domain.Category) (int, error) {
	if len(cats) == 0 {
		return 0, nil
	}
	return i.repo.UpsertCategories(ctx, cats)
}

// ImportSellers upserts Seller profiles; an empty batch is a no-op.
func (i *Importer) ImportSellers(ctx context.Context, sellers []domain.Seller) (int, error) {
	if len(sellers) == 0 {
		return 0, nil
	}
	return i.repo.UpsertSellers(ctx, sellers)
}

// ImportProducts upserts Products; an empty batch is a no-op.
func (i *Importer) ImportProducts(ctx context.Context, products []domain.Product) (int, error) {
	if len(products) == 0 {
		return 0, nil
	}
	return i.repo.UpsertProducts(ctx, products)
}
