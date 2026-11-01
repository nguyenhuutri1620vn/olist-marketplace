package repository_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/repository"
)

// seedBrowse adds five Products: three health_beauty ones that share the
// display name "health_beauty #aaaa", one pc_gamer and one Uncategorized.
func seedBrowse(t *testing.T, repo *repository.Postgres) {
	t.Helper()
	seed(t, repo)
	products := []domain.Product{
		must(domain.NewProduct("aaaa0000000000000000000000000003", "beleza_saude", seller1, 300, 0)),
		must(domain.NewProduct("aaaa0000000000000000000000000001", "beleza_saude", seller2, 100, 0)),
		must(domain.NewProduct("aaaa0000000000000000000000000002", "beleza_saude", seller1, 200, 37)),
		must(domain.NewProduct("bbbb0000000000000000000000000001", "pc_gamer", seller1, 400, 0)),
		must(domain.NewProduct("cccc0000000000000000000000000001", "", seller2, 500, 0)),
	}
	if _, err := repo.UpsertProducts(context.Background(), products); err != nil {
		t.Fatal(err)
	}
}

func list(t *testing.T, repo *repository.Postgres, category string, page, size int32) domain.ProductPage {
	t.Helper()
	q := must(domain.NewProductQuery(category, page, size))
	p, err := repo.ListProducts(context.Background(), q)
	if err != nil {
		t.Fatalf("ListProducts(%q, %d, %d): %v", category, page, size, err)
	}
	return p
}

func ids(p domain.ProductPage) []string {
	out := make([]string, len(p.Products))
	for i, s := range p.Products {
		out[i] = s.ID
	}
	return out
}

func TestCategoriesListsEveryCategoryWithUncategorizedLast(t *testing.T) {
	repo, _ := newRepo(t)
	seed(t, repo)
	cats, err := repo.Categories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Category{
		{NamePT: "beleza_saude", NameEN: "health_beauty"},
		{NamePT: "pc_gamer", NameEN: "pc_gamer"},
		{NamePT: domain.Uncategorized, NameEN: domain.Uncategorized},
	}
	if !slices.Equal(cats, want) {
		t.Errorf("Categories = %+v, want %+v", cats, want)
	}
}

func TestListProductsSortsByDisplayNameThenID(t *testing.T) {
	repo, _ := newRepo(t)
	seedBrowse(t, repo)
	p := list(t, repo, "", 1, 20)
	want := []string{
		"aaaa0000000000000000000000000001", // health_beauty #aaaa (ties broken by id)
		"aaaa0000000000000000000000000002",
		"aaaa0000000000000000000000000003",
		"bbbb0000000000000000000000000001", // pc_gamer #bbbb
		"cccc0000000000000000000000000001", // Uncategorized #cccc
	}
	if !slices.Equal(ids(p), want) || p.Total != 5 {
		t.Fatalf("page = %v total %d, want %v total 5", ids(p), p.Total, want)
	}
	first := p.Products[0]
	if first.DisplayName != "health_beauty #aaaa" || first.Category != (domain.Category{NamePT: "beleza_saude", NameEN: "health_beauty"}) ||
		first.Seller != (domain.Seller{ID: seller2, City: "rio de janeiro", State: "RJ"}) || first.Price != 100 {
		t.Errorf("first row = %+v", first)
	}
	if second := p.Products[1]; second.Price != 200 || second.Freight != 37 {
		t.Errorf("second row price %d freight %d, want 200 and 37", second.Price, second.Freight)
	}
}

func TestListProductsPagesNeverOverlap(t *testing.T) {
	repo, _ := newRepo(t)
	seedBrowse(t, repo)
	var seen []string
	for page := int32(1); page <= 3; page++ {
		p := list(t, repo, "", page, 2)
		if p.Total != 5 {
			t.Errorf("page %d total = %d, want 5", page, p.Total)
		}
		seen = append(seen, ids(p)...)
	}
	all := ids(list(t, repo, "", 1, 20))
	if !slices.Equal(seen, all) {
		t.Errorf("pages of 2 = %v, want %v", seen, all)
	}
}

func TestListProductsFiltersByCategory(t *testing.T) {
	repo, _ := newRepo(t)
	seedBrowse(t, repo)
	p := list(t, repo, domain.Uncategorized, 1, 20)
	if !slices.Equal(ids(p), []string{"cccc0000000000000000000000000001"}) || p.Total != 1 {
		t.Errorf("Uncategorized = %v total %d", ids(p), p.Total)
	}
	if p.Products[0].Category.NameEN != domain.Uncategorized {
		t.Errorf("Uncategorized row category = %+v", p.Products[0].Category)
	}
	if p := list(t, repo, "beleza_saude", 1, 20); len(p.Products) != 3 || p.Total != 3 {
		t.Errorf("beleza_saude = %v total %d, want 3", ids(p), p.Total)
	}
}

func TestListProductsPastTheLastPageIsEmpty(t *testing.T) {
	repo, _ := newRepo(t)
	seedBrowse(t, repo)
	p := list(t, repo, "", 4, 2)
	if len(p.Products) != 0 || p.Total != 5 || p.Products == nil {
		t.Errorf("page 4 of 2 = %v (nil %v) total %d, want an empty list and total 5", ids(p), p.Products == nil, p.Total)
	}
}

func TestListProductsInACategoryWithoutProductsIsEmpty(t *testing.T) {
	repo, _ := newRepo(t)
	seed(t, repo)
	if p := list(t, repo, "pc_gamer", 1, 20); len(p.Products) != 0 || p.Total != 0 {
		t.Errorf("pc_gamer = %v total %d, want empty", ids(p), p.Total)
	}
}

func TestListProductsRejectsAnUnknownCategory(t *testing.T) {
	repo, _ := newRepo(t)
	seedBrowse(t, repo)
	_, err := repo.ListProducts(context.Background(), must(domain.NewProductQuery("no_such_category", 1, 20)))
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestGetProductsReturnsTheCheckoutSnapshotOfKnownIDs(t *testing.T) {
	repo, _ := newRepo(t)
	seedBrowse(t, repo)
	got, err := repo.GetProducts(context.Background(), []string{"cccc0000000000000000000000000001", "aaaa0000000000000000000000000002", "ffff0000000000000000000000000000"})
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.ProductSnapshot{
		{ID: "aaaa0000000000000000000000000002", SellerID: seller1, SellerState: "SP", CategoryEN: "health_beauty", Price: 200, Freight: 37},
		{ID: "cccc0000000000000000000000000001", SellerID: seller2, SellerState: "RJ", CategoryEN: domain.Uncategorized, Price: 500, Freight: 0},
	}
	if !slices.Equal(got, want) {
		t.Errorf("GetProducts = %+v\nwant %+v", got, want)
	}
}
