package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/bootstrap"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Migrate(ctx, migrations.FS); err != nil {
		log.Fatalf("order-service: %v", err)
	}
	log.Println("order-service: migrations applied, running")
	<-ctx.Done()
	log.Println("order-service: shutting down")
}
