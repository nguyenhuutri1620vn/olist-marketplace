package domain_test

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
)

const (
	productID = "1e9e8ef04dbcff4541ed26657ea517e5"
	sellerID  = "3442f8959a84dea7ee197c632cb2df15"
)

func TestNewCategoryFallsBackToPortugueseName(t *testing.T) {
	c, err := domain.NewCategory("pc_gamer", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.NameEN != "pc_gamer" {
		t.Errorf("NameEN = %q, want pc_gamer", c.NameEN)
	}
	c, err = domain.NewCategory("beleza_saude", "health_beauty")
	if err != nil || c.NameEN != "health_beauty" {
		t.Errorf("translated category = %+v, %v", c, err)
	}
}

func TestNewCategoryRejectsEmptyName(t *testing.T) {
	if _, err := domain.NewCategory("  ", "x"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestNewSellerValidates(t *testing.T) {
	s, err := domain.NewSeller(sellerID, "campinas", "SP")
	if err != nil || s.ID != sellerID || s.State != "SP" {
		t.Fatalf("NewSeller = %+v, %v", s, err)
	}
	for _, tc := range []struct{ id, state string }{
		{"abc", "SP"},
		{"3442f8959a84dea7ee197c632cb2df1z", "SP"},
		{sellerID + "0", "SP"},
		{sellerID, "S"},
		{sellerID, "sp1"},
	} {
		if _, err := domain.NewSeller(tc.id, "city", tc.state); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("NewSeller(%q, %q) err = %v, want ErrInvalid", tc.id, tc.state, err)
		}
	}
}

func TestNewProductDefaultsToUncategorized(t *testing.T) {
	p, err := domain.NewProduct(productID, "", sellerID, 5890, 1329)
	if err != nil {
		t.Fatal(err)
	}
	if p.Category != domain.Uncategorized {
		t.Errorf("Category = %q, want %q", p.Category, domain.Uncategorized)
	}
}

func TestNewProductRejectsBadInput(t *testing.T) {
	for name, tc := range map[string]struct {
		id, seller     string
		price, freight int64
	}{
		"bad id":           {"nothex", sellerID, 1, 1},
		"bad seller":       {productID, "nothex", 1, 1},
		"negative price":   {productID, sellerID, -1, 0},
		"negative freight": {productID, sellerID, 0, -1},
	} {
		if _, err := domain.NewProduct(tc.id, "x", tc.seller, tc.price, tc.freight); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestNewProductQueryChecksPageBounds(t *testing.T) {
	q, err := domain.NewProductQuery("  beleza_saude ", 3, 20)
	if err != nil {
		t.Fatal(err)
	}
	if q.Category != "beleza_saude" || q.Limit() != 20 || q.Offset() != 40 {
		t.Errorf("query = %+v, limit %d, offset %d; want beleza_saude, 20, 40", q, q.Limit(), q.Offset())
	}
	if _, err := domain.NewProductQuery("", 1, domain.MaxPageSize); err != nil {
		t.Errorf("page_size %d rejected: %v", domain.MaxPageSize, err)
	}
	for _, tc := range []struct{ page, size int32 }{{0, 20}, {-1, 20}, {1, 0}, {1, -5}, {1, domain.MaxPageSize + 1}} {
		if _, err := domain.NewProductQuery("", tc.page, tc.size); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("page %d size %d: err = %v, want ErrInvalid", tc.page, tc.size, err)
		}
	}
}

func TestProductQueryOffsetDoesNotOverflow(t *testing.T) {
	q, err := domain.NewProductQuery("", math.MaxInt32, domain.MaxPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(math.MaxInt32-1) * domain.MaxPageSize; q.Offset() != want {
		t.Errorf("Offset = %d, want %d", q.Offset(), want)
	}
}

func TestNewProductIDsValidatesTheList(t *testing.T) {
	ids, err := domain.NewProductIDs([]string{productID, sellerID})
	if err != nil || len(ids) != 2 {
		t.Fatalf("NewProductIDs = %v, %v", ids, err)
	}
	many := make([]string, domain.MaxSnapshotIDs+1)
	for i := range many {
		many[i] = fmt.Sprintf("%032x", i)
	}
	for name, in := range map[string][]string{
		"empty":    nil,
		"bad id":   {"xyz"},
		"repeated": {productID, productID},
		"too many": many,
	} {
		if _, err := domain.NewProductIDs(in); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}
