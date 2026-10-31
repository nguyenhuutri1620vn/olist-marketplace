package token_test

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"regexp"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/token"
)

func newKey(t *testing.T) token.Key {
	t.Helper()
	priv, err := token.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return token.NewKey(priv)
}

// publicKeys parses a JWKS document the way a verifier would.
func publicKeys(t *testing.T, doc []byte) map[string]*rsa.PublicKey {
	t.Helper()
	var set struct {
		Keys []struct {
			Kty, Alg, Use, Kid, N, E string
		} `json:"keys"`
	}
	if err := json.Unmarshal(doc, &set); err != nil {
		t.Fatal(err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Alg != "RS256" || k.Use != "sig" {
			t.Errorf("key %s: kty %q alg %q use %q", k.Kid, k.Kty, k.Alg, k.Use)
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			t.Fatalf("key %s: bad n/e encoding", k.Kid)
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	return keys
}

func TestIssuedTokenVerifiesAgainstJWKS(t *testing.T) {
	key := newKey(t)
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	iss := token.NewIssuer(key, func() time.Time { return now })

	tok, err := iss.Issue("0192f3a0-0000-7000-8000-000000000001", domain.RoleCustomer)
	if err != nil {
		t.Fatal(err)
	}
	if tok.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn = %d, want 3600", tok.ExpiresIn)
	}

	doc, err := token.JWKS(key)
	if err != nil {
		t.Fatal(err)
	}
	keys := publicKeys(t, doc)
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(tok.AccessToken, claims, func(tk *jwt.Token) (any, error) {
		return keys[tk.Header["kid"].(string)], nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil || !parsed.Valid {
		t.Fatalf("token does not verify against JWKS: %v", err)
	}
	if claims["sub"] != "0192f3a0-0000-7000-8000-000000000001" || claims["role"] != "customer" || claims["kid"] != key.ID {
		t.Errorf("claims = %v", claims)
	}
	if parsed.Header["kid"] != key.ID {
		t.Errorf("header kid = %v, want %s", parsed.Header["kid"], key.ID)
	}
	iat, _ := claims.GetIssuedAt()
	exp, _ := claims.GetExpirationTime()
	if !iat.Time.Equal(now) || exp.Time.Sub(iat.Time) != time.Hour {
		t.Errorf("iat %v exp %v, want exp = iat + 1h", iat, exp)
	}
}

func TestTokenExpiresAfterOneHour(t *testing.T) {
	key := newKey(t)
	issued := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	tok, err := token.NewIssuer(key, func() time.Time { return issued }).Issue("sub", domain.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	_, err = jwt.Parse(tok.AccessToken, func(*jwt.Token) (any, error) { return key.Private.Public(), nil },
		jwt.WithTimeFunc(func() time.Time { return issued.Add(time.Hour + time.Second) }))
	if err == nil {
		t.Error("token still valid after 1 h")
	}
}

func TestKeyIDIsAStableThumbprint(t *testing.T) {
	a, b := newKey(t), newKey(t)
	if again := token.NewKey(a.Private); again.ID != a.ID {
		t.Errorf("kid changed for the same key: %s vs %s", a.ID, again.ID)
	}
	if a.ID == b.ID {
		t.Error("two keys share a kid")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(a.ID) {
		t.Errorf("kid %q is not a base64url SHA-256 thumbprint", a.ID)
	}
}
