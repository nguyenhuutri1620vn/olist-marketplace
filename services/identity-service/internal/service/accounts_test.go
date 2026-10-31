package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/service"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/token"
)

// memAccounts is an in-memory account store keyed by normalized email.
type memAccounts map[string]domain.Account

func (m memAccounts) CreateAccount(_ context.Context, a domain.Account) error {
	if _, ok := m[a.Email]; ok {
		return domain.ErrEmailTaken
	}
	m[a.Email] = a
	return nil
}

func (m memAccounts) AccountByEmail(_ context.Context, email string) (domain.Account, error) {
	a, ok := m[email]
	if !ok {
		return domain.Account{}, domain.ErrAccountNotFound
	}
	return a, nil
}

// fakeIssuer returns "<sub>|<role>" as the token.
type fakeIssuer struct{}

func (fakeIssuer) Issue(sub string, role domain.Role) (token.Token, error) {
	return token.Token{AccessToken: sub + "|" + string(role), ExpiresIn: 3600}, nil
}

// countingHasher wraps bcrypt at minimum cost and counts hashes and comparisons.
type countingHasher struct{ hashes, compares int }

func (h *countingHasher) Hash(pw string) (string, error) {
	h.hashes++
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	return string(b), err
}

func (h *countingHasher) Compare(hash, pw string) error {
	h.compares++
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw))
}

func newService(t *testing.T) (*service.Accounts, memAccounts, *countingHasher) {
	t.Helper()
	store, hasher := memAccounts{}, &countingHasher{}
	svc, err := service.NewAccounts(store, hasher, fakeIssuer{}, service.NewV7ID)
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, hasher
}

func TestRegisterCreatesCustomerAndLogsIn(t *testing.T) {
	svc, store, _ := newService(t)
	tok, err := svc.Register(context.Background(), " Ana@Example.com ", "s3cret-pw", "")
	if err != nil {
		t.Fatal(err)
	}
	a, ok := store["ana@example.com"]
	if !ok {
		t.Fatalf("no account stored under the normalized email: %v", store)
	}
	if id, err := uuid.Parse(a.ID); err != nil || id.Version() != 7 {
		t.Errorf("account id %q is not a UUIDv7", a.ID)
	}
	if a.Role != domain.RoleCustomer || a.PasswordHash == "s3cret-pw" || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte("s3cret-pw")) != nil {
		t.Errorf("account = %+v, want a Customer with a bcrypt hash", a)
	}
	if tok.AccessToken != a.ID+"|customer" {
		t.Errorf("token = %q, want one issued for the new Customer", tok.AccessToken)
	}
}

func TestRegisterRejectsTakenEmailInAnyCase(t *testing.T) {
	svc, store, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, "ana@example.com", "s3cret-pw", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, "ANA@example.com", "other-pw-1", ""); !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("err = %v, want ErrEmailTaken", err)
	}
	if len(store) != 1 {
		t.Errorf("accounts = %d, want 1", len(store))
	}
}

func TestRegisterRejectsInvalidInputAndStoresNothing(t *testing.T) {
	svc, store, _ := newService(t)
	if _, err := svc.Register(context.Background(), "ana@example.com", "s3cret-pw", domain.RoleAdmin); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
	if len(store) != 0 {
		t.Errorf("accounts = %d, want 0", len(store))
	}
}

func TestLoginWithCorrectPasswordInAnyEmailCase(t *testing.T) {
	svc, store, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, "ana@example.com", "s3cret-pw", ""); err != nil {
		t.Fatal(err)
	}
	tok, err := svc.Login(ctx, " ANA@example.com", "s3cret-pw")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != store["ana@example.com"].ID+"|customer" {
		t.Errorf("token = %q", tok.AccessToken)
	}
}

func TestLoginFailuresLookAlike(t *testing.T) {
	svc, _, hasher := newService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, "ana@example.com", "s3cret-pw", ""); err != nil {
		t.Fatal(err)
	}
	before := hasher.compares
	_, wrongPw := svc.Login(ctx, "ana@example.com", "wrong-pw-1")
	_, unknown := svc.Login(ctx, "nobody@example.com", "s3cret-pw")
	for name, err := range map[string]error{"wrong password": wrongPw, "unknown email": unknown} {
		if !errors.Is(err, domain.ErrBadCredentials) {
			t.Errorf("%s: err = %v, want ErrBadCredentials", name, err)
		}
	}
	if wrongPw.Error() != unknown.Error() {
		t.Errorf("errors differ: %q vs %q", wrongPw, unknown)
	}
	// The unknown email still pays for a bcrypt comparison, so timing does not tell.
	if got := hasher.compares - before; got != 2 {
		t.Errorf("bcrypt comparisons = %d, want 2", got)
	}
}

func TestLoginRejectsPasswordsBcryptWouldTruncate(t *testing.T) {
	svc, _, hasher := newService(t)
	ctx := context.Background()
	pw := strings.Repeat("a", domain.MaxPasswordBytes)
	if _, err := svc.Register(ctx, "ana@example.com", pw, ""); err != nil {
		t.Fatal(err)
	}
	before := hasher.compares
	// bcrypt ignores bytes past 72, so this would match without the length check.
	if _, err := svc.Login(ctx, "ana@example.com", pw+"X"); !errors.Is(err, domain.ErrBadCredentials) {
		t.Errorf("73-byte password err = %v, want ErrBadCredentials", err)
	}
	if hasher.compares-before != 1 {
		t.Errorf("bcrypt comparisons = %d, want 1 (same cost as any failed login)", hasher.compares-before)
	}
}
