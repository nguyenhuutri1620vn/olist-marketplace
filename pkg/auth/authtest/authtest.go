// Package authtest signs test tokens and serves their JWKS, so services and
// the gateway can test role checks without identity-service.
package authtest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
)

// Issuer signs RS256 tokens shaped like identity-service's and serves the
// matching JWKS.
type Issuer struct {
	kid  string
	priv *rsa.PrivateKey
	// JWKSURL serves the issuer's public key.
	JWKSURL string
}

// NewIssuer starts a JWKS server that is closed when t ends.
func NewIssuer(t testing.TB) *Issuer {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	iss := &Issuer{kid: "test-key", priv: priv}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": iss.kid,
			"n": base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
		}}})
	}))
	t.Cleanup(srv.Close)
	iss.JWKSURL = srv.URL
	return iss
}

// Verifier verifies tokens from this issuer.
func (i *Issuer) Verifier() *auth.Verifier {
	return auth.NewVerifier(auth.NewKeySet(i.JWKSURL))
}

// Token signs a token for sub with role, valid for an hour.
func (i *Issuer) Token(t testing.TB, sub string, role auth.Role) string {
	t.Helper()
	return i.TokenAt(t, sub, role, time.Now(), time.Hour)
}

// TokenAt signs a token issued at iat that lives for ttl.
func (i *Issuer) TokenAt(t testing.TB, sub string, role auth.Role, iat time.Time, ttl time.Duration) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": sub, "role": string(role), "kid": i.kid,
		"iat": iat.Unix(), "exp": iat.Add(ttl).Unix(),
	})
	tok.Header["kid"] = i.kid
	raw, err := tok.SignedString(i.priv)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
