package main

import (
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/shipping-simulator/migrations"
)

func main() {
	app.Main(app.Service{Name: "shipping-simulator", Migrations: migrations.FS, UsesKafka: true})
}
