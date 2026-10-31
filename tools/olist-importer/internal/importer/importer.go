// Package importer sends the Olist reference data to catalog's admin gRPC
// (AD-12). It never touches a service database.
package importer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/tools/olist-importer/internal/olist"
)

// BatchSize keeps each request far below gRPC's 4 MB default limit (a Product
// is about 100 bytes on the wire).
const BatchSize = 1000

// BatchTimeout bounds each RPC so a stalled catalog fails the import instead
// of hanging it.
const BatchTimeout = time.Minute

// Summary counts what catalog upserted, plus the Products the dataset could
// not price.
type Summary struct {
	Categories int
	Sellers    int
	Products   int
	Skipped    int
}

// Run imports Categories, then Sellers, then Products (which reference both),
// in batches of size, and stops at the first failed batch.
func Run(ctx context.Context, c catalogv1.CatalogAdminServiceClient, ds olist.Dataset, size int) (Summary, error) {
	sum := Summary{Skipped: len(ds.Skipped)}
	if len(ds.Skipped) > 0 {
		slog.WarnContext(ctx, "products skipped: no order item", "count", len(ds.Skipped), "product_ids", ds.Skipped)
	}
	err := batches(ds.Categories, size, func(b []olist.Category) error {
		req := &catalogv1.ImportCategoriesRequest{}
		for _, c := range b {
			req.Categories = append(req.Categories, &catalogv1.ImportCategory{NamePt: c.NamePT, NameEn: c.NameEN})
		}
		rctx, cancel := context.WithTimeout(ctx, BatchTimeout)
		defer cancel()
		resp, err := c.ImportCategories(rctx, req)
		sum.Categories += int(resp.GetUpserted())
		return err
	})
	if err != nil {
		return sum, fmt.Errorf("import categories: %w", err)
	}
	err = batches(ds.Sellers, size, func(b []olist.Seller) error {
		req := &catalogv1.ImportSellersRequest{}
		for _, s := range b {
			req.Sellers = append(req.Sellers, &catalogv1.ImportSeller{SellerId: s.ID, City: s.City, State: s.State})
		}
		rctx, cancel := context.WithTimeout(ctx, BatchTimeout)
		defer cancel()
		resp, err := c.ImportSellers(rctx, req)
		sum.Sellers += int(resp.GetUpserted())
		return err
	})
	if err != nil {
		return sum, fmt.Errorf("import sellers: %w", err)
	}
	err = batches(ds.Products, size, func(b []olist.Product) error {
		req := &catalogv1.ImportProductsRequest{}
		for _, p := range b {
			req.Products = append(req.Products, &catalogv1.ImportProduct{
				ProductId: p.ID, CategoryNamePt: p.Category, SellerId: p.SellerID,
				PriceCentavos: p.Price, FreightCentavos: p.Freight,
			})
		}
		rctx, cancel := context.WithTimeout(ctx, BatchTimeout)
		defer cancel()
		resp, err := c.ImportProducts(rctx, req)
		sum.Products += int(resp.GetUpserted())
		return err
	})
	if err != nil {
		return sum, fmt.Errorf("import products: %w", err)
	}
	return sum, nil
}

func batches[T any](items []T, size int, send func([]T) error) error {
	for start := 0; start < len(items); start += size {
		if err := send(items[start:min(start+size, len(items))]); err != nil {
			return err
		}
	}
	return nil
}
