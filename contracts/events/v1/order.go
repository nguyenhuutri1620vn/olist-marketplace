package eventsv1

import "time"

// OrderSnapshot is the full current state every Order fact carries, so
// read models never need an earlier event or another service (AD-10).
type OrderSnapshot struct {
	OrderID    string `json:"order_id"`
	CustomerID string `json:"customer_id"`
	// CheckoutID is empty for Historical Orders.
	CheckoutID  string    `json:"checkout_id"`
	PurchasedAt time.Time `json:"purchased_at"`
	Historical  bool      `json:"historical"`
	Status      string    `json:"status"`
	// VoucherAmount is 0 until approved, in centavos.
	VoucherAmount int64       `json:"voucher_amount"`
	Items         []OrderItem `json:"items"`
}

// OrderItem is a Product snapshot taken at Checkout (AD-15), in centavos.
type OrderItem struct {
	ProductID   string `json:"product_id"`
	SellerID    string `json:"seller_id"`
	SellerState string `json:"seller_state"`
	// CategoryEN is "Uncategorized" when the Product has no Category.
	CategoryEN string `json:"category_en"`
	Price      int64  `json:"price"`
	Freight    int64  `json:"freight"`
}

// OrderCreated is published when an Order enters created (FR1).
type OrderCreated struct {
	FromStatus string        `json:"from_status"`
	ToStatus   string        `json:"to_status"`
	Order      OrderSnapshot `json:"order"`
}

func (OrderCreated) EventType() string     { return "OrderCreated" }
func (OrderCreated) EventVersion() string  { return "1.0" }
func (OrderCreated) Topic() string         { return TopicOrderEvents }
func (e OrderCreated) AggregateID() string { return e.Order.OrderID }
