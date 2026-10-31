package importer

import (
	"context"
	"fmt"

	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/tools/olist-importer/internal/olist"
)

var roles = map[string]identityv1.Role{
	olist.RoleCustomer: identityv1.Role_ROLE_CUSTOMER,
	olist.RoleSeller:   identityv1.Role_ROLE_SELLER,
	olist.RoleAdmin:    identityv1.Role_ROLE_ADMIN,
}

// ImportAccounts seeds the demo Accounts with one shared password in a single
// batch (41 rows) through identity's admin gRPC (FR15).
func ImportAccounts(ctx context.Context, c identityv1.IdentityAdminServiceClient, accts []olist.DemoAccount, password string) (int, error) {
	req := &identityv1.ImportAccountsRequest{Password: password}
	for _, a := range accts {
		role, ok := roles[a.Role]
		if !ok {
			return 0, fmt.Errorf("demo account %s has unknown role %q", a.ID, a.Role)
		}
		req.Accounts = append(req.Accounts, &identityv1.ImportAccount{AccountId: a.ID, Email: a.Email, Role: role})
	}
	rctx, cancel := context.WithTimeout(ctx, BatchTimeout)
	defer cancel()
	resp, err := c.ImportAccounts(rctx, req)
	if err != nil {
		return 0, fmt.Errorf("import accounts: %w", err)
	}
	return int(resp.GetUpserted()), nil
}
