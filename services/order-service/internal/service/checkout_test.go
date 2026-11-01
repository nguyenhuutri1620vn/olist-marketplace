package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/service"
)

const (
	customer = "0192f000-0000-7000-8000-000000000c01"
	p1       = "1e9e8ef04dbcff4541ed26657ea517e5"
	p2       = "3aa071139cb16b67ca9e5dea641aaa2f"
	sellerA  = "3442f8959a84dea7ee197c632cb2df15"
	sellerB  = "d1b65fc7debc3361ea86b5f14c68d2e2"
)

type fakeCatalog struct {
	calls int
	items []domain.Item
	err   error
}

func (f *fakeCatalog) Snapshots(_ context.Context, ids []string) ([]domain.Item, error) {
	f.calls++
	return f.items, f.err
}

// fakeStore keeps Checkouts in memory; race makes the next create lose to a
// concurrent request that already stored winner.
type fakeStore struct {
	placed  map[string]domain.Placed
	creates int
	race    *domain.Placed
}

func (f *fakeStore) FindCheckout(_ context.Context, customerID, key string) (domain.Placed, bool, error) {
	p, ok := f.placed[customerID+"/"+key]
	return p, ok, nil
}

func (f *fakeStore) CreateCheckout(_ context.Context, co domain.Checkout, checkoutID string, orders []domain.Order) error {
	f.creates++
	k := co.CustomerID + "/" + co.Key
	if f.race != nil {
		f.placed[k] = *f.race
		f.race = nil
		return domain.ErrCheckoutExists
	}
	if _, ok := f.placed[k]; ok {
		return domain.ErrCheckoutExists
	}
	f.placed[k] = domain.Placed{CheckoutID: checkoutID, Hash: co.Hash(), Orders: orders}
	return nil
}

func (f *fakeStore) ListOrders(context.Context, string, domain.Page) (domain.SummaryPage, error) {
	return domain.SummaryPage{}, nil
}
func (f *fakeStore) GetOrder(context.Context, string, string) (domain.Detail, error) {
	return domain.Detail{}, domain.ErrNotFound
}
func (vanishingStore) ListOrders(context.Context, string, domain.Page) (domain.SummaryPage, error) {
	return domain.SummaryPage{}, nil
}
func (vanishingStore) GetOrder(context.Context, string, string) (domain.Detail, error) {
	return domain.Detail{}, domain.ErrNotFound
}

func items() []domain.Item {
	return []domain.Item{
		{ProductID: p1, SellerID: sellerB, SellerState: "RJ", CategoryEN: "health_beauty", Price: 5890, Freight: 1329},
		{ProductID: p2, SellerID: sellerA, SellerState: "SP", CategoryEN: "pc_gamer", Price: 760},
	}
}

func newService(cat *fakeCatalog, store service.Store) *service.Orders {
	n := 0
	return service.NewOrders(cat, store,
		func() time.Time { return time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC) },
		func() string { n++; return "0192f000-0000-7000-8000-00000000000" + string(rune('0'+n)) })
}

func checkout(t *testing.T, ids ...string) domain.Checkout {
	t.Helper()
	co, err := domain.NewCheckout(customer, "key-1", ids)
	if err != nil {
		t.Fatal(err)
	}
	return co
}

func TestCheckoutPlacesOneOrderPerSeller(t *testing.T) {
	store := &fakeStore{placed: map[string]domain.Placed{}}
	got, err := newService(&fakeCatalog{items: items()}, store).Checkout(context.Background(), checkout(t, p1, p2))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Orders) != 2 || got.CheckoutID == "" || store.creates != 1 {
		t.Errorf("placed %+v after %d creates", got, store.creates)
	}
}

func TestReplayReturnsTheOriginalWithoutAskingCatalog(t *testing.T) {
	store := &fakeStore{placed: map[string]domain.Placed{}}
	cat := &fakeCatalog{items: items()}
	s := newService(cat, store)
	first, _ := s.Checkout(context.Background(), checkout(t, p1, p2))
	cat.err = domain.ErrUnavailable // a replay needs no catalog
	again, err := s.Checkout(context.Background(), checkout(t, p2, p1))
	if err != nil || again.CheckoutID != first.CheckoutID || cat.calls != 1 || store.creates != 1 {
		t.Errorf("replay = %+v, %v (catalog calls %d, creates %d)", again.CheckoutID, err, cat.calls, store.creates)
	}
	if _, err := s.Checkout(context.Background(), checkout(t, p1)); !errors.Is(err, domain.ErrKeyReused) {
		t.Errorf("same key, other Products: %v, want ErrKeyReused", err)
	}
}

func TestCatalogFailuresWriteNothing(t *testing.T) {
	for name, cat := range map[string]*fakeCatalog{
		"unavailable":     {err: domain.ErrUnavailable},
		"unknown product": {items: items()[:1]},
	} {
		store := &fakeStore{placed: map[string]domain.Placed{}}
		_, err := newService(cat, store).Checkout(context.Background(), checkout(t, p1, p2))
		if err == nil || store.creates != 0 {
			t.Errorf("%s: err %v, creates %d", name, err, store.creates)
		}
	}
}

// Two requests with one key: the loser returns the winner's Checkout.
func TestALostRaceReturnsTheWinner(t *testing.T) {
	winner := domain.Placed{CheckoutID: "winner", Hash: checkout(t, p1, p2).Hash()}
	store := &fakeStore{placed: map[string]domain.Placed{}, race: &winner}
	got, err := newService(&fakeCatalog{items: items()}, store).Checkout(context.Background(), checkout(t, p1, p2))
	if err != nil || got.CheckoutID != "winner" {
		t.Errorf("lost race = %+v, %v", got, err)
	}
	other := domain.Placed{CheckoutID: "winner", Hash: "another set"}
	store = &fakeStore{placed: map[string]domain.Placed{}, race: &other}
	if _, err := newService(&fakeCatalog{items: items()}, store).Checkout(context.Background(), checkout(t, p1, p2)); !errors.Is(err, domain.ErrKeyReused) {
		t.Errorf("lost race to other Products: %v, want ErrKeyReused", err)
	}
}

// A conflict whose winner cannot be read is an error, never an empty success.
func TestALostRaceWithoutAWinnerIsAnError(t *testing.T) {
	store := vanishingStore{}
	if got, err := newService(&fakeCatalog{items: items()}, store).Checkout(context.Background(), checkout(t, p1, p2)); err == nil {
		t.Errorf("Checkout = %+v, nil; want an error", got)
	}
}

type vanishingStore struct{}

func (vanishingStore) FindCheckout(context.Context, string, string) (domain.Placed, bool, error) {
	return domain.Placed{}, false, nil
}
func (vanishingStore) CreateCheckout(context.Context, domain.Checkout, string, []domain.Order) error {
	return domain.ErrCheckoutExists
}
