package main

import (
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/analytics-service/migrations"
)

func main() {
	app.Main(app.Service{Name: "analytics-service", Migrations: migrations.FS, GRPC: true, UsesKafka: true})
}
