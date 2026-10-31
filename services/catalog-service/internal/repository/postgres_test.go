package repository_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/dbmigrate"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/repository"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/migrations"
)

const (
	product1 = "1e9e8ef04dbcff4541ed26657ea517e5"
	product2 = "3aa071139cb16b67ca9e5dea641aaa2f"
	seller1  = "3442f8959a84dea7ee197c632cb2df15"
	seller2  = "d1b65fc7debc3361ea86b5f14c68d2e2"
)

// newRepo migrates a fresh schema (dropped after the test) and returns a
// repository on it. It skips the test when DBMIGRATE_TEST_DSN is not set.
func newRepo(t *testing.T) (*repository.Postgres, *pgxpool.Pool) {
	t.Helper()
	base := os.Getenv("DBMIGRATE_TEST_DSN")
	if base == "" {
		t.Skip("DBMIGRATE_TEST_DSN not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("catalog_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	if err := dbmigrate.Up(ctx, u.String(), migrations.FS); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return repository.New(pool), pool
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func seed(t *testing.T, repo *repository.Postgres) {
	t.Helper()
	ctx := context.Background()
	cats := []domain.Category{
		must(domain.NewCategory("beleza_saude", "health_beauty")),
		must(domain.NewCategory("pc_gamer", "")),
	}
	if _, err := repo.UpsertCategories(ctx, cats); err != nil {
		t.Fatal(err)
	}
	sellers := []domain.Seller{
		must(domain.NewSeller(seller1, "campinas", "SP")),
		must(domain.NewSeller(seller2, "rio de janeiro", "RJ")),
	}
	if _, err := repo.UpsertSellers(ctx, sellers); err != nil {
		t.Fatal(err)
	}
}

func TestUncategorizedIsARealCategory(t *testing.T) {
	_, pool := newRepo(t)
	var en string
	err := pool.QueryRow(context.Background(), `SELECT name_en FROM category WHERE name_pt = $1`, domain.Uncategorized).Scan(&en)
	if err != nil || en != domain.Uncategorized {
		t.Fatalf("Uncategorized row: name_en %q, err %v", en, err)
	}
}

func TestUpsertProductsDerivesDisplayNameAndIsIdempotent(t *testing.T) {
	repo, pool := newRepo(t)
	seed(t, repo)
	ctx := context.Background()
	products := []domain.Product{
		must(domain.NewProduct(product1, "beleza_saude", seller1, 5890, 1329)),
		must(domain.NewProduct(product2, "", seller2, 760, 0)),
	}
	for i := range 2 {
		n, err := repo.UpsertProducts(ctx, products)
		if err != nil || n != 2 {
			t.Fatalf("run %d: UpsertProducts = %d, %v", i+1, n, err)
		}
	}
	if got := count(t, pool, "product"); got != 2 {
		t.Fatalf("product rows = %d, want 2", got)
	}

	want := map[string]struct {
		name           string
		price, freight int64
		seller         string
	}{
		product1: {"health_beauty #1e9e", 5890, 1329, seller1},
		product2: {"Uncategorized #3aa0", 760, 0, seller2},
	}
	for id, w := range want {
		var name, seller string
		var price, freight int64
		err := pool.QueryRow(ctx, `SELECT display_name, price, freight, replace(seller_id::text, '-', '') FROM product WHERE id = $1`, id).
			Scan(&name, &price, &freight, &seller)
		if err != nil {
			t.Fatal(err)
		}
		if name != w.name || price != w.price || freight != w.freight || seller != w.seller {
			t.Errorf("%s = (%q, %d, %d, %s), want %+v", id, name, price, freight, seller, w)
		}
	}
}

func TestUpsertProductsUpdatesChangedValues(t *testing.T) {
	repo, pool := newRepo(t)
	seed(t, repo)
	ctx := context.Background()
	if _, err := repo.UpsertProducts(ctx, []domain.Product{must(domain.NewProduct(product1, "beleza_saude", seller1, 100, 10))}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertProducts(ctx, []domain.Product{must(domain.NewProduct(product1, "pc_gamer", seller2, 200, 20))}); err != nil {
		t.Fatal(err)
	}
	var name string
	var price int64
	if err := pool.QueryRow(ctx, `SELECT display_name, price FROM product WHERE id = $1`, product1).Scan(&name, &price); err != nil {
		t.Fatal(err)
	}
	if name != "pc_gamer #1e9e" || price != 200 {
		t.Errorf("after update = (%q, %d), want (pc_gamer #1e9e, 200)", name, price)
	}
}

func TestRenamedCategoryRenamesItsProducts(t *testing.T) {
	repo, pool := newRepo(t)
	seed(t, repo)
	ctx := context.Background()
	if _, err := repo.UpsertProducts(ctx, []domain.Product{must(domain.NewProduct(product1, "pc_gamer", seller1, 1, 1))}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertCategories(ctx, []domain.Category{must(domain.NewCategory("pc_gamer", "pc_gamer_en"))}); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT display_name FROM product WHERE id = $1`, product1).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "pc_gamer_en #1e9e" {
		t.Errorf("display name = %q, want pc_gamer_en #1e9e", name)
	}
}

func TestUpsertSellersAndCategoriesAreIdempotent(t *testing.T) {
	repo, pool := newRepo(t)
	seed(t, repo)
	seed(t, repo)
	if got := count(t, pool, "seller"); got != 2 {
		t.Errorf("seller rows = %d, want 2", got)
	}
	if got := count(t, pool, "category"); got != 3 {
		t.Errorf("category rows = %d, want 3 (2 + Uncategorized)", got)
	}
}

func TestUpsertProductsRejectsBatchWithUnknownReference(t *testing.T) {
	repo, pool := newRepo(t)
	seed(t, repo)
	ctx := context.Background()
	for name, bad := range map[string]domain.Product{
		"unknown category": must(domain.NewProduct(product2, "no_such_category", seller1, 1, 1)),
		"unknown seller":   must(domain.NewProduct(product2, "beleza_saude", "00000000000000000000000000000000", 1, 1)),
		"duplicate id":     must(domain.NewProduct(product1, "beleza_saude", seller1, 2, 2)),
	} {
		batch := []domain.Product{must(domain.NewProduct(product1, "beleza_saude", seller1, 1, 1)), bad}
		if _, err := repo.UpsertProducts(ctx, batch); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
		if got := count(t, pool, "product"); got != 0 {
			t.Errorf("%s: product rows = %d, want 0 (batch rolled back)", name, got)
		}
	}
}

func TestRejectedBatchNamesTheOffendingKey(t *testing.T) {
	repo, _ := newRepo(t)
	seed(t, repo)
	missing := "00000000000000000000000000000abc"
	_, err := repo.UpsertProducts(context.Background(), []domain.Product{must(domain.NewProduct(product1, "beleza_saude", missing, 1, 1))})
	if err == nil || !strings.Contains(err.Error(), "00000000-0000-0000-0000-000000000abc") {
		t.Errorf("err = %v, want it to name the missing seller", err)
	}
}
