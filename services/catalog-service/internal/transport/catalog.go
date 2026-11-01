package transport

import (
	"context"

	"google.golang.org/grpc"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/service"
)

// catalog serves CatalogService, the public browsing RPCs (FR13, NFR7).
type catalog struct {
	catalogv1.UnimplementedCatalogServiceServer
	browser *service.Browser
}

// RegisterCatalog registers CatalogService on s.
func RegisterCatalog(s grpc.ServiceRegistrar, browser *service.Browser) {
	catalogv1.RegisterCatalogServiceServer(s, &catalog{browser: browser})
}

func (c *catalog) ListCategories(ctx context.Context, _ *catalogv1.ListCategoriesRequest) (*catalogv1.ListCategoriesResponse, error) {
	cats, err := c.browser.Categories(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &catalogv1.ListCategoriesResponse{Categories: make([]*catalogv1.Category, len(cats))}
	for i, cat := range cats {
		resp.Categories[i] = &catalogv1.Category{NamePt: cat.NamePT, NameEn: cat.NameEN}
	}
	return resp, nil
}

func (c *catalog) ListProducts(ctx context.Context, req *catalogv1.ListProductsRequest) (*catalogv1.ListProductsResponse, error) {
	q, err := domain.NewProductQuery(req.GetCategory(), req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, toStatus(err)
	}
	page, err := c.browser.Products(ctx, q)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &catalogv1.ListProductsResponse{Total: page.Total, Products: make([]*catalogv1.ProductSummary, len(page.Products))}
	for i, p := range page.Products {
		resp.Products[i] = &catalogv1.ProductSummary{
			ProductId:      p.ID,
			DisplayName:    p.DisplayName,
			CategoryNamePt: p.Category.NamePT,
			CategoryNameEn: p.Category.NameEN,
			SellerId:       p.Seller.ID,
			SellerCity:     p.Seller.City,
			SellerState:    p.Seller.State,
			PriceCentavos:  p.Price,
		}
	}
	return resp, nil
}

func (c *catalog) GetProducts(ctx context.Context, req *catalogv1.GetProductsRequest) (*catalogv1.GetProductsResponse, error) {
	ids, err := domain.NewProductIDs(req.GetProductIds())
	if err != nil {
		return nil, toStatus(err)
	}
	snaps, err := c.browser.Snapshots(ctx, ids)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &catalogv1.GetProductsResponse{Products: make([]*catalogv1.ProductSnapshot, len(snaps))}
	for i, p := range snaps {
		resp.Products[i] = &catalogv1.ProductSnapshot{
			ProductId:       p.ID,
			SellerId:        p.SellerID,
			SellerState:     p.SellerState,
			CategoryNameEn:  p.CategoryEN,
			PriceCentavos:   p.Price,
			FreightCentavos: p.Freight,
		}
	}
	return resp, nil
}
