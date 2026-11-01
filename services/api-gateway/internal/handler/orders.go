package handler

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
)

// orderTimeout bounds an order-service read.
const orderTimeout = 5 * time.Second

// OrderReader is the part of OrderService the Order read routes call.
type OrderReader interface {
	ListMyOrders(ctx context.Context, in *orderv1.ListMyOrdersRequest, opts ...grpc.CallOption) (*orderv1.ListMyOrdersResponse, error)
	GetOrder(ctx context.Context, in *orderv1.GetOrderRequest, opts ...grpc.CallOption) (*orderv1.GetOrderResponse, error)
}

type orderSummary struct {
	OrderID       string `json:"order_id"`
	PurchasedAt   string `json:"purchased_at"`
	Status        string `json:"status"`
	TotalCentavos int64  `json:"total_centavos"`
	SellerID      string `json:"seller_id"`
	ItemCount     int32  `json:"item_count"`
}

type orderList struct {
	Orders   []orderSummary `json:"orders"`
	Total    int64          `json:"total"`
	Page     int32          `json:"page"`
	PageSize int32          `json:"page_size"`
}

type statusChange struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

type orderDetail struct {
	OrderID         string         `json:"order_id"`
	PurchasedAt     string         `json:"purchased_at"`
	Status          string         `json:"status"`
	TotalCentavos   int64          `json:"total_centavos"`
	SellerID        string         `json:"seller_id"`
	VoucherCentavos int64          `json:"voucher_amount_centavos"`
	Items           []checkoutItem `json:"items"`
	Timeline        []statusChange `json:"timeline"`
	CancelReason    string         `json:"cancel_reason"`
	VoucherRestored bool           `json:"voucher_restored"`
}

// rfc3339 formats a protobuf timestamp in UTC.
func rfc3339(ts *timestamppb.Timestamp) string {
	return ts.AsTime().UTC().Format(time.RFC3339Nano)
}

// ListOrders serves GET /api/orders?page=&page_size= for the calling
// Customer (defaults 1 and 20; order-service checks the bounds).
func ListOrders(orders OrderReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, ok := intParam(w, q, "page", defaultPage)
		if !ok {
			return
		}
		size, ok := intParam(w, q, "page_size", defaultPageSize)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), orderTimeout)
		defer cancel()
		resp, err := orders.ListMyOrders(ctx, &orderv1.ListMyOrdersRequest{Page: page, PageSize: size})
		if err != nil {
			WriteGRPCError(w, err)
			return
		}
		out := orderList{Orders: make([]orderSummary, len(resp.GetOrders())), Total: resp.GetTotal(), Page: page, PageSize: size}
		for i, o := range resp.GetOrders() {
			out.Orders[i] = orderSummary{
				OrderID: o.GetOrderId(), PurchasedAt: rfc3339(o.GetPurchasedAt()), Status: o.GetStatus(),
				TotalCentavos: o.GetTotalCentavos(), SellerID: o.GetSellerId(), ItemCount: o.GetItemCount(),
			}
		}
		writeJSON(w, http.StatusOK, out)
	})
}

// GetOrder serves GET /api/orders/{id} for the calling Customer; another
// Customer's Order is 404 like an unknown one.
func GetOrder(orders OrderReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), orderTimeout)
		defer cancel()
		resp, err := orders.GetOrder(ctx, &orderv1.GetOrderRequest{OrderId: r.PathValue("id")})
		if err != nil {
			WriteGRPCError(w, err)
			return
		}
		o := resp.GetOrder()
		if o == nil {
			WriteError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
			return
		}
		out := orderDetail{
			OrderID: o.GetOrderId(), PurchasedAt: rfc3339(o.GetPurchasedAt()), Status: o.GetStatus(),
			TotalCentavos: o.GetTotalCentavos(), SellerID: o.GetSellerId(), VoucherCentavos: o.GetVoucherAmountCentavos(),
			Items: make([]checkoutItem, len(o.GetItems())), Timeline: make([]statusChange, len(o.GetTimeline())),
			CancelReason: o.GetCancelReason(), VoucherRestored: o.GetVoucherRestored(),
		}
		for i, it := range o.GetItems() {
			out.Items[i] = checkoutItem{ProductID: it.GetProductId(), CategoryNameEN: it.GetCategoryNameEn(), PriceCentavos: it.GetPriceCentavos(), FreightCentavos: it.GetFreightCentavos()}
		}
		for i, c := range o.GetTimeline() {
			out.Timeline[i] = statusChange{Status: c.GetStatus(), At: rfc3339(c.GetAt())}
		}
		writeJSON(w, http.StatusOK, out)
	})
}
