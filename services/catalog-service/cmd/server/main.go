package main

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/repository"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/service"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/transport"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/migrations"
)

func main() {
	app.Main(app.Service{
		Name:       "catalog-service",
		Migrations: migrations.FS,
		GRPC:       true,
		RegisterGRPC: func(s *grpc.Server, pool *pgxpool.Pool) {
			transport.RegisterAdmin(s, service.NewImporter(repository.New(pool)))
		},
	})
}
