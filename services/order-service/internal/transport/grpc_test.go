package transport_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/transport"
)

const (
	customer = "0192f000-0000-7000-8000-000000000c01"
	p1       = "1e9e8ef04dbcff4541ed26657ea517e5"
	sellerA  = "3442f8959a84dea7ee197c632cb2df15"
)

// fakeOrders records what it got and answers with its fields or err.
type fakeOrders struct {
	got      *domain.Checkout
	placed   domain.Placed
	list     domain.SummaryPage
	detail   domain.Detail
	customer string
	page     *domain.Page
	orderID  string
	err      error
}

func (f *fakeOrders) MyOrders(_ context.Context, customerID string, p domain.Page) (domain.SummaryPage, error) {
	f.customer, f.page = customerID, &p
	return f.list, f.err
}

func (f *fakeOrders) Order(_ context.Context, customerID, orderID string) (domain.Detail, error) {
	f.customer, f.orderID = customerID, orderID
	return f.detail, f.err
}

func (f *fakeOrders) Checkout(_ context.Context, co domain.Checkout) (domain.Placed, error) {
	f.got = &co
	return f.placed, f.err
}

// client serves the handler with the caller's claims put in ctx, as the
// auth interceptor would.
func client(t *testing.T, f *fakeOrders, claims *auth.Claims) orderv1.OrderServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if claims != nil {
			ctx = auth.WithClaims(ctx, *claims, "jwt")
		}
		return h(ctx, req)
	}))
	transport.Register(srv, f)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return orderv1.NewOrderServiceClient(conn)
}

var caller = &auth.Claims{AccountID: customer, Role: auth.RoleCustomer}

func TestCheckoutUsesTheCallerAndMapsTheOrders(t *testing.T) {
	f := &fakeOrders{placed: domain.Placed{CheckoutID: "k-1", Orders: []domain.Order{{
		ID: "o-1", SellerID: sellerA, Status: "created", PurchasedAt: time.Now(),
		Items: []domain.Item{{ProductID: p1, SellerID: sellerA, CategoryEN: "health_beauty", Price: 5890, Freight: 1329}},
	}}}}
	resp, err := client(t, f, caller).Checkout(context.Background(), &orderv1.CheckoutRequest{ProductIds: []string{p1}, CheckoutIdempotencyKey: "key-1"})
	if err != nil {
		t.Fatal(err)
	}
	if f.got == nil || f.got.CustomerID != customer || f.got.Key != "key-1" || len(f.got.ProductIDs) != 1 {
		t.Errorf("use case got %+v", f.got)
	}
	want := &orderv1.CheckoutResponse{CheckoutId: "k-1", Orders: []*orderv1.Order{{
		OrderId: "o-1", SellerId: sellerA, Status: "created", TotalCentavos: 7219,
		Items: []*orderv1.OrderItem{{ProductId: p1, CategoryNameEn: "health_beauty", PriceCentavos: 5890, FreightCentavos: 1329}},
	}}}
	if !proto.Equal(resp, want) {
		t.Errorf("Checkout = %v\nwant %v", resp, want)
	}
}

func TestCheckoutMapsErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want codes.Code
	}{
		{fmt.Errorf("%w: unknown product", domain.ErrInvalid), codes.InvalidArgument},
		{domain.ErrKeyReused, codes.AlreadyExists},
		{fmt.Errorf("%w: catalog", domain.ErrUnavailable), codes.Unavailable},
		{errors.New("connection reset"), codes.Internal},
	} {
		_, err := client(t, &fakeOrders{err: tc.err}, caller).Checkout(context.Background(), &orderv1.CheckoutRequest{ProductIds: []string{p1}, CheckoutIdempotencyKey: "k"})
		if status.Code(err) != tc.want {
			t.Errorf("%v → %v, want %v", tc.err, status.Code(err), tc.want)
		}
	}
}

func TestCheckoutValidatesBeforeTheUseCase(t *testing.T) {
	f := &fakeOrders{}
	_, err := client(t, f, caller).Checkout(context.Background(), &orderv1.CheckoutRequest{ProductIds: []string{p1, p1}, CheckoutIdempotencyKey: "k"})
	if status.Code(err) != codes.InvalidArgument || f.got != nil {
		t.Errorf("repeated id: %v, use case called %v", err, f.got != nil)
	}
}

// The policy admits only Customers; without claims there is no caller.
func TestCheckoutWithoutACallerIsUnauthenticated(t *testing.T) {
	_, err := client(t, &fakeOrders{}, nil).Checkout(context.Background(), &orderv1.CheckoutRequest{ProductIds: []string{p1}, CheckoutIdempotencyKey: "k"})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("no caller: %v", err)
	}
}

var at = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

func TestListMyOrdersMapsThePage(t *testing.T) {
	f := &fakeOrders{list: domain.SummaryPage{Total: 41, Orders: []domain.Summary{{
		ID: "o-1", SellerID: sellerA, Status: "created", PurchasedAt: at, Total: 7219, ItemCount: 2,
	}}}}
	resp, err := client(t, f, caller).ListMyOrders(context.Background(), &orderv1.ListMyOrdersRequest{Page: 2, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if f.customer != customer || f.page == nil || *f.page != (domain.Page{Number: 2, Size: 20}) {
		t.Errorf("use case got %s %+v", f.customer, f.page)
	}
	want := &orderv1.ListMyOrdersResponse{Total: 41, Orders: []*orderv1.OrderSummary{{
		OrderId: "o-1", SellerId: sellerA, Status: "created", PurchasedAt: timestamppb.New(at), TotalCentavos: 7219, ItemCount: 2,
	}}}
	if !proto.Equal(resp, want) {
		t.Errorf("ListMyOrders = %v\nwant %v", resp, want)
	}
	for _, req := range []*orderv1.ListMyOrdersRequest{{Page: 0, PageSize: 20}, {Page: 1, PageSize: 101}} {
		f := &fakeOrders{}
		if _, err := client(t, f, caller).ListMyOrders(context.Background(), req); status.Code(err) != codes.InvalidArgument || f.page != nil {
			t.Errorf("%v: %v (use case called %v)", req, err, f.page != nil)
		}
	}
}

func TestGetOrderMapsTheDetail(t *testing.T) {
	f := &fakeOrders{detail: domain.Detail{
		Order: domain.Order{ID: "0192f000-0000-7000-8000-0000000000aa", SellerID: sellerA, Status: "canceled", PurchasedAt: at, VoucherAmount: 500,
			Items: []domain.Item{{ProductID: p1, SellerID: sellerA, CategoryEN: "health_beauty", Price: 5890, Freight: 1329}}},
		Timeline:     []domain.StatusChange{{Status: "created", At: at}, {Status: "canceled", At: at.Add(time.Minute)}},
		CancelReason: "CARD_DECLINED", VoucherRestored: true,
	}}
	resp, err := client(t, f, caller).GetOrder(context.Background(), &orderv1.GetOrderRequest{OrderId: "0192F000-0000-7000-8000-0000000000AA"})
	if err != nil {
		t.Fatal(err)
	}
	if f.customer != customer || f.orderID != "0192f000-0000-7000-8000-0000000000aa" {
		t.Errorf("use case got %s %s", f.customer, f.orderID)
	}
	want := &orderv1.GetOrderResponse{Order: &orderv1.OrderDetail{
		OrderId: "0192f000-0000-7000-8000-0000000000aa", SellerId: sellerA, Status: "canceled", PurchasedAt: timestamppb.New(at),
		TotalCentavos: 7219, VoucherAmountCentavos: 500,
		Items:        []*orderv1.OrderItem{{ProductId: p1, CategoryNameEn: "health_beauty", PriceCentavos: 5890, FreightCentavos: 1329}},
		Timeline:     []*orderv1.StatusChange{{Status: "created", At: timestamppb.New(at)}, {Status: "canceled", At: timestamppb.New(at.Add(time.Minute))}},
		CancelReason: "CARD_DECLINED", VoucherRestored: true,
	}}
	if !proto.Equal(resp, want) {
		t.Errorf("GetOrder = %v\nwant %v", resp, want)
	}
}

func TestGetOrderErrors(t *testing.T) {
	if _, err := client(t, &fakeOrders{}, caller).GetOrder(context.Background(), &orderv1.GetOrderRequest{OrderId: "abc"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("abc: %v", err)
	}
	f := &fakeOrders{err: domain.ErrNotFound}
	if _, err := client(t, f, caller).GetOrder(context.Background(), &orderv1.GetOrderRequest{OrderId: "0192f000-0000-7000-8000-0000000000aa"}); status.Code(err) != codes.NotFound {
		t.Errorf("not found: %v", err)
	}
	if _, err := client(t, &fakeOrders{}, nil).ListMyOrders(context.Background(), &orderv1.ListMyOrdersRequest{Page: 1, PageSize: 1}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("no caller: %v", err)
	}
}
