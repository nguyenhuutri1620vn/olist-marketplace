// Package service holds order-service's use cases and the ports they use.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
)

// Catalog snapshots Products at Checkout (AD-15). Unknown ids are absent; an
// unreachable or late catalog is domain.ErrUnavailable.
type Catalog interface {
	Snapshots(ctx context.Context, productIDs []string) ([]domain.Item, error)
}

// Store saves Checkouts. CreateCheckout writes the Orders and their
// OrderCreated outbox rows in one transaction, or returns
// domain.ErrCheckoutExists when the Customer already used the key.
type Store interface {
	FindCheckout(ctx context.Context, customerID, key string) (domain.Placed, bool, error)
	CreateCheckout(ctx context.Context, co domain.Checkout, checkoutID string, orders []domain.Order) error
}

// Orders holds the Order use cases.
type Orders struct {
	catalog Catalog
	store   Store
	now     func() time.Time
	newID   func() string
}

// NewOrders returns the use cases; newID makes Order and Checkout ids (UUIDv7).
func NewOrders(catalog Catalog, store Store, now func() time.Time, newID func() string) *Orders {
	return &Orders{catalog: catalog, store: store, now: now, newID: newID}
}

// Checkout places one created Order per Seller (FR1). A key the Customer
// already used returns that Checkout when the Products are the same and
// domain.ErrKeyReused otherwise; nothing new is written either way. It does
// not start the Saga (AD-4).
func (s *Orders) Checkout(ctx context.Context, co domain.Checkout) (domain.Placed, error) {
	if placed, found, err := s.existing(ctx, co); found || err != nil {
		return placed, err
	}
	snaps, err := s.catalog.Snapshots(ctx, co.ProductIDs)
	if err != nil {
		return domain.Placed{}, err
	}
	checkoutID := s.newID()
	orders, err := co.Plan(checkoutID, snaps, s.now(), s.newID)
	if err != nil {
		return domain.Placed{}, err
	}
	err = s.store.CreateCheckout(ctx, co, checkoutID, orders)
	if errors.Is(err, domain.ErrCheckoutExists) {
		// A concurrent request with the same key won; answer with its Checkout.
		placed, found, err := s.existing(ctx, co)
		if err == nil && !found {
			return domain.Placed{}, fmt.Errorf("checkout key %q conflicted but no Checkout is stored", co.Key)
		}
		return placed, err
	}
	if err != nil {
		return domain.Placed{}, err
	}
	return domain.Placed{CheckoutID: checkoutID, Hash: co.Hash(), Orders: orders}, nil
}

// existing returns the Customer's Checkout for co's key, if any.
func (s *Orders) existing(ctx context.Context, co domain.Checkout) (domain.Placed, bool, error) {
	placed, found, err := s.store.FindCheckout(ctx, co.CustomerID, co.Key)
	switch {
	case err != nil:
		return domain.Placed{}, false, err
	case !found:
		return domain.Placed{}, false, nil
	case placed.Hash != co.Hash():
		return domain.Placed{}, true, domain.ErrKeyReused
	}
	return placed, true, nil
}
