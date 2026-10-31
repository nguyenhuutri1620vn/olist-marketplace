package authtest_test

import (
	"context"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth/authtest"
)

func TestIssuedTokensVerify(t *testing.T) {
	iss := authtest.NewIssuer(t)
	c, err := iss.Verifier().Verify(context.Background(), iss.Token(t, "acc-1", auth.RoleAdmin))
	if err != nil || c != (auth.Claims{AccountID: "acc-1", Role: auth.RoleAdmin}) {
		t.Errorf("Verify = %+v, %v", c, err)
	}
}
