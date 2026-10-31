// Package token signs identity's RS256 JWTs and publishes the matching JWKS
// (AD-14).
package token

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
)

// TTL is the token lifetime; there is no refresh token.
const TTL = time.Hour

const keyBits = 2048

// Key is an RSA signing key and its kid.
type Key struct {
	ID      string
	Private *rsa.PrivateKey
}

// GenerateKey creates a new RSA signing key.
func GenerateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, keyBits)
}

// NewKey wraps priv with its kid: the RFC 7638 SHA-256 thumbprint of the
// public key, so the same key always has the same kid.
func NewKey(priv *rsa.PrivateKey) Key {
	n, e := b64(priv.N), b64(big.NewInt(int64(priv.E)))
	// RFC 7638: required members only, in lexicographic order, no whitespace.
	sum := sha256.Sum256([]byte(`{"e":"` + e + `","kty":"RSA","n":"` + n + `"}`))
	return Key{ID: base64.RawURLEncoding.EncodeToString(sum[:]), Private: priv}
}

func b64(i *big.Int) string { return base64.RawURLEncoding.EncodeToString(i.Bytes()) }

// Token is an issued access token.
type Token struct {
	AccessToken string
	ExpiresIn   int64 // seconds
}

// Issuer signs tokens with one key.
type Issuer struct {
	key Key
	now func() time.Time
}

// NewIssuer returns an Issuer signing with key; now is the clock.
func NewIssuer(key Key, now func() time.Time) *Issuer {
	return &Issuer{key: key, now: now}
}

type claims struct {
	Role string `json:"role"`
	Kid  string `json:"kid"`
	jwt.RegisteredClaims
}

// Issue signs a token for Account sub with role.
func (i *Issuer) Issue(sub string, role domain.Role) (Token, error) {
	now := i.now().Truncate(time.Second)
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims{
		Role: string(role),
		Kid:  i.key.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(TTL)),
		},
	})
	t.Header["kid"] = i.key.ID
	signed, err := t.SignedString(i.key.Private)
	if err != nil {
		return Token{}, err
	}
	return Token{AccessToken: signed, ExpiresIn: int64(TTL / time.Second)}, nil
}

type jwk struct {
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS renders the public half of keys as a JSON Web Key Set.
func JWKS(keys ...Key) ([]byte, error) {
	set := struct {
		Keys []jwk `json:"keys"`
	}{Keys: []jwk{}}
	for _, k := range keys {
		set.Keys = append(set.Keys, jwk{
			Kty: "RSA", Alg: "RS256", Use: "sig", Kid: k.ID,
			N: b64(k.Private.N), E: b64(big.NewInt(int64(k.Private.E))),
		})
	}
	return json.Marshal(set)
}
