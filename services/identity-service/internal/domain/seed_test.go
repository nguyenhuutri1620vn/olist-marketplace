package domain_test

import (
	"errors"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
)

func TestNewSeedNormalizesIDAndEmail(t *testing.T) {
	s, err := domain.NewSeed("8D50F5EADF50201CCDCEDFB9E2AC8455", " Customer-8d50f5ea@Demo.Olist.Local ", domain.RoleCustomer)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Seed{ID: "8d50f5ea-df50-201c-cdce-dfb9e2ac8455", Email: "customer-8d50f5ea@demo.olist.local", Role: domain.RoleCustomer}
	if s != want {
		t.Errorf("NewSeed = %+v, want %+v", s, want)
	}
	if s, err := domain.NewSeed("0192f000-0000-7000-8000-00000000ad01", "admin@demo.olist.local", domain.RoleAdmin); err != nil || s.ID != "0192f000-0000-7000-8000-00000000ad01" {
		t.Errorf("canonical id: %+v, %v", s, err)
	}
}

func TestNewSeedRejectsBadInput(t *testing.T) {
	for name, tc := range map[string]struct {
		id, email string
		role      domain.Role
	}{
		"bad id":          {"nothex", "a@demo.olist.local", domain.RoleSeller},
		"outside domain":  {"8d50f5eadf50201ccdcedfb9e2ac8455", "a@example.com", domain.RoleSeller},
		"subdomain":       {"8d50f5eadf50201ccdcedfb9e2ac8455", "a@x.demo.olist.local", domain.RoleSeller},
		"malformed email": {"8d50f5eadf50201ccdcedfb9e2ac8455", "demo.olist.local", domain.RoleSeller},
		"no role":         {"8d50f5eadf50201ccdcedfb9e2ac8455", "a@demo.olist.local", ""},
		"unknown role":    {"8d50f5eadf50201ccdcedfb9e2ac8455", "a@demo.olist.local", "root"},
	} {
		if _, err := domain.NewSeed(tc.id, tc.email, tc.role); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := domain.ValidatePassword("olist-demo-2026"); err != nil {
		t.Errorf("demo default: %v", err)
	}
	for _, pw := range []string{"short", string(make([]byte, 73))} {
		if err := domain.ValidatePassword(pw); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("password of %d bytes: err = %v, want ErrInvalid", len(pw), err)
		}
	}
}
