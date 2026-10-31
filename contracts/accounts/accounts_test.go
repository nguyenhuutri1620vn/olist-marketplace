package accounts_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/contracts/accounts"
)

func TestAdminConstants(t *testing.T) {
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(accounts.AdminAccountID) {
		t.Errorf("AdminAccountID %q is not a canonical uuid", accounts.AdminAccountID)
	}
	if accounts.AdminEmail != "admin@"+accounts.DemoDomain || !strings.HasSuffix(accounts.AdminEmail, "@demo.olist.local") {
		t.Errorf("AdminEmail = %q", accounts.AdminEmail)
	}
}
