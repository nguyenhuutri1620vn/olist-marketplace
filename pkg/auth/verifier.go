package auth

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// Keys looks up a verification key by kid.
type Keys interface {
	Key(ctx context.Context, kid string) (*rsa.PublicKey, error)
}

// Verifier checks RS256 tokens: signature by a JWKS key, expiry, a subject and
// a known role.
type Verifier struct {
	keys Keys
	cfg  config
}

// NewVerifier verifies against keys.
func NewVerifier(keys Keys, opts ...Option) *Verifier {
	return &Verifier{keys: keys, cfg: newConfig(opts)}
}

type tokenClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Verify returns the caller's claims. A bad, expired or unknown token wraps
// ErrUnauthenticated; an identity outage with no cached keys wraps
// ErrKeysUnavailable.
func (v *Verifier) Verify(ctx context.Context, raw string) (Claims, error) {
	var tc tokenClaims
	_, err := jwt.ParseWithClaims(raw, &tc, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("%w: token has no kid", ErrUnauthenticated)
		}
		return v.keys.Key(ctx, kid)
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithExpirationRequired(), jwt.WithTimeFunc(v.cfg.now))
	if errors.Is(err, ErrKeysUnavailable) {
		return Claims{}, err
	}
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	role := Role(tc.Role)
	switch role {
	case RoleCustomer, RoleSeller, RoleAdmin:
	default:
		return Claims{}, fmt.Errorf("%w: unknown role %q", ErrUnauthenticated, tc.Role)
	}
	if tc.Subject == "" {
		return Claims{}, fmt.Errorf("%w: token has no subject", ErrUnauthenticated)
	}
	return Claims{AccountID: tc.Subject, Role: role}, nil
}
