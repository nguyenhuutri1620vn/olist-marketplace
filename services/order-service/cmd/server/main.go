package main

import (
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/migrations"
)

func main() {
	app.Main(orderService())
}

// orderService wires migrations, gRPC and the outbox relay.
func orderService() app.Service {
	return app.Service{Name: "order-service", Migrations: migrations.FS, GRPC: true, UsesKafka: true, Outbox: true}
}
