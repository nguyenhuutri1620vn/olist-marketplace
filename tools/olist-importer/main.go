// Command olist-importer loads Olist reference data into catalog-service and
// seeds demo Accounts into identity-service, only through their admin gRPC
// (`olist-importer import`). It reads the Kaggle CSVs from DATA_DIR (default
// /data), calls CATALOG_ADDR (default catalog-service:9090) and IDENTITY_ADDR
// (default identity-service:9090), and gives every demo Account the password
// DEMO_PASSWORD (default olist-demo-2026).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/logger"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/tracing"
	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/tools/olist-importer/internal/importer"
	"github.com/nguyenhuutri1620vn/olist-marketplace/tools/olist-importer/internal/olist"
)

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func main() {
	if len(os.Args) != 2 || os.Args[1] != "import" {
		fmt.Fprintln(os.Stderr, "usage: olist-importer import")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.New("olist-importer")
	if err := run(ctx); err != nil {
		slog.Error("import failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	shutdown, err := tracing.Init(ctx, "olist-importer")
	if err != nil {
		return err
	}
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdown(sctx)
	}()

	dir := env("DATA_DIR", "/data")
	ds, err := olist.Load(dir)
	if err != nil {
		return fmt.Errorf("read dataset in %s: %w", dir, err)
	}
	demo, err := olist.DemoAccounts(dir)
	if err != nil {
		return fmt.Errorf("pick demo accounts in %s: %w", dir, err)
	}

	catalog, err := grpcx.Dial(env("CATALOG_ADDR", "catalog-service:9090"))
	if err != nil {
		return err
	}
	defer catalog.Close()
	sum, err := importer.Run(ctx, catalogv1.NewCatalogAdminServiceClient(catalog), ds, importer.BatchSize)
	if err != nil {
		return err
	}

	identity, err := grpcx.Dial(env("IDENTITY_ADDR", "identity-service:9090"))
	if err != nil {
		return err
	}
	defer identity.Close()
	seeded, err := importer.ImportAccounts(ctx, identityv1.NewIdentityAdminServiceClient(identity), demo, env("DEMO_PASSWORD", "olist-demo-2026"))
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "import done", "categories", sum.Categories, "sellers", sum.Sellers,
		"products", sum.Products, "skipped_products", sum.Skipped, "demo_accounts", seeded)
	return nil
}
