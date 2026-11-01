package eventsv1_test

import (
	"encoding/json"
	"testing"
	"time"

	eventsv1 "github.com/nguyenhuutri1620vn/olist-marketplace/contracts/events/v1"
)

func TestOrderCreatedIsInTheCatalog(t *testing.T) {
	ev := eventsv1.OrderCreated{Order: eventsv1.OrderSnapshot{OrderID: "o-1"}}
	if err := eventsv1.Check(ev); err != nil {
		t.Fatal(err)
	}
	if ev.Topic() != eventsv1.TopicOrderEvents || ev.AggregateID() != "o-1" || ev.EventVersion() != "1.0" {
		t.Errorf("topic %s aggregate %s version %s", ev.Topic(), ev.AggregateID(), ev.EventVersion())
	}
}

// AD-10: every Order fact carries the transition and the full snapshot.
func TestOrderCreatedJSON(t *testing.T) {
	at := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)
	b, err := json.Marshal(eventsv1.OrderCreated{
		FromStatus: "", ToStatus: "created",
		Order: eventsv1.OrderSnapshot{
			OrderID: "o", CustomerID: "c", CheckoutID: "k", PurchasedAt: at, Historical: false, Status: "created", VoucherAmount: 0,
			Items: []eventsv1.OrderItem{{ProductID: "p", SellerID: "s", SellerState: "SP", CategoryEN: "health_beauty", Price: 5890, Freight: 1329}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"from_status":"","to_status":"created","order":{"order_id":"o","customer_id":"c","checkout_id":"k",` +
		`"purchased_at":"2026-11-01T09:30:00Z","historical":false,"status":"created","voucher_amount":0,` +
		`"items":[{"product_id":"p","seller_id":"s","seller_state":"SP","category_en":"health_beauty","price":5890,"freight":1329}]}}`
	if string(b) != want {
		t.Errorf("OrderCreated JSON\n got %s\nwant %s", b, want)
	}
}
