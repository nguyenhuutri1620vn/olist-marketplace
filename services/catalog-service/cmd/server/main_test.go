package main

import (
	"testing"

	"google.golang.org/grpc"

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

// Browsing needs no token (NFR7).
func TestBrowsingRPCsArePublic(t *testing.T) {
	p := catalogService().Policy
	for _, m := range []string{
		catalogv1.CatalogService_ListCategories_FullMethodName,
		catalogv1.CatalogService_ListProducts_FullMethodName,
		catalogv1.CatalogService_GetProducts_FullMethodName, // order-service calls it without a token
	} {
		if !p.IsPublic(m) {
			t.Errorf("%s needs a token; catalog reads are public", m)
		}
	}
}

func TestBothServicesAreRegistered(t *testing.T) {
	s := grpc.NewServer()
	catalogService().RegisterGRPC(s, nil)
	info := s.GetServiceInfo()
	for _, name := range []string{catalogv1.CatalogService_ServiceDesc.ServiceName, catalogv1.CatalogAdminService_ServiceDesc.ServiceName} {
		if _, ok := info[name]; !ok {
			t.Errorf("%s not registered", name)
		}
	}
}
