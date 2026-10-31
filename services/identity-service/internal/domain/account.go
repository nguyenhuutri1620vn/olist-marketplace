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

	"github.com/google/uuid"

	"github.com/nguyenhuutri1620vn/olist-marketplace/contracts/accounts"
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

// ReservedDomain belongs to seeded demo Accounts; registration cannot use it.
const ReservedDomain = accounts.DemoDomain

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
	email, domain, err := parseEmail(email)
	if err != nil {
		return Registration{}, err
	}
	if domain == ReservedDomain || strings.HasSuffix(domain, "."+ReservedDomain) {
		return Registration{}, invalid("emails in @%s are reserved for demo Accounts", ReservedDomain)
	}
	if err := ValidatePassword(password); err != nil {
		return Registration{}, err
	}
	return Registration{Email: email, Password: password, Role: RoleCustomer}, nil
}

// parseEmail normalizes email and checks it is a plain address whose domain
// has a dot; it returns the address and its domain.
func parseEmail(email string) (string, string, error) {
	email = NormalizeEmail(email)
	if len(email) > MaxEmailBytes {
		return "", "", invalid("email is longer than %d bytes", MaxEmailBytes)
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		return "", "", invalid("email %q is not a plain address", email)
	}
	domain := email[strings.LastIndexByte(email, '@')+1:]
	if !strings.Contains(domain, ".") {
		return "", "", invalid("email domain %q has no dot", domain)
	}
	return email, domain, nil
}

// ValidatePassword enforces the password length rules.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordChars {
		return invalid("password must have at least %d characters", MinPasswordChars)
	}
	if len(password) > MaxPasswordBytes {
		return invalid("password must be at most %d bytes", MaxPasswordBytes)
	}
	return nil
}

// Seed is a validated demo Account to upsert (FR15).
type Seed struct {
	ID    string // canonical uuid
	Email string
	Role  Role
}

// NewSeed validates a demo Account: a dataset or canonical uuid id, an email
// in the reserved domain, and a known role.
func NewSeed(id, email string, role Role) (Seed, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return Seed{}, invalid("account id %q is not a uuid", id)
	}
	email, domain, err := parseEmail(email)
	if err != nil {
		return Seed{}, err
	}
	if domain != ReservedDomain {
		return Seed{}, invalid("seeded email %q must be in @%s", email, ReservedDomain)
	}
	switch role {
	case RoleCustomer, RoleSeller, RoleAdmin:
	default:
		return Seed{}, invalid("unknown role %q", role)
	}
	return Seed{ID: u.String(), Email: email, Role: role}, nil
}
