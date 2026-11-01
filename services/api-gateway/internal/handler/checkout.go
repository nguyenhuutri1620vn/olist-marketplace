package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"google.golang.org/grpc"

	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
)

// maxCheckoutBody fits 100 product ids and a key.
const maxCheckoutBody = 16 << 10

// checkoutTimeout bounds the order-service call, which includes its own
// catalog call (CATALOG_TIMEOUT).
const checkoutTimeout = 10 * time.Second

// Checkouter is the part of OrderService POST /api/checkout calls.
type Checkouter interface {
	Checkout(ctx context.Context, in *orderv1.CheckoutRequest, opts ...grpc.CallOption) (*orderv1.CheckoutResponse, error)
}

type checkoutBody struct {
	ProductIDs []string `json:"product_ids"`
	Key        string   `json:"checkout_idempotency_key"`
}

type checkoutItem struct {
	ProductID       string `json:"product_id"`
	CategoryNameEN  string `json:"category_name_en"`
	PriceCentavos   int64  `json:"price_centavos"`
	FreightCentavos int64  `json:"freight_centavos"`
}

type checkoutOrder struct {
	OrderID       string         `json:"order_id"`
	SellerID      string         `json:"seller_id"`
	Status        string         `json:"status"`
	TotalCentavos int64          `json:"total_centavos"`
	Items         []checkoutItem `json:"items"`
}

type checkoutResponse struct {
	CheckoutID string          `json:"checkout_id"`
	Orders     []checkoutOrder `json:"orders"`
}

// Checkout serves POST /api/checkout {product_ids, checkout_idempotency_key}
// for a Customer (the route is gated; the JWT is forwarded and order-service
// takes the Customer from it).
func Checkout(orders Checkouter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Unknown fields and trailing data are errors: a typo must not become
		// an empty Product list.
		var body checkoutBody
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCheckoutBody))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil || dec.More() {
			WriteError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "body must be JSON {product_ids, checkout_idempotency_key}")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), checkoutTimeout)
		defer cancel()
		resp, err := orders.Checkout(ctx, &orderv1.CheckoutRequest{ProductIds: body.ProductIDs, CheckoutIdempotencyKey: body.Key})
		if err != nil {
			WriteGRPCError(w, err)
			return
		}
		out := checkoutResponse{CheckoutID: resp.GetCheckoutId(), Orders: make([]checkoutOrder, len(resp.GetOrders()))}
		for i, o := range resp.GetOrders() {
			items := make([]checkoutItem, len(o.GetItems()))
			for j, it := range o.GetItems() {
				items[j] = checkoutItem{ProductID: it.GetProductId(), CategoryNameEN: it.GetCategoryNameEn(), PriceCentavos: it.GetPriceCentavos(), FreightCentavos: it.GetFreightCentavos()}
			}
			out.Orders[i] = checkoutOrder{OrderID: o.GetOrderId(), SellerID: o.GetSellerId(), Status: o.GetStatus(), TotalCentavos: o.GetTotalCentavos(), Items: items}
		}
		writeJSON(w, http.StatusOK, out)
	})
}
