package olist

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/nguyenhuutri1620vn/olist-marketplace/contracts/accounts"
)

// Demo Account roles.
const (
	RoleCustomer = "customer"
	RoleSeller   = "seller"
	RoleAdmin    = "admin"
)

// DemoPerRole is how many dataset Customers and Sellers get a demo Account.
const DemoPerRole = 20

// DemoAccount is a seeded Account (FR15).
type DemoAccount struct {
	ID    string
	Email string
	Role  string
}

// DemoAccounts picks the 20 customer_unique_ids with the most Orders and the
// 20 seller_ids with the most Order Items (ties to the lower id, so every run
// picks the same), then the Admin: Customers, Sellers, Admin in that order.
func DemoAccounts(dir string) ([]DemoAccount, error) {
	unique := map[string]string{}
	err := readCSV(dir, CustomersFile, []string{"customer_id", "customer_unique_id"}, func(r []string) error {
		unique[r[0]] = r[1]
		return nil
	})
	if err != nil {
		return nil, err
	}
	orders := map[string]int{}
	err = readCSV(dir, OrdersFile, []string{"order_id", "customer_id"}, func(r []string) error {
		u, ok := unique[r[1]]
		if !ok {
			return fmt.Errorf("order %s customer %s is not in %s", r[0], r[1], CustomersFile)
		}
		orders[u]++
		return nil
	})
	if err != nil {
		return nil, err
	}
	items := map[string]int{}
	err = readCSV(dir, OrderItemsFile, []string{"seller_id"}, func(r []string) error {
		items[r[0]]++
		return nil
	})
	if err != nil {
		return nil, err
	}

	var out []DemoAccount
	for _, id := range TopIDs(orders, DemoPerRole) {
		out = append(out, demo(id, RoleCustomer))
	}
	for _, id := range TopIDs(items, DemoPerRole) {
		out = append(out, demo(id, RoleSeller))
	}
	out = append(out, DemoAccount{ID: accounts.AdminAccountID, Email: accounts.AdminEmail, Role: RoleAdmin})
	// Emails use only 8 id characters; a clash would fail identity's unique
	// index with a less helpful error.
	seen := map[string]string{}
	for _, a := range out {
		if other, ok := seen[a.Email]; ok {
			return nil, fmt.Errorf("demo accounts %s and %s share email %s", other, a.ID, a.Email)
		}
		seen[a.Email] = a.ID
	}
	return out, nil
}

// demo names a dataset Account `<role>-<first 8 chars of id>@demo.olist.local`.
func demo(id, role string) DemoAccount {
	return DemoAccount{ID: id, Email: role + "-" + id[:min(8, len(id))] + "@" + accounts.DemoDomain, Role: role}
}

// TopIDs returns up to n ids ordered by count DESC, id ASC.
func TopIDs(counts map[string]int, n int) []string {
	ids := make([]string, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int {
		if c := cmp.Compare(counts[b], counts[a]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	return ids[:min(n, len(ids))]
}
