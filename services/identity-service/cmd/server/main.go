package main

import (
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/migrations"
)

func main() {
	app.Main(app.Service{Name: "identity-service", Migrations: migrations.FS, GRPC: true})
}
