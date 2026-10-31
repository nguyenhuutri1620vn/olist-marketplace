package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/bootstrap"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Migrate(ctx, migrations.FS); err != nil {
		log.Fatalf("catalog-service: %v", err)
	}
	log.Println("catalog-service: migrations applied, running")
	<-ctx.Done()
	log.Println("catalog-service: shutting down")
}
