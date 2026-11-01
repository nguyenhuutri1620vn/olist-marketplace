// Package domain holds catalog's reference data: Categories, Seller profiles
// and Products imported from the Olist dataset, with their validation rules.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrInvalid marks input that breaks a domain rule.
var ErrInvalid = errors.New("invalid")

// Uncategorized is the Category of Products the dataset leaves without one. It
// is a real Category row whose Portuguese and English names are both this.
const Uncategorized = "Uncategorized"

// Olist ids are 32 lowercase hex digits; they are stored as uuid.
var (
	datasetID = regexp.MustCompile(`^[0-9a-f]{32}$`)
	stateCode = regexp.MustCompile(`^[A-Z]{2}$`)
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Category is keyed by its Portuguese dataset name.
type Category struct {
	NamePT string
	NameEN string
}

// NewCategory builds a Category; without a translation the English name is the
// Portuguese one, so no display name starts empty.
func NewCategory(namePT, nameEN string) (Category, error) {
	namePT, nameEN = strings.TrimSpace(namePT), strings.TrimSpace(nameEN)
	if namePT == "" {
		return Category{}, invalid("category name is empty")
	}
	if nameEN == "" {
		nameEN = namePT
	}
	return Category{NamePT: namePT, NameEN: nameEN}, nil
}

// Seller is a Seller profile.
type Seller struct {
	ID    string
	City  string
	State string
}

// NewSeller validates a Seller profile.
func NewSeller(id, city, state string) (Seller, error) {
	if !datasetID.MatchString(id) {
		return Seller{}, invalid("seller id %q is not 32 hex digits", id)
	}
	if !stateCode.MatchString(state) {
		return Seller{}, invalid("seller %s state %q is not a two-letter code", id, state)
	}
	return Seller{ID: id, City: city, State: state}, nil
}

// Product carries the price, freight and Seller of its most recent dataset
// Order Item, in centavos.
type Product struct {
	ID       string
	Category string // Portuguese Category name
	SellerID string
	Price    int64
	Freight  int64
}

// NewProduct validates a Product; an empty Category means Uncategorized.
func NewProduct(id, category, sellerID string, price, freight int64) (Product, error) {
	if !datasetID.MatchString(id) {
		return Product{}, invalid("product id %q is not 32 hex digits", id)
	}
	if !datasetID.MatchString(sellerID) {
		return Product{}, invalid("product %s seller id %q is not 32 hex digits", id, sellerID)
	}
	if price < 0 || freight < 0 {
		return Product{}, invalid("product %s has negative price or freight", id)
	}
	if category = strings.TrimSpace(category); category == "" {
		category = Uncategorized
	}
	return Product{ID: id, Category: category, SellerID: sellerID, Price: price, Freight: freight}, nil
}

// MaxPageSize bounds one page of any list (Story 1.8).
const MaxPageSize = 100

// ProductQuery selects one page of Products, optionally in one Category.
type ProductQuery struct {
	Category string // Portuguese Category name; empty means every Category
	Page     int32  // 1-based
	PageSize int32
}

// NewProductQuery validates the page bounds.
func NewProductQuery(category string, page, pageSize int32) (ProductQuery, error) {
	if page < 1 {
		return ProductQuery{}, invalid("page %d is below 1", page)
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		return ProductQuery{}, invalid("page_size %d is outside 1..%d", pageSize, MaxPageSize)
	}
	return ProductQuery{Category: strings.TrimSpace(category), Page: page, PageSize: pageSize}, nil
}

// Limit is the page size.
func (q ProductQuery) Limit() int64 { return int64(q.PageSize) }

// Offset is the number of Products before the page.
func (q ProductQuery) Offset() int64 { return int64(q.Page-1) * int64(q.PageSize) }

// ProductSummary is one Product as browsing clients see it.
type ProductSummary struct {
	ID          string
	DisplayName string
	Category    Category
	Seller      Seller
	Price       int64
}

// ProductPage is one page of Products and the count matching the filter.
type ProductPage struct {
	Products []ProductSummary
	Total    int64
}
