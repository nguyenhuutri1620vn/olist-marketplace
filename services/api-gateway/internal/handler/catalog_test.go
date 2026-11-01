package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/api-gateway/internal/handler"
)

// fakeCatalog records the ListProducts request; err, when set, fails every call.
type fakeCatalog struct {
	req      *catalogv1.ListProductsRequest
	products *catalogv1.ListProductsResponse
	err      error
}

func (f *fakeCatalog) ListCategories(context.Context, *catalogv1.ListCategoriesRequest, ...grpc.CallOption) (*catalogv1.ListCategoriesResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &catalogv1.ListCategoriesResponse{Categories: []*catalogv1.Category{
		{NamePt: "beleza_saude", NameEn: "health_beauty"},
		{NamePt: "Uncategorized", NameEn: "Uncategorized"},
	}}, nil
}

func (f *fakeCatalog) ListProducts(_ context.Context, r *catalogv1.ListProductsRequest, _ ...grpc.CallOption) (*catalogv1.ListProductsResponse, error) {
	f.req = r
	if f.err != nil {
		return nil, f.err
	}
	if f.products != nil {
		return f.products, nil
	}
	return &catalogv1.ListProductsResponse{}, nil
}

func serveCatalog(f *fakeCatalog, path string) (*httptest.ResponseRecorder, map[string]any) {
	mux := http.NewServeMux()
	handler.RegisterCatalog(mux, f)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestCategoriesRoute(t *testing.T) {
	rec, _ := serveCatalog(&fakeCatalog{}, "/api/categories")
	want := `{"categories":[{"name_pt":"beleza_saude","name_en":"health_beauty"},{"name_pt":"Uncategorized","name_en":"Uncategorized"}]}`
	if rec.Code != http.StatusOK || rec.Body.String() != want+"\n" {
		t.Errorf("GET /api/categories → %d %s, want 200 %s", rec.Code, rec.Body, want)
	}
}

func TestProductsRouteDefaultsToFirstPageOf20(t *testing.T) {
	f := &fakeCatalog{}
	rec, body := serveCatalog(f, "/api/products")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body)
	}
	if want := (&catalogv1.ListProductsRequest{Page: 1, PageSize: 20}); !proto.Equal(f.req, want) {
		t.Errorf("request = %v, want %v", f.req, want)
	}
	// An empty page is an empty list, never null.
	if p, ok := body["products"].([]any); !ok || len(p) != 0 || body["total"] != 0.0 || body["page"] != 1.0 || body["page_size"] != 20.0 {
		t.Errorf("body = %v, want products [] total 0 page 1 page_size 20", body)
	}
}

func TestProductsRoutePassesFilterAndPage(t *testing.T) {
	f := &fakeCatalog{products: &catalogv1.ListProductsResponse{Total: 610, Products: []*catalogv1.ProductSummary{{
		ProductId: "cccc0000000000000000000000000001", DisplayName: "Uncategorized #cccc",
		CategoryNamePt: "Uncategorized", CategoryNameEn: "Uncategorized",
		SellerId: "6560211a19b47992c3666cc44a7e94c0", SellerCity: "ibitinga", SellerState: "SP",
		PriceCentavos: 12990, FreightCentavos: 1500,
	}}}}
	rec, _ := serveCatalog(f, "/api/products?category=Uncategorized&page=2&page_size=50")
	if want := (&catalogv1.ListProductsRequest{Category: "Uncategorized", Page: 2, PageSize: 50}); !proto.Equal(f.req, want) {
		t.Errorf("request = %v, want %v", f.req, want)
	}
	want := `{"products":[{"product_id":"cccc0000000000000000000000000001","display_name":"Uncategorized #cccc",` +
		`"category_name_pt":"Uncategorized","category_name_en":"Uncategorized",` +
		`"seller_id":"6560211a19b47992c3666cc44a7e94c0","seller_city":"ibitinga","seller_state":"SP",` +
		`"price_centavos":12990,"freight_centavos":1500}],"total":610,"page":2,"page_size":50}`
	if rec.Code != http.StatusOK || rec.Body.String() != want+"\n" {
		t.Errorf("→ %d %s\nwant 200 %s", rec.Code, rec.Body, want)
	}
}

func TestProductsRouteRejectsNonIntegerPagesWithoutCalling(t *testing.T) {
	for _, q := range []string{"page=abc", "page_size=1.5", "page=", "page=99999999999", "page_size=-"} {
		f := &fakeCatalog{}
		rec, body := serveCatalog(f, "/api/products?"+q)
		e, _ := body["error"].(map[string]any)
		if rec.Code != http.StatusBadRequest || e["code"] != "INVALID_ARGUMENT" || f.req != nil {
			t.Errorf("%s → %d %s, called %v; want 400 INVALID_ARGUMENT without a call", q, rec.Code, rec.Body, f.req != nil)
		}
	}
}

func TestProductsRouteMapsServiceErrors(t *testing.T) {
	f := &fakeCatalog{err: status.Error(codes.InvalidArgument, "invalid: unknown category \"x\"")}
	rec, body := serveCatalog(f, "/api/products?category=x")
	e, _ := body["error"].(map[string]any)
	if rec.Code != http.StatusBadRequest || e["message"] != `invalid: unknown category "x"` {
		t.Errorf("→ %d %s, want 400 with the service message", rec.Code, rec.Body)
	}
	rec, _ = serveCatalog(&fakeCatalog{err: status.Error(codes.Unavailable, "dial")}, "/api/categories")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("categories with catalog down → %d, want 503", rec.Code)
	}
}
