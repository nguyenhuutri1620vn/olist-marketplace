package main

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/repository"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/service"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/transport"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/migrations"
)

func main() {
	app.Main(catalogService())
}

// catalogService wires migrations, the browsing and import RPCs and their policy.
func catalogService() app.Service {
	return app.Service{
		Name:       "catalog-service",
		Migrations: migrations.FS,
		GRPC:       true,
		Policy: auth.Policy{Public: []string{
			// Browsing needs no token (NFR7).
			catalogv1.CatalogService_ListCategories_FullMethodName,
			catalogv1.CatalogService_ListProducts_FullMethodName,
			// order-service's Checkout snapshot; public catalog data, called on
			// the internal network only (the gateway never routes it).
			catalogv1.CatalogService_GetProducts_FullMethodName,
			// The import RPCs are reachable only on the internal network (the
			// gateway never routes them); the importer runs without a token.
			catalogv1.CatalogAdminService_ImportCategories_FullMethodName,
			catalogv1.CatalogAdminService_ImportSellers_FullMethodName,
			catalogv1.CatalogAdminService_ImportProducts_FullMethodName,
		}},
		RegisterGRPC: func(s *grpc.Server, pool *pgxpool.Pool) {
			repo := repository.New(pool)
			transport.RegisterCatalog(s, service.NewBrowser(repo))
			transport.RegisterAdmin(s, service.NewImporter(repo))
		},
	}
}
