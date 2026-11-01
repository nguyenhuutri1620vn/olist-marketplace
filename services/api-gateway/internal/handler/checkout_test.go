package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/api-gateway/internal/handler"
)

type fakeOrders struct {
	req  *orderv1.CheckoutRequest
	resp *orderv1.CheckoutResponse
	err  error
}

func (f *fakeOrders) Checkout(_ context.Context, r *orderv1.CheckoutRequest, _ ...grpc.CallOption) (*orderv1.CheckoutResponse, error) {
	f.req = r
	return f.resp, f.err
}

func postCheckout(f *fakeOrders, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle("POST /api/checkout", handler.Checkout(f))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/checkout", strings.NewReader(body)))
	return rec
}

func TestCheckoutPassesTheRequestAndReturnsTheOrders(t *testing.T) {
	f := &fakeOrders{resp: &orderv1.CheckoutResponse{CheckoutId: "k-1", Orders: []*orderv1.Order{{
		OrderId: "o-1", SellerId: "s", Status: "created", TotalCentavos: 7219,
		Items: []*orderv1.OrderItem{{ProductId: "p", CategoryNameEn: "health_beauty", PriceCentavos: 5890, FreightCentavos: 1329}},
	}}}}
	rec := postCheckout(f, `{"product_ids":["p","q"],"checkout_idempotency_key":"key-1"}`)
	if want := (&orderv1.CheckoutRequest{ProductIds: []string{"p", "q"}, CheckoutIdempotencyKey: "key-1"}); !proto.Equal(f.req, want) {
		t.Errorf("request = %v", f.req)
	}
	want := `{"checkout_id":"k-1","orders":[{"order_id":"o-1","seller_id":"s","status":"created","total_centavos":7219,` +
		`"items":[{"product_id":"p","category_name_en":"health_beauty","price_centavos":5890,"freight_centavos":1329}]}]}`
	if rec.Code != http.StatusOK || rec.Body.String() != want+"\n" {
		t.Errorf("→ %d %s\nwant 200 %s", rec.Code, rec.Body, want)
	}
}

func TestCheckoutErrors(t *testing.T) {
	for _, tc := range []struct {
		body string
		err  error
		want int
	}{
		{`{nope`, nil, http.StatusBadRequest},
		{`{"product_id":["p"],"checkout_idempotency_key":"k"}`, nil, http.StatusBadRequest},
		{`{"product_ids":["p"],"checkout_idempotency_key":"k"}garbage`, nil, http.StatusBadRequest},
		{`{"product_ids":["p"],"checkout_idempotency_key":"k"}`, status.Error(codes.AlreadyExists, "checkout key already used with other products"), http.StatusConflict},
		{`{"product_ids":["p"],"checkout_idempotency_key":"k"}`, status.Error(codes.InvalidArgument, "invalid: unknown product ids: p"), http.StatusBadRequest},
		{`{"product_ids":["p"],"checkout_idempotency_key":"k"}`, status.Error(codes.Unavailable, "catalog unavailable"), http.StatusServiceUnavailable},
	} {
		f := &fakeOrders{err: tc.err}
		rec := postCheckout(f, tc.body)
		if rec.Code != tc.want || !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("%s / %v → %d %s, want %d", tc.body, tc.err, rec.Code, rec.Body, tc.want)
		}
		if tc.err == nil && f.req != nil {
			t.Error("bad JSON reached order-service")
		}
	}
}
