package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
)

// ListOrders returns one page of the Customer's Orders, newest first
// (purchased_at DESC, id DESC: Orders of one Checkout share purchased_at, so
// the id keeps pages from overlapping), with the count across all pages, both
// from one snapshot.
func (r *Postgres) ListOrders(ctx context.Context, customerID string, p domain.Page) (domain.SummaryPage, error) {
	out := domain.SummaryPage{Orders: []domain.Summary{}}
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM orders WHERE customer_id = $1`, customerID).Scan(&out.Total); err != nil {
			return err
		}
		// One pass over each Order's items; an Order without items (some
		// Historical ones) has no Seller, total 0 and 0 items.
		rows, err := tx.Query(ctx, `
			SELECT o.id::text, o.status, o.purchased_at,
				coalesce(replace(it.seller_id, '-', ''), ''), coalesce(it.total, 0), coalesce(it.items, 0)
			FROM orders o
			LEFT JOIN LATERAL (
				SELECT min(i.seller_id::text) AS seller_id, sum(i.price + i.freight) AS total, count(*) AS items
				FROM order_items i WHERE i.order_id = o.id
			) it ON true
			WHERE o.customer_id = $1
			ORDER BY o.purchased_at DESC, o.id DESC
			LIMIT $2 OFFSET $3`, customerID, p.Limit(), p.Offset())
		if err != nil {
			return err
		}
		list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Summary, error) {
			var s domain.Summary
			err := row.Scan(&s.ID, &s.Status, &s.PurchasedAt, &s.SellerID, &s.Total, &s.ItemCount)
			s.PurchasedAt = s.PurchasedAt.UTC()
			return s, err
		})
		out.Orders = append(out.Orders, list...)
		return err
	})
	if err != nil {
		return domain.SummaryPage{}, fmt.Errorf("list orders: %w", err)
	}
	return out, nil
}

// GetOrder returns the Customer's Order with its items and timeline (oldest
// first, status order on ties). Another Customer's or an unknown id is
// domain.ErrNotFound.
func (r *Postgres) GetOrder(ctx context.Context, customerID, orderID string) (domain.Detail, error) {
	var d domain.Detail
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var checkoutID *string
		var reason *string
		err := tx.QueryRow(ctx, `
			SELECT id::text, customer_id::text, checkout_id::text, status, purchased_at, historical, voucher_amount, cancel_reason, voucher_restored
			FROM orders WHERE id = $1 AND customer_id = $2`, orderID, customerID).
			Scan(&d.ID, &d.CustomerID, &checkoutID, &d.Status, &d.PurchasedAt, &d.Historical, &d.VoucherAmount, &reason, &d.VoucherRestored)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		d.PurchasedAt = d.PurchasedAt.UTC()
		if checkoutID != nil {
			d.CheckoutID = *checkoutID
		}
		if reason != nil {
			d.CancelReason = *reason
		}
		rows, err := tx.Query(ctx, `
			SELECT replace(product_id::text, '-', ''), replace(seller_id::text, '-', ''), seller_state, category_en, price, freight
			FROM order_items WHERE order_id = $1 ORDER BY line`, orderID)
		if err != nil {
			return err
		}
		d.Items, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Item, error) {
			var it domain.Item
			err := row.Scan(&it.ProductID, &it.SellerID, &it.SellerState, &it.CategoryEN, &it.Price, &it.Freight)
			return it, err
		})
		if err != nil {
			return err
		}
		// The lowest Seller id, as in ListOrders: a Historical Order may span
		// several Sellers, and both views must name the same one.
		for _, it := range d.Items {
			if d.SellerID == "" || it.SellerID < d.SellerID {
				d.SellerID = it.SellerID
			}
		}
		rows, err = tx.Query(ctx, `
			SELECT status, at FROM order_status_timeline WHERE order_id = $1
			ORDER BY at, array_position($2::text[], status)`, orderID, domain.Statuses)
		if err != nil {
			return err
		}
		d.Timeline, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.StatusChange, error) {
			var c domain.StatusChange
			err := row.Scan(&c.Status, &c.At)
			c.At = c.At.UTC()
			return c, err
		})
		return err
	})
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Detail{}, err
	}
	if err != nil {
		return domain.Detail{}, fmt.Errorf("get order: %w", err)
	}
	return d, nil
}
