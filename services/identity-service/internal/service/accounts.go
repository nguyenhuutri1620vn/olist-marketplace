// Package service holds identity's use cases and the ports they depend on.
package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/token"
)

// AccountStore persists Accounts by normalized email.
type AccountStore interface {
	CreateAccount(context.Context, domain.Account) error
	AccountByEmail(context.Context, string) (domain.Account, error)
}

// Hasher hashes and checks passwords (bcrypt in production).
type Hasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

// Issuer signs access tokens.
type Issuer interface {
	Issue(sub string, role domain.Role) (token.Token, error)
}

// NewV7ID returns a new UUIDv7 Account id.
func NewV7ID() (string, error) {
	id, err := uuid.NewV7()
	return id.String(), err
}

// Accounts registers Customers and logs Accounts in (FR14).
type Accounts struct {
	store  AccountStore
	hasher Hasher
	issuer Issuer
	newID  func() (string, error)
	// dummyHash is compared against when the email is unknown, so a failed
	// login costs the same bcrypt work either way.
	dummyHash string
}

// NewAccounts wires the use cases.
func NewAccounts(store AccountStore, hasher Hasher, issuer Issuer, newID func() (string, error)) (*Accounts, error) {
	dummy, err := hasher.Hash("dummy-password-for-unknown-emails")
	if err != nil {
		return nil, err
	}
	return &Accounts{store: store, hasher: hasher, issuer: issuer, newID: newID, dummyHash: dummy}, nil
}

// Register creates a Customer Account and returns a token for it.
func (s *Accounts) Register(ctx context.Context, email, password string, role domain.Role) (token.Token, error) {
	reg, err := domain.NewRegistration(email, password, role)
	if err != nil {
		return token.Token{}, err
	}
	id, err := s.newID()
	if err != nil {
		return token.Token{}, err
	}
	hash, err := s.hasher.Hash(reg.Password)
	if err != nil {
		return token.Token{}, err
	}
	if err := s.store.CreateAccount(ctx, domain.Account{ID: id, Email: reg.Email, PasswordHash: hash, Role: reg.Role}); err != nil {
		return token.Token{}, err
	}
	return s.issuer.Issue(id, reg.Role)
}

// Login checks an email and password. An unknown email and a wrong password
// both return domain.ErrBadCredentials.
func (s *Accounts) Login(ctx context.Context, email, password string) (token.Token, error) {
	// bcrypt ignores bytes past 72, so a longer password could match a hash
	// made from its prefix; registration never stores one that long.
	if len(password) > domain.MaxPasswordBytes {
		_ = s.hasher.Compare(s.dummyHash, password[:domain.MaxPasswordBytes])
		return token.Token{}, domain.ErrBadCredentials
	}
	a, err := s.store.AccountByEmail(ctx, domain.NormalizeEmail(email))
	if errors.Is(err, domain.ErrAccountNotFound) {
		_ = s.hasher.Compare(s.dummyHash, password)
		return token.Token{}, domain.ErrBadCredentials
	}
	if err != nil {
		return token.Token{}, err
	}
	if s.hasher.Compare(a.PasswordHash, password) != nil {
		return token.Token{}, domain.ErrBadCredentials
	}
	return s.issuer.Issue(a.ID, a.Role)
}
