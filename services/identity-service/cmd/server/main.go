package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/bootstrap"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Migrate(ctx, migrations.FS); err != nil {
		log.Fatalf("identity-service: %v", err)
	}
	log.Println("identity-service: migrations applied, running")
	<-ctx.Done()
	log.Println("identity-service: shutting down")
}
