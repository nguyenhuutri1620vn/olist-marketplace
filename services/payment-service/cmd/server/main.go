package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/bootstrap"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/payment-service/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Migrate(ctx, migrations.FS); err != nil {
		log.Fatalf("payment-service: %v", err)
	}
	log.Println("payment-service: migrations applied, running")
	<-ctx.Done()
	log.Println("payment-service: shutting down")
}
