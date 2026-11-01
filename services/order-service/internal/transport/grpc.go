// Package transport exposes order-service's use cases over gRPC.
package transport

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
)

// Orders is the use case port the handlers call.
type Orders interface {
	Checkout(ctx context.Context, co domain.Checkout) (domain.Placed, error)
}

type server struct {
	orderv1.UnimplementedOrderServiceServer
	orders Orders
}

// Register registers OrderService on s.
func Register(s grpc.ServiceRegistrar, orders Orders) {
	orderv1.RegisterOrderServiceServer(s, &server{orders: orders})
}

func (s *server) Checkout(ctx context.Context, req *orderv1.CheckoutRequest) (*orderv1.CheckoutResponse, error) {
	caller, ok := auth.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no caller")
	}
	co, err := domain.NewCheckout(caller.AccountID, req.GetCheckoutIdempotencyKey(), req.GetProductIds())
	if err != nil {
		return nil, toStatus(ctx, err)
	}
	placed, err := s.orders.Checkout(ctx, co)
	if err != nil {
		return nil, toStatus(ctx, err)
	}
	resp := &orderv1.CheckoutResponse{CheckoutId: placed.CheckoutID, Orders: make([]*orderv1.Order, len(placed.Orders))}
	for i, o := range placed.Orders {
		items := make([]*orderv1.OrderItem, len(o.Items))
		for j, it := range o.Items {
			items[j] = &orderv1.OrderItem{ProductId: it.ProductID, CategoryNameEn: it.CategoryEN, PriceCentavos: it.Price, FreightCentavos: it.Freight}
		}
		resp.Orders[i] = &orderv1.Order{OrderId: o.ID, SellerId: o.SellerID, Status: o.Status, TotalCentavos: o.Total(), Items: items}
	}
	return resp, nil
}

// toStatus maps domain errors to gRPC codes; anything else is INTERNAL with
// the detail logged, not returned.
func toStatus(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrKeyReused):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrUnavailable):
		slog.WarnContext(ctx, "checkout refused: dependency unavailable", "err", err)
		return status.Error(codes.Unavailable, "catalog unavailable; nothing was created")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	}
	slog.ErrorContext(ctx, "order request failed", "err", err)
	return status.Error(codes.Internal, "internal error")
}
