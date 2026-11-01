// Package domain holds Orders and the Checkout rules (FR1, AD-5, AD-15).
package domain

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	// ErrInvalid marks input that breaks a domain rule.
	ErrInvalid = errors.New("invalid")
	// ErrKeyReused marks a Checkout key already used with other Products.
	ErrKeyReused = errors.New("checkout key already used with other products")
	// ErrUnavailable marks a dependency (catalog) that did not answer in time.
	ErrUnavailable = errors.New("unavailable")
	// ErrCheckoutExists means the Customer already saved a Checkout under the
	// key; read it instead.
	ErrCheckoutExists = errors.New("checkout key already taken")
)

// Order Status values: the dataset words (AD-5).
const (
	StatusCreated    = "created"
	StatusApproved   = "approved"
	StatusProcessing = "processing"
	StatusShipped    = "shipped"
	StatusDelivered  = "delivered"
	StatusCanceled   = "canceled"
)

// Statuses lists every Order Status.
var Statuses = []string{StatusCreated, StatusApproved, StatusProcessing, StatusShipped, StatusDelivered, StatusCanceled}

// Checkout limits: there are no quantities, so a Product appears once.
const (
	MaxProducts = 100
	maxKeyLen   = 200
)

var (
	datasetID = regexp.MustCompile(`^[0-9a-f]{32}$`)
	stateCode = regexp.MustCompile(`^[A-Z]{2}$`)
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Checkout is a validated Checkout request.
type Checkout struct {
	CustomerID string
	Key        string
	ProductIDs []string
}

// NewCheckout validates a request: a uuid Customer, a non-empty key and 1 to
// MaxProducts distinct dataset Product ids.
func NewCheckout(customerID, key string, productIDs []string) (Checkout, error) {
	cid, err := uuid.Parse(customerID)
	if err != nil {
		return Checkout{}, invalid("customer id %q is not a uuid", customerID)
	}
	if key == "" || len(key) > maxKeyLen || !utf8.ValidString(key) || strings.ContainsRune(key, 0) {
		return Checkout{}, invalid("checkout_idempotency_key must be 1 to %d bytes of UTF-8 text", maxKeyLen)
	}
	if len(productIDs) == 0 || len(productIDs) > MaxProducts {
		return Checkout{}, invalid("want 1 to %d product ids, got %d", MaxProducts, len(productIDs))
	}
	seen := make(map[string]bool, len(productIDs))
	for _, id := range productIDs {
		if !datasetID.MatchString(id) {
			return Checkout{}, invalid("product id %q is not 32 hex digits", id)
		}
		if seen[id] {
			return Checkout{}, invalid("product id %s is listed twice; there are no quantities", id)
		}
		seen[id] = true
	}
	return Checkout{CustomerID: cid.String(), Key: key, ProductIDs: slices.Clone(productIDs)}, nil
}

// Hash identifies the requested Product set: the same Products in another
// order are the same request.
func (c Checkout) Hash() string {
	ids := slices.Sorted(slices.Values(c.ProductIDs))
	sum := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	return hex.EncodeToString(sum[:])
}

// Item is a Product snapshot taken at Checkout (AD-15), in centavos.
type Item struct {
	ProductID   string
	SellerID    string
	SellerState string
	CategoryEN  string
	Price       int64
	Freight     int64
}

// Order is one Seller's part of a Checkout.
type Order struct {
	ID            string
	CustomerID    string
	CheckoutID    string
	SellerID      string
	Status        string
	PurchasedAt   time.Time
	Historical    bool
	VoucherAmount int64
	Items         []Item
}

// Total is Σ(price + freight) (AD-15).
func (o Order) Total() int64 {
	var t int64
	for _, it := range o.Items {
		t += it.Price + it.Freight
	}
	return t
}

// Plan groups the catalog snapshots into one created Order per Seller,
// sorted by seller id, items in request order. Every requested Product must
// be among the snapshots. A snapshot that breaks the catalog contract is an
// error of the system, not of the request. purchased_at is now in UTC at
// the microsecond precision Postgres stores.
func (c Checkout) Plan(checkoutID string, snapshots []Item, now time.Time, newID func() string) ([]Order, error) {
	now = now.UTC().Truncate(time.Microsecond)
	byID := make(map[string]Item, len(snapshots))
	for _, s := range snapshots {
		if !datasetID.MatchString(s.SellerID) || !stateCode.MatchString(s.SellerState) || s.CategoryEN == "" || s.Price < 0 || s.Freight < 0 {
			return nil, fmt.Errorf("catalog snapshot of %s breaks the contract: %+v", s.ProductID, s)
		}
		byID[s.ProductID] = s
	}
	var missing []string
	bySeller := map[string][]Item{}
	for _, id := range c.ProductIDs {
		it, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		bySeller[it.SellerID] = append(bySeller[it.SellerID], it)
	}
	if len(missing) > 0 {
		return nil, invalid("unknown product ids: %s", strings.Join(missing, ", "))
	}
	orders := make([]Order, 0, len(bySeller))
	for seller, items := range bySeller {
		orders = append(orders, Order{
			CustomerID:  c.CustomerID,
			CheckoutID:  checkoutID,
			SellerID:    seller,
			Status:      StatusCreated,
			PurchasedAt: now,
			Items:       items,
		})
	}
	slices.SortFunc(orders, func(a, b Order) int { return cmp.Compare(a.SellerID, b.SellerID) })
	for i := range orders {
		orders[i].ID = newID()
	}
	return orders, nil
}

// Placed is a saved Checkout: its id, request hash and Orders by seller id.
type Placed struct {
	CheckoutID string
	Hash       string
	Orders     []Order
}

// ErrNotFound marks an Order the caller may not see: unknown, or another
// Customer's (the same answer, so ids cannot be probed).
var ErrNotFound = errors.New("order not found")

// MaxPageSize bounds a page of Orders (as Story 1.8 bounds Products).
const MaxPageSize = 100

// Page selects one page of a list.
type Page struct {
	Number int32 // 1-based
	Size   int32
}

// NewPage validates the bounds: page ≥ 1, size 1..MaxPageSize.
func NewPage(number, size int32) (Page, error) {
	if number < 1 {
		return Page{}, invalid("page %d is below 1", number)
	}
	if size < 1 || size > MaxPageSize {
		return Page{}, invalid("page_size %d is outside 1..%d", size, MaxPageSize)
	}
	return Page{Number: number, Size: size}, nil
}

// Limit is the page size.
func (p Page) Limit() int64 { return int64(p.Size) }

// Offset is the number of rows before the page.
func (p Page) Offset() int64 { return int64(p.Number-1) * int64(p.Size) }

// ParseOrderID returns the canonical form of an Order id.
func ParseOrderID(id string) (string, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return "", invalid("order id %q is not a uuid", id)
	}
	return u.String(), nil
}

// Summary is one row of a Customer's Order list.
type Summary struct {
	ID          string
	SellerID    string
	Status      string
	PurchasedAt time.Time
	Total       int64
	ItemCount   int
}

// SummaryPage is one page of Orders and the count across all pages.
type SummaryPage struct {
	Orders []Summary
	Total  int64
}

// StatusChange is one timeline entry.
type StatusChange struct {
	Status string
	At     time.Time
}

// Detail is an Order with its timeline, oldest first.
type Detail struct {
	Order
	Timeline        []StatusChange
	CancelReason    string
	VoucherRestored bool
}
