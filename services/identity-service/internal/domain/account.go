// Package domain holds identity's Accounts, roles and the rules for
// registering them.
package domain

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// ErrInvalid marks input that breaks a registration rule.
	ErrInvalid = errors.New("invalid")
	// ErrEmailTaken means an Account already uses the normalized email.
	ErrEmailTaken = errors.New("email already registered")
	// ErrAccountNotFound means no Account has the email.
	ErrAccountNotFound = errors.New("account not found")
	// ErrBadCredentials covers both an unknown email and a wrong password, so
	// callers cannot tell which.
	ErrBadCredentials = errors.New("invalid email or password")
)

// Role is an Account's role, carried in the token's role claim.
type Role string

const (
	RoleCustomer Role = "customer"
	RoleSeller   Role = "seller"
	RoleAdmin    Role = "admin"
)

// ReservedDomain belongs to seeded demo Accounts (Story 1.5); registration
// cannot use it.
const ReservedDomain = "demo.olist.local"

// MaxEmailBytes is the longest address SMTP allows (RFC 5321).
const MaxEmailBytes = 254

// Password limits: at least 8 characters, at most 72 bytes (bcrypt's limit).
const (
	MinPasswordChars = 8
	MaxPasswordBytes = 72
)

// Account is a registered or seeded user.
type Account struct {
	ID           string
	Email        string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
}

// Registration is a validated request to create a Customer Account.
type Registration struct {
	Email    string
	Password string
	Role     Role
}

// NormalizeEmail trims and lower-cases an email; Accounts are unique on it.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// NewRegistration validates a registration. An empty role means Customer;
// registration never creates a Seller or Admin (FR14).
func NewRegistration(email, password string, role Role) (Registration, error) {
	if role != "" && role != RoleCustomer {
		return Registration{}, invalid("registration creates Customer Accounts only, not %q", role)
	}
	email = NormalizeEmail(email)
	if len(email) > MaxEmailBytes {
		return Registration{}, invalid("email is longer than %d bytes", MaxEmailBytes)
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		return Registration{}, invalid("email %q is not a plain address", email)
	}
	domain := email[strings.LastIndexByte(email, '@')+1:]
	if !strings.Contains(domain, ".") {
		return Registration{}, invalid("email domain %q has no dot", domain)
	}
	if domain == ReservedDomain || strings.HasSuffix(domain, "."+ReservedDomain) {
		return Registration{}, invalid("emails in @%s are reserved for demo Accounts", ReservedDomain)
	}
	if utf8.RuneCountInString(password) < MinPasswordChars {
		return Registration{}, invalid("password must have at least %d characters", MinPasswordChars)
	}
	if len(password) > MaxPasswordBytes {
		return Registration{}, invalid("password must be at most %d bytes", MaxPasswordBytes)
	}
	return Registration{Email: email, Password: password, Role: RoleCustomer}, nil
}
