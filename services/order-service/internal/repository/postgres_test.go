package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/dbmigrate"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/repository"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/migrations"
)

const (
	customer = "0192f000-0000-7000-8000-000000000c01"
	p1       = "1e9e8ef04dbcff4541ed26657ea517e5"
	p2       = "3aa071139cb16b67ca9e5dea641aaa2f"
	sellerA  = "3442f8959a84dea7ee197c632cb2df15"
	sellerB  = "d1b65fc7debc3361ea86b5f14c68d2e2"
)

// newRepo migrates a fresh schema (dropped after the test). It skips the
// test when DBMIGRATE_TEST_DSN is not set.
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
	schema := fmt.Sprintf("order_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })
	u, _ := url.Parse(base)
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

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

var purchased = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

func plan(t *testing.T, key string) (domain.Checkout, string, []domain.Order) {
	t.Helper()
	co, err := domain.NewCheckout(customer, key, []string{p1, p2})
	if err != nil {
		t.Fatal(err)
	}
	checkoutID := uuid.Must(uuid.NewV7()).String()
	orders, err := co.Plan(checkoutID, []domain.Item{
		{ProductID: p1, SellerID: sellerB, SellerState: "RJ", CategoryEN: "health_beauty", Price: 5890, Freight: 1329},
		{ProductID: p2, SellerID: sellerA, SellerState: "SP", CategoryEN: "Uncategorized", Price: 760, Freight: 0},
	}, purchased, func() string { return uuid.Must(uuid.NewV7()).String() })
	if err != nil {
		t.Fatal(err)
	}
	return co, checkoutID, orders
}

func TestCreateCheckoutWritesOrdersTimelineAndOutboxTogether(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	co, checkoutID, orders := plan(t, "key-1")
	if err := repo.CreateCheckout(ctx, co, checkoutID, orders); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int{"checkouts": 1, "orders": 2, "order_items": 2, "order_status_timeline": 2, "outbox": 2} {
		if got := count(t, pool, table); got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}
	rows, err := pool.Query(ctx, `SELECT topic, envelope FROM outbox ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var topic string
		var env eventsv1.Envelope
		if err := rows.Scan(&topic, &env); err != nil {
			t.Fatal(err)
		}
		var ev eventsv1.OrderCreated
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			t.Fatal(err)
		}
		o := orders[i]
		if topic != eventsv1.TopicOrderEvents || env.EventType != "OrderCreated" || env.AggregateID != o.ID || !env.OccurredAt.Equal(purchased) {
			t.Errorf("envelope %d: topic %s %+v", i, topic, env)
		}
		s := ev.Order
		if ev.FromStatus != "" || ev.ToStatus != "created" || s.OrderID != o.ID || s.CustomerID != customer || s.CheckoutID != checkoutID ||
			s.Status != "created" || s.Historical || s.VoucherAmount != 0 || !s.PurchasedAt.Equal(purchased) || len(s.Items) != 1 ||
			s.Items[0] != (eventsv1.OrderItem{ProductID: o.Items[0].ProductID, SellerID: o.SellerID, SellerState: o.Items[0].SellerState,
				CategoryEN: o.Items[0].CategoryEN, Price: o.Items[0].Price, Freight: o.Items[0].Freight}) {
			t.Errorf("OrderCreated %d = %+v", i, ev)
		}
		i++
	}
	if i != 2 {
		t.Errorf("outbox events = %d", i)
	}
}

func TestFindCheckoutReturnsTheStoredOrders(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	co, checkoutID, orders := plan(t, "key-1")
	if err := repo.CreateCheckout(ctx, co, checkoutID, orders); err != nil {
		t.Fatal(err)
	}
	got, found, err := repo.FindCheckout(ctx, customer, "key-1")
	if err != nil || !found {
		t.Fatalf("FindCheckout = %v, %v", found, err)
	}
	if got.CheckoutID != checkoutID || got.Hash != co.Hash() || len(got.Orders) != 2 {
		t.Fatalf("stored = %+v", got)
	}
	for i, o := range got.Orders {
		w := orders[i]
		if o.ID != w.ID || o.SellerID != w.SellerID || o.Status != "created" || o.Total() != w.Total() ||
			!o.PurchasedAt.Equal(purchased) || len(o.Items) != len(w.Items) || o.Items[0] != w.Items[0] {
			t.Errorf("order %d = %+v, want %+v", i, o, w)
		}
	}
	if _, found, _ := repo.FindCheckout(ctx, "0192f000-0000-7000-8000-000000000c02", "key-1"); found {
		t.Error("another Customer found this Customer's Checkout")
	}
}

func TestASecondCheckoutWithTheSameKeyIsRefusedAndWritesNothing(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	co, checkoutID, orders := plan(t, "key-1")
	if err := repo.CreateCheckout(ctx, co, checkoutID, orders); err != nil {
		t.Fatal(err)
	}
	co2, checkoutID2, orders2 := plan(t, "key-1")
	if err := repo.CreateCheckout(ctx, co2, checkoutID2, orders2); !errors.Is(err, domain.ErrCheckoutExists) {
		t.Errorf("err = %v, want ErrCheckoutExists", err)
	}
	if count(t, pool, "orders") != 2 || count(t, pool, "outbox") != 2 {
		t.Errorf("orders %d outbox %d after the refused Checkout", count(t, pool, "orders"), count(t, pool, "outbox"))
	}
}

const p3 = "96bd76ec8810374ed1b65e291975717f"

// A Seller with several Products: the replay keeps one Order, items in line
// order, and the same total.
func TestFindCheckoutKeepsMultiItemOrdersTogether(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	co, _ := domain.NewCheckout(customer, "multi", []string{p3, p1, p2})
	checkoutID := uuid.Must(uuid.NewV7()).String()
	orders, err := co.Plan(checkoutID, []domain.Item{
		{ProductID: p1, SellerID: sellerB, SellerState: "RJ", CategoryEN: "health_beauty", Price: 5890, Freight: 1329},
		{ProductID: p2, SellerID: sellerA, SellerState: "SP", CategoryEN: "pc_gamer", Price: 760, Freight: 15},
		{ProductID: p3, SellerID: sellerB, SellerState: "RJ", CategoryEN: "Uncategorized", Price: 1000, Freight: 250},
	}, purchased.Add(123456*time.Nanosecond), func() string { return uuid.Must(uuid.NewV7()).String() })
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateCheckout(ctx, co, checkoutID, orders); err != nil {
		t.Fatal(err)
	}
	got, _, err := repo.FindCheckout(ctx, customer, "multi")
	if err != nil || len(got.Orders) != 2 {
		t.Fatalf("FindCheckout = %+v, %v", got, err)
	}
	b := got.Orders[1]
	if b.SellerID != sellerB || len(b.Items) != 2 || b.Items[0].ProductID != p3 || b.Items[1].ProductID != p1 || b.Total() != orders[1].Total() {
		t.Errorf("seller B order = %+v, want items [p3 p1] totalling %d", b, orders[1].Total())
	}
	if !b.PurchasedAt.Equal(orders[1].PurchasedAt) || b.PurchasedAt.Location() != time.UTC {
		t.Errorf("purchased_at %v, want %v in UTC", b.PurchasedAt, orders[1].PurchasedAt)
	}
}

// Two requests with one key at once: exactly one commits.
func TestConcurrentCheckoutsWithOneKeyCommitOnce(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	errs := make(chan error, 2)
	for range 2 {
		co, id, orders := plan(t, "race")
		go func() { errs <- repo.CreateCheckout(ctx, co, id, orders) }()
	}
	var ok, taken int
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrCheckoutExists):
			taken++
		default:
			t.Errorf("unexpected error %v", err)
		}
	}
	if ok != 1 || taken != 1 || count(t, pool, "orders") != 2 || count(t, pool, "outbox") != 2 {
		t.Errorf("ok %d taken %d, orders %d outbox %d", ok, taken, count(t, pool, "orders"), count(t, pool, "outbox"))
	}
}
