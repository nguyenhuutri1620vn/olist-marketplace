// Package olist reads the Olist Kaggle CSVs into the reference data catalog
// imports: Categories, Sellers and Products priced from their most recent
// Order Item (FR13, FR17).
package olist

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Dataset file names under the data directory.
const (
	TranslationsFile = "product_category_name_translation.csv"
	SellersFile      = "olist_sellers_dataset.csv"
	ProductsFile     = "olist_products_dataset.csv"
	OrdersFile       = "olist_orders_dataset.csv"
	OrderItemsFile   = "olist_order_items_dataset.csv"
	CustomersFile    = "olist_customers_dataset.csv"
)

// Category is a dataset Category; NameEN is empty when it has no translation.
type Category struct {
	NamePT string
	NameEN string
}

// Seller is a Seller profile.
type Seller struct {
	ID    string
	City  string
	State string
}

// Product carries the Seller, price and freight (centavos) of its most recent
// Order Item. Category is empty when the dataset gives none.
type Product struct {
	ID       string
	Category string
	SellerID string
	Price    int64
	Freight  int64
}

// Dataset is the reference data to import, each slice sorted by key.
type Dataset struct {
	Categories []Category
	Sellers    []Seller
	Products   []Product
	// Skipped lists Products with no Order Item, so no price.
	Skipped []string
}

var centavos = regexp.MustCompile(`^([0-9]+)(?:\.([0-9]{1,2}))?$`)

// ParseCentavos converts a dataset decimal such as "58.90" to centavos
// without going through floating point.
func ParseCentavos(s string) (int64, error) {
	m := centavos.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("%q is not a non-negative amount with at most 2 decimals", s)
	}
	units, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || units > math.MaxInt64/100-1 {
		return 0, fmt.Errorf("%q is out of range", s)
	}
	frac := int64(0)
	if m[2] != "" {
		frac, _ = strconv.ParseInt((m[2] + "0")[:2], 10, 64)
	}
	return units*100 + frac, nil
}

// Load reads the dataset CSVs in dir.
func Load(dir string) (Dataset, error) {
	var ds Dataset
	translations := map[string]string{}
	err := readCSV(dir, TranslationsFile, []string{"product_category_name", "product_category_name_english"}, func(r []string) error {
		translations[r[0]] = r[1]
		return nil
	})
	if err != nil {
		return ds, err
	}
	err = readCSV(dir, SellersFile, []string{"seller_id", "seller_city", "seller_state"}, func(r []string) error {
		ds.Sellers = append(ds.Sellers, Seller{ID: r[0], City: r[1], State: r[2]})
		return nil
	})
	if err != nil {
		return ds, err
	}
	purchased := map[string]time.Time{}
	err = readCSV(dir, OrdersFile, []string{"order_id", "order_purchase_timestamp"}, func(r []string) error {
		t, err := time.Parse(time.DateTime, r[1])
		if err != nil {
			return fmt.Errorf("order %s purchase timestamp: %w", r[0], err)
		}
		purchased[r[0]] = t
		return nil
	})
	if err != nil {
		return ds, err
	}
	latest, err := latestItems(dir, purchased)
	if err != nil {
		return ds, err
	}

	categories := map[string]bool{}
	for pt := range translations {
		categories[pt] = true
	}
	err = readCSV(dir, ProductsFile, []string{"product_id", "product_category_name"}, func(r []string) error {
		id, category := r[0], r[1]
		if category != "" {
			categories[category] = true
		}
		it, ok := latest[id]
		if !ok {
			ds.Skipped = append(ds.Skipped, id)
			return nil
		}
		ds.Products = append(ds.Products, Product{ID: id, Category: category, SellerID: it.sellerID, Price: it.price, Freight: it.freight})
		return nil
	})
	if err != nil {
		return ds, err
	}
	for pt := range categories {
		ds.Categories = append(ds.Categories, Category{NamePT: pt, NameEN: translations[pt]})
	}
	slices.SortFunc(ds.Categories, func(a, b Category) int { return strings.Compare(a.NamePT, b.NamePT) })
	slices.SortFunc(ds.Sellers, func(a, b Seller) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(ds.Products, func(a, b Product) int { return strings.Compare(a.ID, b.ID) })
	slices.Sort(ds.Skipped)
	return ds, nil
}

type item struct {
	purchased      time.Time
	orderID        string
	itemID         int
	sellerID       string
	price, freight int64
}

// newer orders items by purchase timestamp, then order_id, then
// order_item_id, matching ORDER BY ... DESC LIMIT 1, so re-imports pick the
// same row.
func (a item) newer(b item) bool {
	if !a.purchased.Equal(b.purchased) {
		return a.purchased.After(b.purchased)
	}
	if a.orderID != b.orderID {
		return a.orderID > b.orderID
	}
	return a.itemID > b.itemID
}

// latestItems returns each Product's most recent Order Item.
func latestItems(dir string, purchased map[string]time.Time) (map[string]item, error) {
	latest := map[string]item{}
	cols := []string{"order_id", "order_item_id", "product_id", "seller_id", "price", "freight_value"}
	err := readCSV(dir, OrderItemsFile, cols, func(r []string) error {
		t, ok := purchased[r[0]]
		if !ok {
			return fmt.Errorf("order %s is not in %s", r[0], OrdersFile)
		}
		itemID, err := strconv.Atoi(r[1])
		if err != nil {
			return fmt.Errorf("order_item_id %q: %w", r[1], err)
		}
		price, err := ParseCentavos(r[4])
		if err != nil {
			return fmt.Errorf("price: %w", err)
		}
		freight, err := ParseCentavos(r[5])
		if err != nil {
			return fmt.Errorf("freight_value: %w", err)
		}
		it := item{purchased: t, orderID: r[0], itemID: itemID, sellerID: r[3], price: price, freight: freight}
		if cur, ok := latest[r[2]]; !ok || it.newer(cur) {
			latest[r[2]] = it
		}
		return nil
	})
	return latest, err
}

var bom = []byte("\xef\xbb\xbf")

// readCSV calls row with the named columns of every record in dir/file.
func readCSV(dir, file string, columns []string, row func([]string) error) error {
	b, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return err
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, bom)))
	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("%s header: %w", file, err)
	}
	idx := make([]int, len(columns))
	for i, c := range columns {
		if idx[i] = slices.Index(header, c); idx[i] < 0 {
			return fmt.Errorf("%s has no column %q", file, c)
		}
	}
	vals := make([]string, len(columns))
	for line := 2; ; line++ {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		for i, j := range idx {
			vals[i] = rec[j]
		}
		if err := row(vals); err != nil {
			return fmt.Errorf("%s line %d: %w", file, line, err)
		}
	}
}
