package olist_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/tools/olist-importer/internal/olist"
)

func TestTopIDsBreaksTiesByLowerID(t *testing.T) {
	counts := map[string]int{"c": 5, "b": 3, "a": 3, "d": 3, "e": 1}
	if got := olist.TopIDs(counts, 3); !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Errorf("TopIDs = %v, want [c a b]", got)
	}
	if got := olist.TopIDs(counts, 10); len(got) != 5 {
		t.Errorf("TopIDs with n > len = %v, want all 5", got)
	}
}

func TestDemoAccounts(t *testing.T) {
	dir := writeDataset(t)
	// Two customer_ids share one customer_unique_id, so it has 2 Orders.
	customers := `"customer_id","customer_unique_id","customer_zip_code_prefix","customer_city","customer_state"` + "\n" +
		"c1,u1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa,1,x,SP\n" +
		"c2,u2bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb,1,x,SP\n" +
		"c3,u1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa,1,x,SP\n"
	if err := os.WriteFile(filepath.Join(dir, olist.CustomersFile), []byte(customers), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := olist.DemoAccounts(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []olist.DemoAccount{
		// u1 has orders aaaa and cccc; u2 has bbbb.
		{ID: "u1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Email: "customer-u1aaaaaa@demo.olist.local", Role: olist.RoleCustomer},
		{ID: "u2bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Email: "customer-u2bbbbbb@demo.olist.local", Role: olist.RoleCustomer},
		// Item counts: s1 3, s2 2, s3 1.
		{ID: s1, Email: "seller-" + s1[:8] + "@demo.olist.local", Role: olist.RoleSeller},
		{ID: s2, Email: "seller-" + s2[:8] + "@demo.olist.local", Role: olist.RoleSeller},
		{ID: s3, Email: "seller-" + s3[:8] + "@demo.olist.local", Role: olist.RoleSeller},
		{ID: "0192f000-0000-7000-8000-00000000ad01", Email: "admin@demo.olist.local", Role: olist.RoleAdmin},
	}
	if !slices.Equal(got, want) {
		t.Errorf("DemoAccounts =\n%v\nwant\n%v", got, want)
	}
}

func TestDemoAccountsRealDataset(t *testing.T) {
	dir := os.Getenv("OLIST_DATA_DIR")
	if dir == "" {
		t.Skip("OLIST_DATA_DIR not set")
	}
	got, err := olist.DemoAccounts(dir)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]int{}
	emails := map[string]bool{}
	for _, a := range got {
		roles[a.Role]++
		emails[a.Email] = true
	}
	if roles[olist.RoleCustomer] != 20 || roles[olist.RoleSeller] != 20 || roles[olist.RoleAdmin] != 1 || len(emails) != 41 {
		t.Errorf("roles %v, distinct emails %d; want 20/20/1 and 41", roles, len(emails))
	}
	// Top picks, verified independently against the CSVs.
	if got[0].ID != "8d50f5eadf50201ccdcedfb9e2ac8455" || got[20].ID != "6560211a19b47992c3666cc44a7e94c0" {
		t.Errorf("first customer %s, first seller %s", got[0].ID, got[20].ID)
	}
}

func TestDemoAccountsRejectsEmailCollisions(t *testing.T) {
	dir := writeDataset(t)
	// Two customers whose ids share the first 8 characters get the same email.
	customers := `"customer_id","customer_unique_id"` + "\n" +
		"c1,abcdefgh11111111111111111111111\n" +
		"c2,abcdefgh22222222222222222222222\n" +
		"c3,abcdefgh22222222222222222222222\n"
	if err := os.WriteFile(filepath.Join(dir, olist.CustomersFile), []byte(customers), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := olist.DemoAccounts(dir); err == nil || !strings.Contains(err.Error(), "customer-abcdefgh@demo.olist.local") {
		t.Errorf("err = %v, want it to name the colliding email", err)
	}
}
