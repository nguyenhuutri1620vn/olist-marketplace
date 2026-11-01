// Package repository is order-service's Postgres adapter.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/eventbus"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/outbox"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
)

// Postgres stores Orders.
type Postgres struct {
	pool *pgxpool.Pool
}

// New returns a repository on pool.
func New(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

// CreateCheckout writes the Checkout, its Orders, their items, first
// timeline entries and one OrderCreated outbox row per Order in one
// transaction. A key the Customer already used is domain.ErrCheckoutExists and writes
// nothing; the key's row lock makes a concurrent duplicate wait for the
// first and then see it.
func (r *Postgres) CreateCheckout(ctx context.Context, co domain.Checkout, checkoutID string, orders []domain.Order) error {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO checkouts (customer_id, idempotency_key, request_hash, checkout_id)
			VALUES ($1, $2, $3, $4) ON CONFLICT (customer_id, idempotency_key) DO NOTHING`, co.CustomerID, co.Key, co.Hash(), checkoutID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrCheckoutExists
		}
		for _, o := range orders {
			if err := insertOrder(ctx, tx, o); err != nil {
				return err
			}
			ev := eventsv1.OrderCreated{FromStatus: "", ToStatus: domain.StatusCreated, Order: Snapshot(o)}
			if err := outbox.Write(ctx, tx, ev, eventbus.Meta{OccurredAt: o.PurchasedAt}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, domain.ErrCheckoutExists) {
		return fmt.Errorf("create checkout: %w", err)
	}
	return err
}

func insertOrder(ctx context.Context, tx pgx.Tx, o domain.Order) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO orders (id, customer_id, checkout_id, status, purchased_at, historical, voucher_amount)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		o.ID, o.CustomerID, o.CheckoutID, o.Status, o.PurchasedAt, o.Historical, o.VoucherAmount); err != nil {
		return err
	}
	for i, it := range o.Items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_items (order_id, line, product_id, seller_id, seller_state, category_en, price, freight)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			o.ID, i+1, it.ProductID, it.SellerID, it.SellerState, it.CategoryEN, it.Price, it.Freight); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `INSERT INTO order_status_timeline (order_id, status, at) VALUES ($1, $2, $3)`, o.ID, o.Status, o.PurchasedAt)
	return err
}

// Snapshot is the AD-10 Order snapshot carried by every Order fact.
func Snapshot(o domain.Order) eventsv1.OrderSnapshot {
	items := make([]eventsv1.OrderItem, len(o.Items))
	for i, it := range o.Items {
		items[i] = eventsv1.OrderItem{
			ProductID: it.ProductID, SellerID: it.SellerID, SellerState: it.SellerState,
			CategoryEN: it.CategoryEN, Price: it.Price, Freight: it.Freight,
		}
	}
	return eventsv1.OrderSnapshot{
		OrderID: o.ID, CustomerID: o.CustomerID, CheckoutID: o.CheckoutID, PurchasedAt: o.PurchasedAt.UTC(),
		Historical: o.Historical, Status: o.Status, VoucherAmount: o.VoucherAmount, Items: items,
	}
}

// FindCheckout returns the Customer's Checkout for key with its Orders
// sorted by seller id.
func (r *Postgres) FindCheckout(ctx context.Context, customerID, key string) (domain.Placed, bool, error) {
	var s domain.Placed
	err := r.pool.QueryRow(ctx, `
		SELECT checkout_id::text, request_hash FROM checkouts WHERE customer_id = $1 AND idempotency_key = $2`,
		customerID, key).Scan(&s.CheckoutID, &s.Hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Placed{}, false, nil
	}
	if err != nil {
		return domain.Placed{}, false, fmt.Errorf("find checkout: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT o.id::text, o.customer_id::text, o.status, o.purchased_at, o.historical, o.voucher_amount,
			replace(i.product_id::text, '-', ''), replace(i.seller_id::text, '-', ''), i.seller_state, i.category_en, i.price, i.freight
		FROM orders o JOIN order_items i ON i.order_id = o.id
		WHERE o.checkout_id = $1
		ORDER BY i.seller_id, o.id, i.line`, s.CheckoutID)
	if err != nil {
		return domain.Placed{}, false, fmt.Errorf("find checkout orders: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var o domain.Order
		var it domain.Item
		if err := rows.Scan(&o.ID, &o.CustomerID, &o.Status, &o.PurchasedAt, &o.Historical, &o.VoucherAmount,
			&it.ProductID, &it.SellerID, &it.SellerState, &it.CategoryEN, &it.Price, &it.Freight); err != nil {
			return domain.Placed{}, false, fmt.Errorf("find checkout orders: %w", err)
		}
		o.PurchasedAt = o.PurchasedAt.UTC()
		if n := len(s.Orders); n == 0 || s.Orders[n-1].ID != o.ID {
			o.CheckoutID, o.SellerID = s.CheckoutID, it.SellerID
			s.Orders = append(s.Orders, o)
		}
		last := &s.Orders[len(s.Orders)-1]
		last.Items = append(last.Items, it)
	}
	if err := rows.Err(); err != nil {
		return domain.Placed{}, false, fmt.Errorf("find checkout orders: %w", err)
	}
	return s, true, nil
}
