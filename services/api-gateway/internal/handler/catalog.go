package handler

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"google.golang.org/grpc"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
)

// catalogTimeout bounds the catalog call behind each browsing route.
const catalogTimeout = 5 * time.Second

// Defaults when the query leaves the page out (UX-DR8: 20 per page).
const (
	defaultPage     = 1
	defaultPageSize = 20
)

type category struct {
	NamePT string `json:"name_pt"`
	NameEN string `json:"name_en"`
}

type product struct {
	ProductID      string `json:"product_id"`
	DisplayName    string `json:"display_name"`
	CategoryNamePT string `json:"category_name_pt"`
	CategoryNameEN string `json:"category_name_en"`
	SellerID       string `json:"seller_id"`
	SellerCity     string `json:"seller_city"`
	SellerState    string `json:"seller_state"`
	PriceCentavos  int64  `json:"price_centavos"`
}

type productPage struct {
	Products []product `json:"products"`
	Total    int64     `json:"total"`
	Page     int32     `json:"page"`
	PageSize int32     `json:"page_size"`
}

// CatalogBrowser is the part of CatalogService the browsing routes call.
type CatalogBrowser interface {
	ListCategories(ctx context.Context, in *catalogv1.ListCategoriesRequest, opts ...grpc.CallOption) (*catalogv1.ListCategoriesResponse, error)
	ListProducts(ctx context.Context, in *catalogv1.ListProductsRequest, opts ...grpc.CallOption) (*catalogv1.ListProductsResponse, error)
}

// RegisterCatalog adds the public GET /api/categories and
// GET /api/products?category=&page=&page_size= (FR13, NFR7). catalog checks
// the page bounds and the Category.
func RegisterCatalog(mux *http.ServeMux, catalog CatalogBrowser) {
	mux.HandleFunc("GET /api/categories", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), catalogTimeout)
		defer cancel()
		resp, err := catalog.ListCategories(ctx, &catalogv1.ListCategoriesRequest{})
		if err != nil {
			WriteGRPCError(w, err)
			return
		}
		cats := make([]category, len(resp.GetCategories()))
		for i, c := range resp.GetCategories() {
			cats[i] = category{NamePT: c.GetNamePt(), NameEN: c.GetNameEn()}
		}
		writeJSON(w, http.StatusOK, map[string]any{"categories": cats})
	})
	mux.HandleFunc("GET /api/products", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, ok := intParam(w, q, "page", defaultPage)
		if !ok {
			return
		}
		size, ok := intParam(w, q, "page_size", defaultPageSize)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), catalogTimeout)
		defer cancel()
		resp, err := catalog.ListProducts(ctx, &catalogv1.ListProductsRequest{Category: q.Get("category"), Page: page, PageSize: size})
		if err != nil {
			WriteGRPCError(w, err)
			return
		}
		out := productPage{Products: make([]product, len(resp.GetProducts())), Total: resp.GetTotal(), Page: page, PageSize: size}
		for i, p := range resp.GetProducts() {
			out.Products[i] = product{
				ProductID:      p.GetProductId(),
				DisplayName:    p.GetDisplayName(),
				CategoryNamePT: p.GetCategoryNamePt(),
				CategoryNameEN: p.GetCategoryNameEn(),
				SellerID:       p.GetSellerId(),
				SellerCity:     p.GetSellerCity(),
				SellerState:    p.GetSellerState(),
				PriceCentavos:  p.GetPriceCentavos(),
			}
		}
		writeJSON(w, http.StatusOK, out)
	})
}

// intParam reads an optional int32 query parameter; a present value that is
// not an integer answers 400.
func intParam(w http.ResponseWriter, q url.Values, name string, def int32) (int32, bool) {
	if !q.Has(name) {
		return def, true
	}
	n, err := strconv.ParseInt(q.Get(name), 10, 32)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ARGUMENT", name+" must be an integer")
		return 0, false
	}
	return int32(n), true
}
