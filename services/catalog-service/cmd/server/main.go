package main

import (
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/migrations"
)

func main() {
	app.Main(app.Service{Name: "catalog-service", Migrations: migrations.FS, GRPC: true})
}
