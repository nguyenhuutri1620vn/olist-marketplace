package olist_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/tools/olist-importer/internal/olist"
)

func TestParseCentavos(t *testing.T) {
	for in, want := range map[string]int64{
		"58.90": 5890, "7.6": 760, "1050": 105000, "0.00": 0, "1050.61": 105061, " 13.29 ": 1329,
	} {
		got, err := olist.ParseCentavos(in)
		if err != nil || got != want {
			t.Errorf("ParseCentavos(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"1.234", "-1", "abc", "", ".5", "1.", "1,50", "+1", "99999999999999999999"} {
		if got, err := olist.ParseCentavos(in); err == nil {
			t.Errorf("ParseCentavos(%q) = %d, want error", in, got)
		}
	}
}

const (
	p1 = "1e9e8ef04dbcff4541ed26657ea517e5"
	p2 = "3aa071139cb16b67ca9e5dea641aaa2f"
	p3 = "96bd76ec8810374ed1b65e291975717f"
	s1 = "3442f8959a84dea7ee197c632cb2df15"
	s2 = "d1b65fc7debc3361ea86b5f14c68d2e2"
	s3 = "ce3ad9de960102d0677a81f5d0bb7b2d"
)

// writeDataset writes a tiny dataset in the Olist CSV layout.
func writeDataset(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		olist.TranslationsFile: "\xef\xbb\xbfproduct_category_name,product_category_name_english\n" +
			"beleza_saude,health_beauty\nartes,art\n",
		olist.SellersFile: `"seller_id","seller_zip_code_prefix","seller_city","seller_state"` + "\n" +
			`"` + s1 + `","13023",campinas,SP` + "\n" +
			`"` + s2 + `","20000",rio de janeiro,RJ` + "\n" +
			`"` + s3 + `","30000",belo horizonte,MG` + "\n",
		olist.ProductsFile: `"product_id","product_category_name","product_name_lenght"` + "\n" +
			`"` + p1 + `",beleza_saude,40` + "\n" +
			`"` + p2 + `",,10` + "\n" +
			`"` + p3 + `",pc_gamer,10` + "\n" +
			`"ffffffffffffffffffffffffffffffff",artes,1` + "\n",
		olist.OrdersFile: `"order_id","customer_id","order_status","order_purchase_timestamp"` + "\n" +
			"aaaa,c1,delivered,2017-10-02 10:56:33\n" +
			"bbbb,c2,delivered,2018-01-01 08:00:00\n" +
			"cccc,c3,delivered,2018-01-01 08:00:00\n",
		// p1: newest purchase wins (bbbb/cccc tie on time → cccc by order_id DESC).
		// p2: one order, two items → order_item_id 2 wins.
		// p3: only in the oldest order.
		olist.OrderItemsFile: `"order_id","order_item_id","product_id","seller_id","shipping_limit_date","price","freight_value"` + "\n" +
			"aaaa,1," + p1 + "," + s1 + ",2017-10-06 11:07:15,10.00,1.00\n" +
			"bbbb,1," + p1 + "," + s1 + ",2018-01-05 08:00:00,20.00,2.00\n" +
			"cccc,1," + p1 + "," + s2 + ",2018-01-05 08:00:00,30.00,3.00\n" +
			"aaaa,1," + p2 + "," + s1 + ",2017-10-06 11:07:15,5.00,0.50\n" +
			"aaaa,2," + p2 + "," + s3 + ",2017-10-06 11:07:15,6.00,0.60\n" +
			"aaaa,3," + p3 + "," + s2 + ",2017-10-06 11:07:15,7.6,0\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadPicksMostRecentOrderItem(t *testing.T) {
	ds, err := olist.Load(writeDataset(t))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]olist.Product{}
	for _, p := range ds.Products {
		got[p.ID] = p
	}
	want := map[string]olist.Product{
		p1: {ID: p1, Category: "beleza_saude", SellerID: s2, Price: 3000, Freight: 300},
		p2: {ID: p2, Category: "", SellerID: s3, Price: 600, Freight: 60},
		p3: {ID: p3, Category: "pc_gamer", SellerID: s2, Price: 760, Freight: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("products = %v, want %v", got, want)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("product %s = %+v, want %+v", id, got[id], w)
		}
	}
	if len(ds.Skipped) != 1 || ds.Skipped[0] != "ffffffffffffffffffffffffffffffff" {
		t.Errorf("Skipped = %v, want the product without Order Items", ds.Skipped)
	}
}

func TestLoadIncludesUntranslatedCategories(t *testing.T) {
	ds, err := olist.Load(writeDataset(t))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range ds.Categories {
		got[c.NamePT] = c.NameEN
	}
	want := map[string]string{"beleza_saude": "health_beauty", "artes": "art", "pc_gamer": ""}
	if len(got) != len(want) {
		t.Fatalf("categories = %v, want %v", got, want)
	}
	for pt, en := range want {
		if en2, ok := got[pt]; !ok || en2 != en {
			t.Errorf("category %q = %q (present %v), want %q", pt, en2, ok, en)
		}
	}
}

func TestLoadReadsSellers(t *testing.T) {
	ds, err := olist.Load(writeDataset(t))
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by id: s1 < s3 < s2.
	if len(ds.Sellers) != 3 || ds.Sellers[2] != (olist.Seller{ID: s2, City: "rio de janeiro", State: "RJ"}) {
		t.Errorf("sellers = %+v", ds.Sellers)
	}
}

func TestLoadFailsOnBadData(t *testing.T) {
	for name, mutate := range map[string]func(dir string){
		"missing file": func(dir string) { os.Remove(filepath.Join(dir, olist.SellersFile)) },
		"bad price": func(dir string) {
			appendLine(t, dir, olist.OrderItemsFile, "aaaa,9,"+p1+","+s1+",x,1.234,0\n")
		},
		"item of unknown order": func(dir string) {
			appendLine(t, dir, olist.OrderItemsFile, "zzzz,1,"+p1+","+s1+",x,1.00,0\n")
		},
		"missing column": func(dir string) {
			os.WriteFile(filepath.Join(dir, olist.OrdersFile), []byte("order_id,customer_id\naaaa,c1\n"), 0o644)
		},
	} {
		dir := writeDataset(t)
		mutate(dir)
		if _, err := olist.Load(dir); err == nil {
			t.Errorf("%s: Load succeeded, want error", name)
		} else if !strings.Contains(err.Error(), "csv") {
			t.Errorf("%s: error %q does not name the file", name, err)
		}
	}
}

func appendLine(t *testing.T, dir, file, line string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, file), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
}

// TestLoadRealDataset checks the Kaggle files when OLIST_DATA_DIR points at them.
func TestLoadRealDataset(t *testing.T) {
	dir := os.Getenv("OLIST_DATA_DIR")
	if dir == "" {
		t.Skip("OLIST_DATA_DIR not set")
	}
	ds, err := olist.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Products) != 32951 || len(ds.Sellers) != 3095 || len(ds.Categories) != 73 || len(ds.Skipped) != 0 {
		t.Errorf("products %d, sellers %d, categories %d, skipped %d; want 32951, 3095, 73, 0",
			len(ds.Products), len(ds.Sellers), len(ds.Categories), len(ds.Skipped))
	}
	for _, c := range ds.Categories {
		if c.NamePT == "" || strings.HasPrefix(c.NamePT, "\xef\xbb\xbf") {
			t.Errorf("bad category name %q", c.NamePT)
		}
	}
}
