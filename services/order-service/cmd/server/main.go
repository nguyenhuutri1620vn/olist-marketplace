package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/catalog"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/repository"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/service"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/transport"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/migrations"
)

func main() {
	app.Main(orderService())
}

// orderService wires migrations, the outbox relay, the catalog client
// (CATALOG_ADDR, default catalog-service:9090; CATALOG_TIMEOUT) and the
// Customer-only Checkout RPC.
func orderService() app.Service {
	var orders *service.Orders
	return app.Service{
		Name:       "order-service",
		Migrations: migrations.FS,
		GRPC:       true,
		UsesKafka:  true,
		Outbox:     true,
		Policy: auth.Policy{Roles: map[string][]auth.Role{
			orderv1.OrderService_Checkout_FullMethodName:     {auth.RoleCustomer},
			orderv1.OrderService_ListMyOrders_FullMethodName: {auth.RoleCustomer},
			orderv1.OrderService_GetOrder_FullMethodName:     {auth.RoleCustomer},
		}},
		Init: func(_ context.Context, pool *pgxpool.Pool) error {
			timeout, err := catalog.Timeout()
			if err != nil {
				return err
			}
			addr := os.Getenv("CATALOG_ADDR")
			if addr == "" {
				addr = "catalog-service:9090"
			}
			// The connection lives as long as the process; it dials lazily.
			conn, err := grpcx.Dial(addr)
			if err != nil {
				return fmt.Errorf("dial catalog: %w", err)
			}
			orders = service.NewOrders(catalog.New(catalogv1.NewCatalogServiceClient(conn), timeout), repository.New(pool),
				time.Now, func() string { return uuid.Must(uuid.NewV7()).String() })
			return nil
		},
		RegisterGRPC: func(s *grpc.Server, _ *pgxpool.Pool) {
			transport.Register(s, orders)
		},
	}
}
