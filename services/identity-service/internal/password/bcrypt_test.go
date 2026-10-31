package password_test

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/password"
)

func TestBcryptHashesAndCompares(t *testing.T) {
	h := password.Bcrypt{Cost: bcrypt.MinCost}
	hash, err := h.Hash("s3cret-pw")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$2a$") || strings.Contains(hash, "s3cret-pw") {
		t.Errorf("hash %q is not a bcrypt hash", hash)
	}
	if h.Compare(hash, "s3cret-pw") != nil || h.Compare(hash, "wrong") == nil {
		t.Error("Compare does not tell right from wrong password")
	}
}

func TestDefaultCostIsBcryptDefault(t *testing.T) {
	hash, err := password.Bcrypt{}.Hash("s3cret-pw")
	if err != nil {
		t.Fatal(err)
	}
	if cost, _ := bcrypt.Cost([]byte(hash)); cost != bcrypt.DefaultCost {
		t.Errorf("cost = %d, want %d", cost, bcrypt.DefaultCost)
	}
}
