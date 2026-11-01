package catalog_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/catalog"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
)

const (
	p1      = "1e9e8ef04dbcff4541ed26657ea517e5"
	sellerA = "3442f8959a84dea7ee197c632cb2df15"
)

// fakeServer answers GetProducts after delay with resp or err.
type fakeServer struct {
	catalogv1.UnimplementedCatalogServiceServer
	delay time.Duration
	resp  *catalogv1.GetProductsResponse
	err   error
	asked []string
}

func (f *fakeServer) GetProducts(ctx context.Context, req *catalogv1.GetProductsRequest) (*catalogv1.GetProductsResponse, error) {
	f.asked = req.GetProductIds()
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return f.resp, f.err
}

func client(t *testing.T, f *fakeServer) catalogv1.CatalogServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	catalogv1.RegisterCatalogServiceServer(srv, f)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return catalogv1.NewCatalogServiceClient(conn)
}

func TestSnapshotsMapTheCatalogAnswer(t *testing.T) {
	f := &fakeServer{resp: &catalogv1.GetProductsResponse{Products: []*catalogv1.ProductSnapshot{{
		ProductId: p1, SellerId: sellerA, SellerState: "SP", CategoryNameEn: "health_beauty", PriceCentavos: 5890, FreightCentavos: 1329,
	}}}}
	got, err := catalog.New(client(t, f), time.Second).Snapshots(context.Background(), []string{p1})
	want := domain.Item{ProductID: p1, SellerID: sellerA, SellerState: "SP", CategoryEN: "health_beauty", Price: 5890, Freight: 1329}
	if err != nil || len(got) != 1 || got[0] != want || len(f.asked) != 1 {
		t.Errorf("Snapshots = %+v, %v", got, err)
	}
}

func TestADownOrSlowCatalogIsUnavailable(t *testing.T) {
	for name, f := range map[string]*fakeServer{
		"unavailable": {err: status.Error(codes.Unavailable, "connection refused")},
		"exhausted":   {err: status.Error(codes.ResourceExhausted, "busy")},
		"too slow":    {delay: time.Second, resp: &catalogv1.GetProductsResponse{}},
	} {
		start := time.Now()
		_, err := catalog.New(client(t, f), 50*time.Millisecond).Snapshots(context.Background(), []string{p1})
		if !errors.Is(err, domain.ErrUnavailable) || time.Since(start) > 500*time.Millisecond {
			t.Errorf("%s: err %v after %v, want ErrUnavailable within the deadline", name, err, time.Since(start))
		}
	}
}

func TestACatalogRejectionIsInvalid(t *testing.T) {
	f := &fakeServer{err: status.Error(codes.InvalidArgument, "product id is repeated")}
	if _, err := catalog.New(client(t, f), time.Second).Snapshots(context.Background(), []string{p1}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestTimeoutFromEnv(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want time.Duration
		ok   bool
	}{{"", 2 * time.Second, true}, {"500ms", 500 * time.Millisecond, true}, {"0", 0, false}, {"soon", 0, false}} {
		t.Setenv("CATALOG_TIMEOUT", tc.env)
		got, err := catalog.Timeout()
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("CATALOG_TIMEOUT=%q → %v, %v", tc.env, got, err)
		}
	}
}

func TestOnlyTransientCatalogFailuresAreUnavailable(t *testing.T) {
	for name, f := range map[string]*fakeServer{
		"unimplemented": {err: status.Error(codes.Unimplemented, "old catalog")},
		"denied":        {err: status.Error(codes.PermissionDenied, "policy")},
	} {
		_, err := catalog.New(client(t, f), time.Second).Snapshots(context.Background(), []string{p1})
		if err == nil || errors.Is(err, domain.ErrUnavailable) || errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want an internal error", name, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := catalog.New(client(t, &fakeServer{}), time.Second).Snapshots(ctx, []string{p1}); !errors.Is(err, context.Canceled) {
		t.Errorf("caller gone: err = %v, want context.Canceled", err)
	}
}
