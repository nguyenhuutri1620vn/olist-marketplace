package transport_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/service"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/transport"
)

const (
	productID = "1e9e8ef04dbcff4541ed26657ea517e5"
	sellerID  = "3442f8959a84dea7ee197c632cb2df15"
)

// fakeRepo records what reaches the repository and returns err.
type fakeRepo struct {
	cats     []domain.Category
	sellers  []domain.Seller
	products []domain.Product
	err      error
}

func (f *fakeRepo) UpsertCategories(_ context.Context, c []domain.Category) (int, error) {
	f.cats = c
	return len(c), f.err
}
func (f *fakeRepo) UpsertSellers(_ context.Context, s []domain.Seller) (int, error) {
	f.sellers = s
	return len(s), f.err
}
func (f *fakeRepo) UpsertProducts(_ context.Context, p []domain.Product) (int, error) {
	f.products = p
	return len(p), f.err
}

func client(t *testing.T, repo *fakeRepo) catalogv1.CatalogAdminServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	transport.RegisterAdmin(srv, service.NewImporter(repo))
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return catalogv1.NewCatalogAdminServiceClient(conn)
}

func TestImportMapsRequestsToDomain(t *testing.T) {
	repo := &fakeRepo{}
	c := client(t, repo)
	ctx := context.Background()

	cr, err := c.ImportCategories(ctx, &catalogv1.ImportCategoriesRequest{Categories: []*catalogv1.ImportCategory{{NamePt: "pc_gamer"}}})
	if err != nil || cr.GetUpserted() != 1 {
		t.Fatalf("ImportCategories = %v, %v", cr, err)
	}
	if got := repo.cats[0]; got.NameEN != "pc_gamer" {
		t.Errorf("category = %+v, want English name falling back to pc_gamer", got)
	}

	sr, err := c.ImportSellers(ctx, &catalogv1.ImportSellersRequest{Sellers: []*catalogv1.ImportSeller{{SellerId: sellerID, City: "campinas", State: "SP"}}})
	if err != nil || sr.GetUpserted() != 1 || repo.sellers[0].City != "campinas" {
		t.Fatalf("ImportSellers = %v, %v, repo %+v", sr, err, repo.sellers)
	}

	pr, err := c.ImportProducts(ctx, &catalogv1.ImportProductsRequest{Products: []*catalogv1.ImportProduct{
		{ProductId: productID, SellerId: sellerID, PriceCentavos: 5890, FreightCentavos: 1329},
	}})
	if err != nil || pr.GetUpserted() != 1 {
		t.Fatalf("ImportProducts = %v, %v", pr, err)
	}
	want := domain.Product{ID: productID, Category: domain.Uncategorized, SellerID: sellerID, Price: 5890, Freight: 1329}
	if repo.products[0] != want {
		t.Errorf("product = %+v, want %+v", repo.products[0], want)
	}
}

func TestImportRejectsInvalidInput(t *testing.T) {
	c := client(t, &fakeRepo{})
	ctx := context.Background()
	calls := map[string]func() error{
		"category without name": func() error {
			_, err := c.ImportCategories(ctx, &catalogv1.ImportCategoriesRequest{Categories: []*catalogv1.ImportCategory{{NameEn: "x"}}})
			return err
		},
		"seller bad id": func() error {
			_, err := c.ImportSellers(ctx, &catalogv1.ImportSellersRequest{Sellers: []*catalogv1.ImportSeller{{SellerId: "x", State: "SP"}}})
			return err
		},
		"product negative price": func() error {
			_, err := c.ImportProducts(ctx, &catalogv1.ImportProductsRequest{Products: []*catalogv1.ImportProduct{{ProductId: productID, SellerId: sellerID, PriceCentavos: -1}}})
			return err
		},
	}
	for name, call := range calls {
		if got := status.Code(call()); got != codes.InvalidArgument {
			t.Errorf("%s: code = %v, want InvalidArgument", name, got)
		}
	}
}

func TestImportMapsRepositoryErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want codes.Code
	}{
		{fmt.Errorf("upsert: %w", domain.ErrInvalid), codes.InvalidArgument},
		{errors.New("connection reset"), codes.Internal},
	} {
		c := client(t, &fakeRepo{err: tc.err})
		_, err := c.ImportSellers(context.Background(), &catalogv1.ImportSellersRequest{Sellers: []*catalogv1.ImportSeller{{SellerId: sellerID, State: "SP"}}})
		if got := status.Code(err); got != tc.want {
			t.Errorf("repo error %v: code = %v, want %v", tc.err, got, tc.want)
		}
	}
}
