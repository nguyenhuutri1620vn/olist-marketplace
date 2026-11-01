package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/api-gateway/internal/handler"
)

type fakeOrderReader struct {
	list    *orderv1.ListMyOrdersRequest
	get     *orderv1.GetOrderRequest
	listRes *orderv1.ListMyOrdersResponse
	getRes  *orderv1.GetOrderResponse
	err     error
}

func (f *fakeOrderReader) ListMyOrders(_ context.Context, r *orderv1.ListMyOrdersRequest, _ ...grpc.CallOption) (*orderv1.ListMyOrdersResponse, error) {
	f.list = r
	if f.listRes == nil {
		f.listRes = &orderv1.ListMyOrdersResponse{}
	}
	return f.listRes, f.err
}

func (f *fakeOrderReader) GetOrder(_ context.Context, r *orderv1.GetOrderRequest, _ ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	f.get = r
	return f.getRes, f.err
}

func serveOrders(f *fakeOrderReader, path string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle("GET /api/orders", handler.ListOrders(f))
	mux.Handle("GET /api/orders/{id}", handler.GetOrder(f))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

var at = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

func TestListOrdersRoute(t *testing.T) {
	f := &fakeOrderReader{listRes: &orderv1.ListMyOrdersResponse{Total: 3, Orders: []*orderv1.OrderSummary{{
		OrderId: "o-1", PurchasedAt: timestamppb.New(at), Status: "created", TotalCentavos: 7219, SellerId: "s", ItemCount: 2,
	}}}}
	rec := serveOrders(f, "/api/orders?page=2&page_size=5")
	if want := (&orderv1.ListMyOrdersRequest{Page: 2, PageSize: 5}); !proto.Equal(f.list, want) {
		t.Errorf("request = %v", f.list)
	}
	want := `{"orders":[{"order_id":"o-1","purchased_at":"2026-11-01T10:00:00Z","status":"created","total_centavos":7219,"seller_id":"s","item_count":2}],"total":3,"page":2,"page_size":5}`
	if rec.Code != 200 || rec.Body.String() != want+"\n" {
		t.Errorf("→ %d %s\nwant %s", rec.Code, rec.Body, want)
	}
	f = &fakeOrderReader{}
	if rec := serveOrders(f, "/api/orders"); rec.Code != 200 || !proto.Equal(f.list, &orderv1.ListMyOrdersRequest{Page: 1, PageSize: 20}) || !strings.Contains(rec.Body.String(), `"orders":[]`) {
		t.Errorf("defaults: %v → %d %s", f.list, rec.Code, rec.Body)
	}
	f = &fakeOrderReader{}
	if rec := serveOrders(f, "/api/orders?page=x"); rec.Code != 400 || f.list != nil {
		t.Errorf("page=x → %d, called %v", rec.Code, f.list != nil)
	}
}

func TestGetOrderRoute(t *testing.T) {
	f := &fakeOrderReader{getRes: &orderv1.GetOrderResponse{Order: &orderv1.OrderDetail{
		OrderId: "o-1", PurchasedAt: timestamppb.New(at), Status: "canceled", TotalCentavos: 7219, SellerId: "s", VoucherAmountCentavos: 500,
		Items:        []*orderv1.OrderItem{{ProductId: "p", CategoryNameEn: "health_beauty", PriceCentavos: 5890, FreightCentavos: 1329}},
		Timeline:     []*orderv1.StatusChange{{Status: "created", At: timestamppb.New(at)}, {Status: "canceled", At: timestamppb.New(at.Add(time.Minute))}},
		CancelReason: "CARD_DECLINED", VoucherRestored: true,
	}}}
	rec := serveOrders(f, "/api/orders/o-1")
	if f.get.GetOrderId() != "o-1" {
		t.Errorf("asked for %v", f.get)
	}
	want := `{"order_id":"o-1","purchased_at":"2026-11-01T10:00:00Z","status":"canceled","total_centavos":7219,"seller_id":"s",` +
		`"voucher_amount_centavos":500,"items":[{"product_id":"p","category_name_en":"health_beauty","price_centavos":5890,"freight_centavos":1329}],` +
		`"timeline":[{"status":"created","at":"2026-11-01T10:00:00Z"},{"status":"canceled","at":"2026-11-01T10:01:00Z"}],` +
		`"cancel_reason":"CARD_DECLINED","voucher_restored":true}`
	if rec.Code != 200 || rec.Body.String() != want+"\n" {
		t.Errorf("→ %d %s\nwant %s", rec.Code, rec.Body, want)
	}
	for err, code := range map[error]int{
		status.Error(codes.NotFound, "order not found"):       404,
		status.Error(codes.InvalidArgument, "not a uuid"):     400,
		status.Error(codes.Unavailable, "order-service down"): 503,
	} {
		if rec := serveOrders(&fakeOrderReader{err: err}, "/api/orders/x"); rec.Code != code {
			t.Errorf("%v → %d, want %d", err, rec.Code, code)
		}
	}
}

// A detail order-service answered without an Order is its bug, not data.
func TestGetOrderWithoutAnOrderIsAnError(t *testing.T) {
	rec := serveOrders(&fakeOrderReader{getRes: &orderv1.GetOrderResponse{}}, "/api/orders/x")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"INTERNAL"`) {
		t.Errorf("empty detail → %d %s", rec.Code, rec.Body)
	}
}
