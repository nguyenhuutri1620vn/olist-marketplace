package main

import (
	"testing"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
)

// The importer calls these without a token, so the fail-closed policy must
// list them as public.
func TestImportRPCsArePublic(t *testing.T) {
	p := catalogService().Policy
	for _, m := range []string{
		catalogv1.CatalogAdminService_ImportCategories_FullMethodName,
		catalogv1.CatalogAdminService_ImportSellers_FullMethodName,
		catalogv1.CatalogAdminService_ImportProducts_FullMethodName,
	} {
		if !p.IsPublic(m) {
			t.Errorf("%s needs a token; the importer has none", m)
		}
	}
}
