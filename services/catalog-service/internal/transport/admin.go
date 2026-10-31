// Package transport exposes catalog's use cases over gRPC.
package transport

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/service"
)

// admin serves CatalogAdminService (AD-12).
type admin struct {
	catalogv1.UnimplementedCatalogAdminServiceServer
	importer *service.Importer
}

// RegisterAdmin registers CatalogAdminService on s.
func RegisterAdmin(s grpc.ServiceRegistrar, importer *service.Importer) {
	catalogv1.RegisterCatalogAdminServiceServer(s, &admin{importer: importer})
}

func (a *admin) ImportCategories(ctx context.Context, req *catalogv1.ImportCategoriesRequest) (*catalogv1.ImportCategoriesResponse, error) {
	cats := make([]domain.Category, 0, len(req.GetCategories()))
	for _, c := range req.GetCategories() {
		cat, err := domain.NewCategory(c.GetNamePt(), c.GetNameEn())
		if err != nil {
			return nil, toStatus(err)
		}
		cats = append(cats, cat)
	}
	n, err := a.importer.ImportCategories(ctx, cats)
	if err != nil {
		return nil, toStatus(err)
	}
	return &catalogv1.ImportCategoriesResponse{Upserted: int32(n)}, nil
}

func (a *admin) ImportSellers(ctx context.Context, req *catalogv1.ImportSellersRequest) (*catalogv1.ImportSellersResponse, error) {
	sellers := make([]domain.Seller, 0, len(req.GetSellers()))
	for _, s := range req.GetSellers() {
		seller, err := domain.NewSeller(s.GetSellerId(), s.GetCity(), s.GetState())
		if err != nil {
			return nil, toStatus(err)
		}
		sellers = append(sellers, seller)
	}
	n, err := a.importer.ImportSellers(ctx, sellers)
	if err != nil {
		return nil, toStatus(err)
	}
	return &catalogv1.ImportSellersResponse{Upserted: int32(n)}, nil
}

func (a *admin) ImportProducts(ctx context.Context, req *catalogv1.ImportProductsRequest) (*catalogv1.ImportProductsResponse, error) {
	products := make([]domain.Product, 0, len(req.GetProducts()))
	for _, p := range req.GetProducts() {
		product, err := domain.NewProduct(p.GetProductId(), p.GetCategoryNamePt(), p.GetSellerId(), p.GetPriceCentavos(), p.GetFreightCentavos())
		if err != nil {
			return nil, toStatus(err)
		}
		products = append(products, product)
	}
	n, err := a.importer.ImportProducts(ctx, products)
	if err != nil {
		return nil, toStatus(err)
	}
	return &catalogv1.ImportProductsResponse{Upserted: int32(n)}, nil
}

// toStatus maps domain rule violations to INVALID_ARGUMENT and anything else
// to INTERNAL.
func toStatus(err error) error {
	if errors.Is(err, domain.ErrInvalid) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}
