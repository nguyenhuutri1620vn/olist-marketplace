package domain_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
)

const (
	customer = "0192f000-0000-7000-8000-000000000c01"
	p1       = "1e9e8ef04dbcff4541ed26657ea517e5"
	p2       = "3aa071139cb16b67ca9e5dea641aaa2f"
	p3       = "96bd76ec8810374ed1b65e291975717f"
	sellerA  = "3442f8959a84dea7ee197c632cb2df15"
	sellerB  = "d1b65fc7debc3361ea86b5f14c68d2e2"
)

func TestNewCheckoutValidates(t *testing.T) {
	if _, err := domain.NewCheckout(customer, "key-1", []string{p1, p2}); err != nil {
		t.Fatal(err)
	}
	many := make([]string, domain.MaxProducts+1)
	for i := range many {
		many[i] = fmt.Sprintf("%032x", i)
	}
	for name, tc := range map[string]struct {
		customer, key string
		ids           []string
	}{
		"empty list":       {customer, "k", nil},
		"repeated id":      {customer, "k", []string{p1, p1}},
		"bad id":           {customer, "k", []string{"nope"}},
		"too many":         {customer, "k", many},
		"no key":           {customer, "", []string{p1}},
		"long key":         {customer, strings.Repeat("k", 201), []string{p1}},
		"customer no uuid": {"bob", "k", []string{p1}},
	} {
		if _, err := domain.NewCheckout(tc.customer, tc.key, tc.ids); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestTheHashIgnoresProductOrder(t *testing.T) {
	a, _ := domain.NewCheckout(customer, "k", []string{p1, p2, p3})
	b, _ := domain.NewCheckout(customer, "k", []string{p3, p1, p2})
	c, _ := domain.NewCheckout(customer, "k", []string{p1, p2})
	if a.Hash() != b.Hash() || a.Hash() == c.Hash() || len(a.Hash()) != 64 {
		t.Errorf("hashes %s %s %s", a.Hash(), b.Hash(), c.Hash())
	}
}

func snapshots() []domain.Item {
	return []domain.Item{
		{ProductID: p1, SellerID: sellerB, SellerState: "RJ", CategoryEN: "health_beauty", Price: 5890, Freight: 1329},
		{ProductID: p2, SellerID: sellerA, SellerState: "SP", CategoryEN: "pc_gamer", Price: 100000, Freight: 2500},
		{ProductID: p3, SellerID: sellerB, SellerState: "RJ", CategoryEN: "Uncategorized", Price: 760, Freight: 0},
	}
}

func TestPlanCreatesOneCreatedOrderPerSeller(t *testing.T) {
	co, _ := domain.NewCheckout(customer, "k", []string{p1, p2, p3})
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	n := 0
	orders, err := co.Plan("0192f000-0000-7000-8000-0000000c4ec0", snapshots(), now, func() string { n++; return fmt.Sprintf("order-%d", n) })
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 {
		t.Fatalf("orders = %d, want 2", len(orders))
	}
	a, b := orders[0], orders[1]
	if a.SellerID != sellerA || b.SellerID != sellerB {
		t.Errorf("orders not sorted by seller: %s, %s", a.SellerID, b.SellerID)
	}
	if a.Total() != 102500 || b.Total() != 5890+1329+760 {
		t.Errorf("totals %d %d", a.Total(), b.Total())
	}
	var bProducts []string
	for _, it := range b.Items {
		bProducts = append(bProducts, it.ProductID)
	}
	if !slices.Equal(bProducts, []string{p1, p3}) {
		t.Errorf("seller B items %v, want [p1 p3]", bProducts)
	}
	for _, o := range orders {
		if o.Status != domain.StatusCreated || o.CustomerID != customer || o.CheckoutID != "0192f000-0000-7000-8000-0000000c4ec0" ||
			!o.PurchasedAt.Equal(now) || o.Historical || o.ID == "" {
			t.Errorf("order %+v", o)
		}
	}
}

func TestPlanRejectsProductsUnknownToCatalog(t *testing.T) {
	co, _ := domain.NewCheckout(customer, "k", []string{p1, p2, "ffff0000000000000000000000000000"})
	_, err := co.Plan("c", snapshots()[:2], time.Now(), func() string { return "o" })
	if !errors.Is(err, domain.ErrInvalid) || !strings.Contains(err.Error(), "ffff0000000000000000000000000000") {
		t.Errorf("err = %v, want ErrInvalid naming the unknown id", err)
	}
}

func TestStatusesAreTheDatasetWords(t *testing.T) {
	want := []string{"created", "approved", "processing", "shipped", "delivered", "canceled"}
	if !slices.Equal(domain.Statuses, want) {
		t.Errorf("Statuses = %v", domain.Statuses)
	}
}

func TestNewCheckoutRejectsKeysPostgresCannotStore(t *testing.T) {
	for _, key := range []string{"a\x00b", "bad\xffutf8"} {
		if _, err := domain.NewCheckout(customer, key, []string{p1}); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("key %q: err = %v, want ErrInvalid", key, err)
		}
	}
}

func TestNewCheckoutCanonicalisesTheCustomer(t *testing.T) {
	co, err := domain.NewCheckout(strings.ToUpper(customer), "k", []string{p1})
	if err != nil || co.CustomerID != customer {
		t.Errorf("customer = %q, %v", co.CustomerID, err)
	}
}

func TestPlanRejectsASnapshotThatBreaksTheCatalogContract(t *testing.T) {
	co, _ := domain.NewCheckout(customer, "k", []string{p1})
	for name, it := range map[string]domain.Item{
		"bad seller":   {ProductID: p1, SellerID: "x", SellerState: "SP", CategoryEN: "c"},
		"bad state":    {ProductID: p1, SellerID: sellerA, SellerState: "Sao Paulo", CategoryEN: "c"},
		"no category":  {ProductID: p1, SellerID: sellerA, SellerState: "SP"},
		"negative fee": {ProductID: p1, SellerID: sellerA, SellerState: "SP", CategoryEN: "c", Freight: -1},
	} {
		_, err := co.Plan("c", []domain.Item{it}, time.Now(), func() string { return "o" })
		if err == nil || errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want a non-client error", name, err)
		}
	}
}

func TestPlanKeepsMicrosecondsInUTC(t *testing.T) {
	co, _ := domain.NewCheckout(customer, "k", []string{p1})
	loc := time.FixedZone("BRT", -3*3600)
	orders, _ := co.Plan("c", snapshots()[:1], time.Date(2026, 11, 1, 7, 0, 0, 123456789, loc), func() string { return "o" })
	if got := orders[0].PurchasedAt; got.Location() != time.UTC || got.Nanosecond() != 123456000 {
		t.Errorf("purchased_at = %v, want UTC truncated to µs", got)
	}
}
