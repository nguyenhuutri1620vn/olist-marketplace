package importer_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"google.golang.org/grpc"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/tools/olist-importer/internal/importer"
	"github.com/nguyenhuutri1620vn/olist-marketplace/tools/olist-importer/internal/olist"
)

// fakeCatalog records every call as "<rpc>:<batch size>".
type fakeCatalog struct {
	calls []string
	// noDeadline counts calls whose context had no deadline.
	noDeadline int
	failOn     string
	got        []*catalogv1.ImportProduct
	cats       []*catalogv1.ImportCategory
}

func (f *fakeCatalog) record(rpc string, n int) error {
	f.calls = append(f.calls, fmt.Sprintf("%s:%d", rpc, n))
	if rpc == f.failOn {
		return errors.New("rpc failed")
	}
	return nil
}

func (f *fakeCatalog) ImportCategories(ctx context.Context, r *catalogv1.ImportCategoriesRequest, _ ...grpc.CallOption) (*catalogv1.ImportCategoriesResponse, error) {
	f.checkDeadline(ctx)
	f.cats = append(f.cats, r.GetCategories()...)
	return &catalogv1.ImportCategoriesResponse{Upserted: int32(len(r.GetCategories()))}, f.record("categories", len(r.GetCategories()))
}

func (f *fakeCatalog) ImportSellers(ctx context.Context, r *catalogv1.ImportSellersRequest, _ ...grpc.CallOption) (*catalogv1.ImportSellersResponse, error) {
	f.checkDeadline(ctx)
	return &catalogv1.ImportSellersResponse{Upserted: int32(len(r.GetSellers()))}, f.record("sellers", len(r.GetSellers()))
}

func (f *fakeCatalog) ImportProducts(ctx context.Context, r *catalogv1.ImportProductsRequest, _ ...grpc.CallOption) (*catalogv1.ImportProductsResponse, error) {
	f.checkDeadline(ctx)
	f.got = append(f.got, r.GetProducts()...)
	return &catalogv1.ImportProductsResponse{Upserted: int32(len(r.GetProducts()))}, f.record("products", len(r.GetProducts()))
}

func (f *fakeCatalog) checkDeadline(ctx context.Context) {
	if _, ok := ctx.Deadline(); !ok {
		f.noDeadline++
	}
}

func TestEveryBatchHasADeadline(t *testing.T) {
	f := &fakeCatalog{}
	if _, err := importer.Run(context.Background(), f, dataset(), 3); err != nil {
		t.Fatal(err)
	}
	if f.noDeadline != 0 {
		t.Errorf("%d of %d calls had no deadline, want every batch bounded", f.noDeadline, len(f.calls))
	}
}

func dataset() olist.Dataset {
	ds := olist.Dataset{
		Categories: []olist.Category{{NamePT: "artes", NameEN: "art"}, {NamePT: "pc_gamer"}},
		Skipped:    []string{"x"},
	}
	for i := range 5 {
		ds.Sellers = append(ds.Sellers, olist.Seller{ID: fmt.Sprintf("s%d", i), State: "SP"})
	}
	for i := range 7 {
		ds.Products = append(ds.Products, olist.Product{ID: fmt.Sprintf("p%d", i), Category: "artes", SellerID: "s1", Price: int64(i), Freight: 1})
	}
	return ds
}

func TestRunImportsInDependencyOrderAndBatches(t *testing.T) {
	f := &fakeCatalog{}
	sum, err := importer.Run(context.Background(), f, dataset(), 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"categories:2", "sellers:3", "sellers:2", "products:3", "products:3", "products:1"}
	if !slices.Equal(f.calls, want) {
		t.Errorf("calls = %v, want %v", f.calls, want)
	}
	if sum != (importer.Summary{Categories: 2, Sellers: 5, Products: 7, Skipped: 1}) {
		t.Errorf("summary = %+v", sum)
	}
	p := f.got[6]
	if p.GetProductId() != "p6" || p.GetCategoryNamePt() != "artes" || p.GetSellerId() != "s1" || p.GetPriceCentavos() != 6 || p.GetFreightCentavos() != 1 {
		t.Errorf("last product = %v", p)
	}
	if f.cats[1].GetNamePt() != "pc_gamer" || f.cats[1].GetNameEn() != "" {
		t.Errorf("untranslated category sent as %v, want empty English name", f.cats[1])
	}
}

func TestRunStopsAtFirstFailure(t *testing.T) {
	f := &fakeCatalog{failOn: "sellers"}
	if _, err := importer.Run(context.Background(), f, dataset(), 3); err == nil {
		t.Fatal("Run succeeded, want the sellers error")
	}
	if want := []string{"categories:2", "sellers:3"}; !slices.Equal(f.calls, want) {
		t.Errorf("calls = %v, want %v (no products after a failed seller batch)", f.calls, want)
	}
}
