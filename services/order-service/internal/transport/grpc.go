// Package transport exposes order-service's use cases over gRPC.
package transport

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
)

// Orders is the use case port the handlers call.
type Orders interface {
	Checkout(ctx context.Context, co domain.Checkout) (domain.Placed, error)
	MyOrders(ctx context.Context, customerID string, p domain.Page) (domain.SummaryPage, error)
	Order(ctx context.Context, customerID, orderID string) (domain.Detail, error)
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
		resp.Orders[i] = &orderv1.Order{OrderId: o.ID, SellerId: o.SellerID, Status: o.Status, TotalCentavos: o.Total(), Items: items(o.Items)}
	}
	return resp, nil
}

func (s *server) ListMyOrders(ctx context.Context, req *orderv1.ListMyOrdersRequest) (*orderv1.ListMyOrdersResponse, error) {
	caller, ok := auth.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no caller")
	}
	p, err := domain.NewPage(req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, toStatus(ctx, err)
	}
	list, err := s.orders.MyOrders(ctx, caller.AccountID, p)
	if err != nil {
		return nil, toStatus(ctx, err)
	}
	resp := &orderv1.ListMyOrdersResponse{Total: list.Total, Orders: make([]*orderv1.OrderSummary, len(list.Orders))}
	for i, o := range list.Orders {
		resp.Orders[i] = &orderv1.OrderSummary{
			OrderId: o.ID, PurchasedAt: timestamppb.New(o.PurchasedAt), Status: o.Status,
			TotalCentavos: o.Total, SellerId: o.SellerID, ItemCount: int32(o.ItemCount),
		}
	}
	return resp, nil
}

func (s *server) GetOrder(ctx context.Context, req *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	caller, ok := auth.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no caller")
	}
	id, err := domain.ParseOrderID(req.GetOrderId())
	if err != nil {
		return nil, toStatus(ctx, err)
	}
	d, err := s.orders.Order(ctx, caller.AccountID, id)
	if err != nil {
		return nil, toStatus(ctx, err)
	}
	out := &orderv1.OrderDetail{
		OrderId: d.ID, PurchasedAt: timestamppb.New(d.PurchasedAt), Status: d.Status, TotalCentavos: d.Total(),
		SellerId: d.SellerID, VoucherAmountCentavos: d.VoucherAmount, CancelReason: d.CancelReason, VoucherRestored: d.VoucherRestored,
		Items: items(d.Items), Timeline: make([]*orderv1.StatusChange, len(d.Timeline)),
	}
	for i, c := range d.Timeline {
		out.Timeline[i] = &orderv1.StatusChange{Status: c.Status, At: timestamppb.New(c.At)}
	}
	return &orderv1.GetOrderResponse{Order: out}, nil
}

func items(its []domain.Item) []*orderv1.OrderItem {
	out := make([]*orderv1.OrderItem, len(its))
	for i, it := range its {
		out[i] = &orderv1.OrderItem{ProductId: it.ProductID, CategoryNameEn: it.CategoryEN, PriceCentavos: it.Price, FreightCentavos: it.Freight}
	}
	return out
}

// toStatus maps domain errors to gRPC codes; anything else is INTERNAL with
// the detail logged, not returned.
func toStatus(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "order not found")
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
