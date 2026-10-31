package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// RefreshInterval limits JWKS refetches on unknown kids once keys are cached.
const RefreshInterval = time.Minute

// emptyRetryInterval limits fetches while no key was ever loaded (identity
// may start after the caller).
const emptyRetryInterval = time.Second

// fetchTimeout bounds one JWKS fetch, independent of the caller's request.
const fetchTimeout = 5 * time.Second

// KeySet caches identity's JWKS. An unknown kid triggers a refetch at most
// once per RefreshInterval; a failed refetch keeps the last good keys. Lookups
// of cached kids never wait for a fetch.
type KeySet struct {
	url string
	cfg config

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	lastFetch time.Time
	inflight  chan struct{} // closed when the running fetch ends
}

// NewKeySet caches the JWKS served at url. Nothing is fetched until first use.
func NewKeySet(url string, opts ...Option) *KeySet {
	return &KeySet{url: url, cfg: newConfig(opts), keys: map[string]*rsa.PublicKey{}}
}

// Key returns the public key for kid. An unknown kid wraps ErrUnauthenticated;
// when no key was ever loaded and identity cannot be reached, the error wraps
// ErrKeysUnavailable instead.
func (k *KeySet) Key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	k.mu.Lock()
	if key, ok := k.keys[kid]; ok {
		k.mu.Unlock()
		return key, nil
	}
	wait := k.inflight
	if wait == nil && k.mayFetch() {
		k.lastFetch = k.cfg.now()
		done := make(chan struct{})
		k.inflight, wait = done, done
		k.mu.Unlock()
		// The fetch outlives a canceled caller, so one aborted request does
		// not use up the refresh budget.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		keys, err := k.fetch(fctx)
		cancel()
		k.mu.Lock()
		if err != nil {
			slog.WarnContext(ctx, "jwks refresh failed; keeping cached keys", "url", k.url, "err", err)
		} else {
			k.keys = keys
		}
		k.inflight = nil
		close(done)
		k.mu.Unlock()
	} else {
		k.mu.Unlock()
	}
	if wait != nil {
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if key, ok := k.keys[kid]; ok {
		return key, nil
	}
	if len(k.keys) == 0 {
		return nil, fmt.Errorf("%w: jwks at %s", ErrKeysUnavailable, k.url)
	}
	return nil, fmt.Errorf("%w: unknown key id %q", ErrUnauthenticated, kid)
}

// mayFetch applies the refresh rate limit; k.mu must be held.
func (k *KeySet) mayFetch() bool {
	wait := RefreshInterval
	if len(k.keys) == 0 {
		wait = emptyRetryInterval
	}
	return k.lastFetch.IsZero() || k.cfg.now().Sub(k.lastFetch) >= wait
}

func (k *KeySet) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := k.cfg.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct{ Kty, Alg, Kid, N, E string } `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, j := range set.Keys {
		if j.Kty != "RSA" || j.Alg != "RS256" || j.Kid == "" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(j.N)
		e, err2 := base64.RawURLEncoding.DecodeString(j.E)
		eInt := new(big.Int).SetBytes(e)
		if err1 != nil || err2 != nil || len(n) == 0 || !eInt.IsInt64() || eInt.Int64() < 3 {
			// One bad entry must not hide the others (e.g. during rotation).
			slog.Warn("jwks key skipped: bad n or e", "kid", j.Kid)
			continue
		}
		keys[j.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(eInt.Int64())}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("jwks has no RS256 keys")
	}
	return keys, nil
}
