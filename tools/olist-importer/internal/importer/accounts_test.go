package importer_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"

	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/tools/olist-importer/internal/importer"
	"github.com/nguyenhuutri1620vn/olist-marketplace/tools/olist-importer/internal/olist"
)

type fakeIdentity struct {
	req         *identityv1.ImportAccountsRequest
	hadDeadline bool
	err         error
}

func (f *fakeIdentity) ImportAccounts(ctx context.Context, r *identityv1.ImportAccountsRequest, _ ...grpc.CallOption) (*identityv1.ImportAccountsResponse, error) {
	f.req = r
	_, f.hadDeadline = ctx.Deadline()
	if f.err != nil {
		return nil, f.err
	}
	return &identityv1.ImportAccountsResponse{Upserted: int32(len(r.GetAccounts()))}, nil
}

func TestImportAccountsSendsRolesAndPassword(t *testing.T) {
	f := &fakeIdentity{}
	accts := []olist.DemoAccount{
		{ID: "u1", Email: "customer-u1@demo.olist.local", Role: olist.RoleCustomer},
		{ID: "s1", Email: "seller-s1@demo.olist.local", Role: olist.RoleSeller},
		{ID: "a1", Email: "admin@demo.olist.local", Role: olist.RoleAdmin},
	}
	n, err := importer.ImportAccounts(context.Background(), f, accts, "olist-demo-2026")
	if err != nil || n != 3 {
		t.Fatalf("ImportAccounts = %d, %v", n, err)
	}
	if f.req.GetPassword() != "olist-demo-2026" || !f.hadDeadline {
		t.Errorf("password %q, deadline %v", f.req.GetPassword(), f.hadDeadline)
	}
	want := []identityv1.Role{identityv1.Role_ROLE_CUSTOMER, identityv1.Role_ROLE_SELLER, identityv1.Role_ROLE_ADMIN}
	for i, a := range f.req.GetAccounts() {
		if a.GetRole() != want[i] || a.GetAccountId() != accts[i].ID || a.GetEmail() != accts[i].Email {
			t.Errorf("account %d = %v", i, a)
		}
	}
}

func TestImportAccountsReturnsTheRPCError(t *testing.T) {
	f := &fakeIdentity{err: errors.New("invalid")}
	if _, err := importer.ImportAccounts(context.Background(), f, []olist.DemoAccount{{ID: "u1", Role: olist.RoleCustomer}}, "pw"); err == nil {
		t.Error("ImportAccounts succeeded, want the RPC error")
	}
}

func TestImportAccountsRejectsUnknownRole(t *testing.T) {
	f := &fakeIdentity{}
	if _, err := importer.ImportAccounts(context.Background(), f, []olist.DemoAccount{{ID: "u1", Role: "root"}}, "pw"); err == nil || f.req != nil {
		t.Errorf("err = %v, request sent %v; want an error before any RPC", err, f.req != nil)
	}
}
