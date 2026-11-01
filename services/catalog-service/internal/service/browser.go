package service

import (
	"context"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
)

// Catalog reads reference data for browsing.
type Catalog interface {
	Categories(context.Context) ([]domain.Category, error)
	ListProducts(context.Context, domain.ProductQuery) (domain.ProductPage, error)
}

// Browser lists Categories and Products (FR13).
type Browser struct {
	catalog Catalog
}

// NewBrowser returns a Browser reading from catalog.
func NewBrowser(catalog Catalog) *Browser {
	return &Browser{catalog: catalog}
}

// Categories lists every Category, "Uncategorized" included.
func (b *Browser) Categories(ctx context.Context) ([]domain.Category, error) {
	return b.catalog.Categories(ctx)
}

// Products returns one page of Products; an unknown Category is ErrInvalid.
func (b *Browser) Products(ctx context.Context, q domain.ProductQuery) (domain.ProductPage, error) {
	return b.catalog.ListProducts(ctx, q)
}
