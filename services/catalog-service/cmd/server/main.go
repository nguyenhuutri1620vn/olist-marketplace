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

// catalogService wires migrations, the import RPCs and their policy.
func catalogService() app.Service {
	return app.Service{
		Name:       "catalog-service",
		Migrations: migrations.FS,
		GRPC:       true,
		// The import RPCs are reachable only on the internal network (the
		// gateway never routes them); the importer runs without a token.
		Policy: auth.Policy{Public: []string{
			catalogv1.CatalogAdminService_ImportCategories_FullMethodName,
			catalogv1.CatalogAdminService_ImportSellers_FullMethodName,
			catalogv1.CatalogAdminService_ImportProducts_FullMethodName,
		}},
		RegisterGRPC: func(s *grpc.Server, pool *pgxpool.Pool) {
			transport.RegisterAdmin(s, service.NewImporter(repository.New(pool)))
		},
	}
}
