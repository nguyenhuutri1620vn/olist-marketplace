package domain_test

import (
	"errors"
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
