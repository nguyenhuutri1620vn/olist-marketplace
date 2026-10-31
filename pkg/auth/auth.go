// Package auth verifies identity-service's RS256 tokens against its JWKS,
// carries the caller's account id and role in the context, and enforces a
// per-RPC role policy (AD-14, FR16).
package auth

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Role is the token's role claim.
type Role string

const (
	RoleCustomer Role = "customer"
	RoleSeller   Role = "seller"
	RoleAdmin    Role = "admin"
)

var (
	// ErrUnauthenticated means the caller has no valid, unexpired token.
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrKeysUnavailable means no verification key could be loaded (identity's
	// JWKS is unreachable and nothing is cached): a server-side outage, not a
	// bad token.
	ErrKeysUnavailable = errors.New("signing keys unavailable")
)

// Claims identify the caller: the Account id (customer_id for a Customer,
// seller_id for a Seller) and its role.
type Claims struct {
	AccountID string
	Role      Role
}

type config struct {
	now    func() time.Time
	client *http.Client
}

// Option configures a KeySet or Verifier.
type Option func(*config)

// WithClock replaces time.Now (for tests).
func WithClock(now func() time.Time) Option { return func(c *config) { c.now = now } }

// WithHTTPClient replaces the client used to fetch the JWKS.
func WithHTTPClient(client *http.Client) Option { return func(c *config) { c.client = client } }

func newConfig(opts []Option) config {
	c := config{now: time.Now, client: &http.Client{Timeout: 5 * time.Second}}
	for _, o := range opts {
		o(&c)
	}
	return c
}

type claimsKey struct{}
type tokenKey struct{}

// WithClaims returns ctx carrying the caller's claims and raw token.
func WithClaims(ctx context.Context, c Claims, raw string) context.Context {
	return context.WithValue(context.WithValue(ctx, claimsKey{}, c), tokenKey{}, raw)
}

// FromContext returns the verified caller, if any.
func FromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(Claims)
	return c, ok
}

// TokenFromContext returns the caller's raw JWT, for forwarding.
func TokenFromContext(ctx context.Context) (string, bool) {
	t, ok := ctx.Value(tokenKey{}).(string)
	return t, ok && t != ""
}
