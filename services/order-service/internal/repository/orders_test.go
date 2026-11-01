package repository_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/repository"
)

const otherCustomer = "0192f000-0000-7000-8000-000000000c02"

// place stores one Checkout for customer at the given time; its Orders
// share purchased_at, so ids break the tie.
func place(t *testing.T, repo *repository.Postgres, customerID, key string, at time.Time) []domain.Order {
	t.Helper()
	co, err := domain.NewCheckout(customerID, key, []string{p1, p2})
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.Must(uuid.NewV7()).String()
	orders, err := co.Plan(id, []domain.Item{
		{ProductID: p1, SellerID: sellerB, SellerState: "RJ", CategoryEN: "health_beauty", Price: 5890, Freight: 1329},
		{ProductID: p2, SellerID: sellerA, SellerState: "SP", CategoryEN: "pc_gamer", Price: 760, Freight: 15},
	}, at, func() string { return uuid.Must(uuid.NewV7()).String() })
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateCheckout(context.Background(), co, id, orders); err != nil {
		t.Fatal(err)
	}
	return orders
}

func page(t *testing.T, repo *repository.Postgres, customerID string, n, size int32) domain.SummaryPage {
	t.Helper()
	p, err := domain.NewPage(n, size)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListOrders(context.Background(), customerID, p)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func ids(p domain.SummaryPage) []string {
	out := make([]string, len(p.Orders))
	for i, o := range p.Orders {
		out[i] = o.ID
	}
	return out
}

func TestListOrdersNewestFirstWithoutRepeatsAcrossPages(t *testing.T) {
	repo, _ := newRepo(t)
	early := place(t, repo, customer, "a", purchased)
	late := place(t, repo, customer, "b", purchased.Add(time.Hour))
	place(t, repo, otherCustomer, "a", purchased.Add(2*time.Hour))

	all := page(t, repo, customer, 1, 20)
	// Newest Checkout first; inside one Checkout, higher id first.
	byIDDesc := func(os []domain.Order) []string {
		s := []string{os[0].ID, os[1].ID}
		slices.Sort(s)
		slices.Reverse(s)
		return s
	}
	want := append(byIDDesc(late), byIDDesc(early)...)
	if !slices.Equal(ids(all), want) || all.Total != 4 {
		t.Fatalf("list = %v total %d, want %v total 4", ids(all), all.Total, want)
	}
	var paged []string
	for n := int32(1); n <= 3; n++ {
		paged = append(paged, ids(page(t, repo, customer, n, 3))...)
	}
	if !slices.Equal(paged, want) {
		t.Errorf("pages of 3 = %v, want %v", paged, want)
	}
	first := all.Orders[0]
	i := slices.IndexFunc(late, func(o domain.Order) bool { return o.ID == first.ID })
	if i < 0 {
		t.Fatalf("first row %s is not from the latest Checkout", first.ID)
	}
	if o := late[i]; first.SellerID != o.SellerID || first.Total != o.Total() || first.ItemCount != 1 || first.Status != "created" || !first.PurchasedAt.Equal(o.PurchasedAt) {
		t.Errorf("summary %+v, want order %+v", first, o)
	}
	if got := page(t, repo, customer, 9, 20); len(got.Orders) != 0 || got.Total != 4 || got.Orders == nil {
		t.Errorf("past the last page: %+v", got)
	}
}

func TestListOrdersShowsOnlyTheCallersOrders(t *testing.T) {
	repo, _ := newRepo(t)
	place(t, repo, customer, "a", purchased)
	if got := page(t, repo, otherCustomer, 1, 20); len(got.Orders) != 0 || got.Total != 0 {
		t.Errorf("other Customer sees %v", ids(got))
	}
}

func TestGetOrderReturnsItemsAndTimelineToItsOwnerOnly(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	o := place(t, repo, customer, "a", purchased)[1]
	got, err := repo.GetOrder(ctx, customer, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != o.ID || got.SellerID != o.SellerID || got.Total() != o.Total() || len(got.Items) != 1 || got.Items[0] != o.Items[0] ||
		!slices.Equal(got.Timeline, []domain.StatusChange{{Status: "created", At: o.PurchasedAt}}) || got.CancelReason != "" {
		t.Errorf("detail %+v", got)
	}
	for name, tc := range map[string]struct{ customer, id string }{
		"another Customer": {otherCustomer, o.ID},
		"unknown id":       {customer, uuid.NewString()},
	} {
		if _, err := repo.GetOrder(ctx, tc.customer, tc.id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
}

// approved and processing share one commit time: status order breaks the tie.
func TestTheTimelineIsOldestFirstWithStatusOrderOnTies(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	o := place(t, repo, customer, "a", purchased)[0]
	paid := purchased.Add(time.Minute)
	exec(t, pool, `INSERT INTO order_status_timeline (order_id, status, at) VALUES ($1, 'processing', $2), ($1, 'approved', $2)`, o.ID, paid)
	exec(t, pool, `UPDATE orders SET status = 'processing' WHERE id = $1`, o.ID)
	got, err := repo.GetOrder(ctx, customer, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.StatusChange{{Status: "created", At: purchased}, {Status: "approved", At: paid}, {Status: "processing", At: paid}}
	if !slices.EqualFunc(got.Timeline, want, func(a, b domain.StatusChange) bool { return a.Status == b.Status && a.At.Equal(b.At) }) || got.Status != "processing" {
		t.Errorf("timeline %+v status %s", got.Timeline, got.Status)
	}
}

func TestACanceledOrderCarriesItsReason(t *testing.T) {
	repo, pool := newRepo(t)
	o := place(t, repo, customer, "a", purchased)[0]
	exec(t, pool, `UPDATE orders SET status = 'canceled', cancel_reason = 'CARD_DECLINED', voucher_restored = true WHERE id = $1`, o.ID)
	got, err := repo.GetOrder(context.Background(), customer, o.ID)
	if err != nil || got.Status != "canceled" || got.CancelReason != "CARD_DECLINED" || !got.VoucherRestored {
		t.Errorf("canceled detail %+v, %v", got, err)
	}
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

// Historical Orders may have no items; they still list, with no Seller.
func TestAnOrderWithoutItemsStillLists(t *testing.T) {
	repo, pool := newRepo(t)
	id := uuid.Must(uuid.NewV7()).String()
	exec(t, pool, `INSERT INTO orders (id, customer_id, status, purchased_at, historical) VALUES ($1, $2, 'canceled', $3, true)`, id, customer, purchased)
	got := page(t, repo, customer, 1, 20)
	if len(got.Orders) != 1 || got.Orders[0].SellerID != "" || got.Orders[0].Total != 0 || got.Orders[0].ItemCount != 0 {
		t.Errorf("item-less Order listed as %+v", got.Orders)
	}
	d, err := repo.GetOrder(context.Background(), customer, id)
	if err != nil || d.SellerID != "" || len(d.Items) != 0 {
		t.Errorf("item-less detail %+v, %v", d, err)
	}
}

// List and detail name the same Seller even for a multi-Seller Historical Order.
func TestListAndDetailAgreeOnTheSeller(t *testing.T) {
	repo, pool := newRepo(t)
	id := uuid.Must(uuid.NewV7()).String()
	exec(t, pool, `INSERT INTO orders (id, customer_id, status, purchased_at, historical) VALUES ($1, $2, 'delivered', $3, true)`, id, customer, purchased)
	exec(t, pool, `INSERT INTO order_items (order_id, line, product_id, seller_id, seller_state, category_en, price, freight)
		VALUES ($1, 1, $2, $3, 'RJ', 'c', 1, 0), ($1, 2, $4, $5, 'SP', 'c', 1, 0)`, id, p1, sellerB, p2, sellerA)
	list := page(t, repo, customer, 1, 20).Orders[0]
	d, err := repo.GetOrder(context.Background(), customer, id)
	if err != nil || list.SellerID != d.SellerID {
		t.Errorf("list seller %s, detail seller %s (%v)", list.SellerID, d.SellerID, err)
	}
}
