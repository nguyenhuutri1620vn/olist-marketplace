package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
)

// order-service publishes OrderCreated through the outbox.
func TestOrderServiceRunsTheOutboxRelay(t *testing.T) {
	s := orderService()
	if !s.Outbox || !s.UsesKafka || s.Migrations == nil {
		t.Errorf("order-service: Outbox %v, UsesKafka %v, Migrations set %v", s.Outbox, s.UsesKafka, s.Migrations != nil)
	}
}

func TestOnlyCustomersCheckOutAndReadTheirOrders(t *testing.T) {
	p := orderService().Policy
	for _, m := range []string{
		orderv1.OrderService_Checkout_FullMethodName,
		orderv1.OrderService_ListMyOrders_FullMethodName,
		orderv1.OrderService_GetOrder_FullMethodName,
	} {
		if p.IsPublic(m) || !slices.Equal(p.Roles[m], []auth.Role{auth.RoleCustomer}) {
			t.Errorf("%s policy: public %v, roles %v", m, p.IsPublic(m), p.Roles[m])
		}
	}
}

func TestInitBuildsTheServiceAndRegistersOrderService(t *testing.T) {
	s := orderService()
	if err := s.Init(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	s.RegisterGRPC(g, nil)
	if _, ok := g.GetServiceInfo()[orderv1.OrderService_ServiceDesc.ServiceName]; !ok {
		t.Error("OrderService not registered")
	}
}

func TestInitRejectsABadCatalogTimeout(t *testing.T) {
	t.Setenv("CATALOG_TIMEOUT", "never")
	if err := orderService().Init(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "CATALOG_TIMEOUT") {
		t.Errorf("Init = %v, want a CATALOG_TIMEOUT error", err)
	}
}
