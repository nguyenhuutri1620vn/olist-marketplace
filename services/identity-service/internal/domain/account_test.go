package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
)

func TestNormalizeEmail(t *testing.T) {
	if got := domain.NormalizeEmail("  Ana@Example.COM "); got != "ana@example.com" {
		t.Errorf("NormalizeEmail = %q", got)
	}
}

func TestNewRegistrationAcceptsCustomer(t *testing.T) {
	for _, role := range []domain.Role{"", domain.RoleCustomer} {
		r, err := domain.NewRegistration(" Ana@Example.com ", "12345678", role)
		if err != nil {
			t.Fatalf("role %q: %v", role, err)
		}
		if r.Email != "ana@example.com" || r.Role != domain.RoleCustomer || r.Password != "12345678" {
			t.Errorf("role %q: registration = %+v", role, r)
		}
	}
}

func TestNewRegistrationRejectsBadInput(t *testing.T) {
	ok := "12345678"
	for name, tc := range map[string]struct {
		email, password string
		role            domain.Role
	}{
		"empty email":        {"", ok, ""},
		"malformed email":    {"not-an-email", ok, ""},
		"display name":       {`"Ana" <ana@example.com>`, ok, ""},
		"no domain dot":      {"ana@localhost", ok, ""},
		"reserved domain":    {"x@demo.olist.local", ok, ""},
		"reserved any case":  {"X@Demo.Olist.Local", ok, ""},
		"reserved subdomain": {"x@sub.demo.olist.local", ok, ""},
		"email too long":     {strings.Repeat("a", 64) + "@" + strings.Repeat("b", 190) + ".com", ok, ""},
		"short password":     {"ana@example.com", "1234567", ""},
		"short multibyte":    {"ana@example.com", "ééééééé", ""},
		"long password":      {"ana@example.com", strings.Repeat("a", 73), ""},
		"seller":             {"ana@example.com", ok, domain.RoleSeller},
		"admin":              {"ana@example.com", ok, domain.RoleAdmin},
		"unknown role":       {"ana@example.com", ok, "root"},
	} {
		if _, err := domain.NewRegistration(tc.email, tc.password, tc.role); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestPasswordLimitsAreInclusive(t *testing.T) {
	for _, pw := range []string{"12345678", strings.Repeat("a", 72), "éééééééé"} {
		if _, err := domain.NewRegistration("ana@example.com", pw, ""); err != nil {
			t.Errorf("password of %d bytes: %v", len(pw), err)
		}
	}
}

func TestEmailOfMaximumLengthIsAccepted(t *testing.T) {
	email := strings.Repeat("a", 64) + "@" + strings.Repeat("b", 185) + ".com" // 254 bytes
	if _, err := domain.NewRegistration(email, "12345678", ""); err != nil {
		t.Errorf("254-byte email: %v", err)
	}
}
