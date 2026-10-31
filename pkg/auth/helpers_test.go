package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// signer is a test RSA key with a kid, like identity-service's.
type signer struct {
	kid  string
	priv *rsa.PrivateKey
}

func newSigner(t *testing.T, kid string) signer {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return signer{kid: kid, priv: priv}
}

// token signs claims sub/role/kid that expire after ttl from now.
func (s signer) token(t *testing.T, sub, role string, now time.Time, ttl time.Duration) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": sub, "role": role, "kid": s.kid,
		"iat": now.Unix(), "exp": now.Add(ttl).Unix(),
	})
	tok.Header["kid"] = s.kid
	raw, err := tok.SignedString(s.priv)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// jwksServer serves the public halves of the current signers and counts fetches.
type jwksServer struct {
	*httptest.Server
	mu      sync.Mutex
	signers []signer
	down    atomic.Bool
	fetches atomic.Int32
	// gate, when set, blocks each fetch until it is closed.
	gate atomic.Pointer[chan struct{}]
}

func newJWKSServer(t *testing.T, signers ...signer) *jwksServer {
	t.Helper()
	j := &jwksServer{signers: signers}
	j.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		j.fetches.Add(1)
		if g := j.gate.Load(); g != nil {
			<-*g
		}
		if j.down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		j.mu.Lock()
		defer j.mu.Unlock()
		var keys []map[string]string
		for _, s := range j.signers {
			keys = append(keys, map[string]string{
				"kty": "RSA", "alg": "RS256", "use": "sig", "kid": s.kid,
				"n": base64.RawURLEncoding.EncodeToString(s.priv.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.priv.E)).Bytes()),
			})
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(j.Close)
	return j
}

func (j *jwksServer) set(signers ...signer) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.signers = signers
}

// clock is a settable time source.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
