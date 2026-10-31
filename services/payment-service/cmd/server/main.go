package main

import (
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/payment-service/migrations"
)

func main() {
	app.Main(app.Service{Name: "payment-service", Migrations: migrations.FS, GRPC: true, UsesKafka: true})
}
