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

// fakeOrders records the Checkout it got and answers with placed or err.
type fakeOrders struct {
	got    *domain.Checkout
	placed domain.Placed
	err    error
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
