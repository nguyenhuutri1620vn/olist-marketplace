// Command olist-importer loads Olist reference data into catalog-service
// through its admin gRPC (`olist-importer import`). It reads the Kaggle CSVs
// from DATA_DIR (default /data) and calls CATALOG_ADDR (default
// catalog-service:9090).
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
	conn, err := grpcx.Dial(env("CATALOG_ADDR", "catalog-service:9090"))
	if err != nil {
		return err
	}
	defer conn.Close()

	sum, err := importer.Run(ctx, catalogv1.NewCatalogAdminServiceClient(conn), ds, importer.BatchSize)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "import done", "categories", sum.Categories, "sellers", sum.Sellers,
		"products", sum.Products, "skipped_products", sum.Skipped)
	return nil
}
