package transport_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/service"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/transport"
)

// fakeCatalog records the query that reaches the repository.
type fakeCatalog struct {
	cats      []domain.Category
	page      domain.ProductPage
	query     *domain.ProductQuery
	snapshots []domain.ProductSnapshot
	ids       []string
	err       error
}

func (f *fakeCatalog) Categories(context.Context) ([]domain.Category, error) { return f.cats, f.err }
func (f *fakeCatalog) ListProducts(_ context.Context, q domain.ProductQuery) (domain.ProductPage, error) {
	f.query = &q
	return f.page, f.err
}

func (f *fakeCatalog) GetProducts(_ context.Context, ids []string) ([]domain.ProductSnapshot, error) {
	f.ids = ids
	return f.snapshots, f.err
}

func catalogClient(t *testing.T, repo *fakeCatalog) catalogv1.CatalogServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	transport.RegisterCatalog(srv, service.NewBrowser(repo))
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

func TestListCategoriesMapsEveryCategory(t *testing.T) {
	c := catalogClient(t, &fakeCatalog{cats: []domain.Category{
		{NamePT: "beleza_saude", NameEN: "health_beauty"},
		{NamePT: domain.Uncategorized, NameEN: domain.Uncategorized},
	}})
	resp, err := c.ListCategories(context.Background(), &catalogv1.ListCategoriesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := &catalogv1.ListCategoriesResponse{Categories: []*catalogv1.Category{
		{NamePt: "beleza_saude", NameEn: "health_beauty"},
		{NamePt: domain.Uncategorized, NameEn: domain.Uncategorized},
	}}
	if !proto.Equal(resp, want) {
		t.Errorf("ListCategories = %v, want %v", resp, want)
	}
}

func TestListProductsMapsQueryAndPage(t *testing.T) {
	repo := &fakeCatalog{page: domain.ProductPage{Total: 41, Products: []domain.ProductSummary{{
		ID: productID, DisplayName: "health_beauty #1e9e",
		Category: domain.Category{NamePT: "beleza_saude", NameEN: "health_beauty"},
		Seller:   domain.Seller{ID: sellerID, City: "campinas", State: "SP"},
		Price:    5890,
	}}}}
	c := catalogClient(t, repo)
	resp, err := c.ListProducts(context.Background(), &catalogv1.ListProductsRequest{Category: "beleza_saude", Page: 3, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if q := repo.query; q == nil || *q != (domain.ProductQuery{Category: "beleza_saude", Page: 3, PageSize: 20}) {
		t.Errorf("query = %+v", q)
	}
	want := &catalogv1.ListProductsResponse{Total: 41, Products: []*catalogv1.ProductSummary{{
		ProductId: productID, DisplayName: "health_beauty #1e9e",
		CategoryNamePt: "beleza_saude", CategoryNameEn: "health_beauty",
		SellerId: sellerID, SellerCity: "campinas", SellerState: "SP",
		PriceCentavos: 5890,
	}}}
	if !proto.Equal(resp, want) {
		t.Errorf("ListProducts = %v, want %v", resp, want)
	}
}

func TestListProductsRejectsBadPagesWithoutQuerying(t *testing.T) {
	for _, req := range []*catalogv1.ListProductsRequest{
		{Page: 0, PageSize: 20},
		{Page: 1, PageSize: 0},
		{Page: 1, PageSize: 101},
	} {
		repo := &fakeCatalog{}
		_, err := catalogClient(t, repo).ListProducts(context.Background(), req)
		if status.Code(err) != codes.InvalidArgument || repo.query != nil {
			t.Errorf("%v: err %v, repo queried %v; want InvalidArgument without a query", req, err, repo.query != nil)
		}
	}
}

func TestListMapsRepositoryErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want codes.Code
	}{
		{fmt.Errorf("list products: %w: unknown category", domain.ErrInvalid), codes.InvalidArgument},
		{errors.New("connection reset"), codes.Internal},
	} {
		c := catalogClient(t, &fakeCatalog{err: tc.err})
		_, err := c.ListProducts(context.Background(), &catalogv1.ListProductsRequest{Category: "x", Page: 1, PageSize: 20})
		if got := status.Code(err); got != tc.want {
			t.Errorf("ListProducts with repo error %v: code = %v, want %v", tc.err, got, tc.want)
		}
		_, err = c.ListCategories(context.Background(), &catalogv1.ListCategoriesRequest{})
		if got := status.Code(err); got != tc.want {
			t.Errorf("ListCategories with repo error %v: code = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestGetProductsMapsSnapshotsAndValidatesIDs(t *testing.T) {
	repo := &fakeCatalog{snapshots: []domain.ProductSnapshot{{ID: productID, SellerID: sellerID, SellerState: "SP", CategoryEN: "health_beauty", Price: 5890, Freight: 1329}}}
	c := catalogClient(t, repo)
	resp, err := c.GetProducts(context.Background(), &catalogv1.GetProductsRequest{ProductIds: []string{productID}})
	if err != nil {
		t.Fatal(err)
	}
	want := &catalogv1.GetProductsResponse{Products: []*catalogv1.ProductSnapshot{{
		ProductId: productID, SellerId: sellerID, SellerState: "SP", CategoryNameEn: "health_beauty", PriceCentavos: 5890, FreightCentavos: 1329,
	}}}
	if !proto.Equal(resp, want) || !slices.Equal(repo.ids, []string{productID}) {
		t.Errorf("GetProducts = %v (repo asked %v)", resp, repo.ids)
	}
	repo.ids = nil
	if _, err := c.GetProducts(context.Background(), &catalogv1.GetProductsRequest{}); status.Code(err) != codes.InvalidArgument || repo.ids != nil {
		t.Errorf("empty list: %v, repo asked %v", err, repo.ids)
	}
}
