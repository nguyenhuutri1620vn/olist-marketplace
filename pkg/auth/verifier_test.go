package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
)

var t0 = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func newVerifier(t *testing.T, j *jwksServer, c *clock) *auth.Verifier {
	t.Helper()
	return auth.NewVerifier(auth.NewKeySet(j.URL, auth.WithClock(c.Now)), auth.WithClock(c.Now))
}

func TestVerifyReturnsClaims(t *testing.T) {
	s := newSigner(t, "k1")
	c := &clock{now: t0}
	v := newVerifier(t, newJWKSServer(t, s), c)
	claims, err := v.Verify(context.Background(), s.token(t, "acc-1", "seller", t0, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if claims != (auth.Claims{AccountID: "acc-1", Role: auth.RoleSeller}) {
		t.Errorf("claims = %+v", claims)
	}
}

func TestVerifyRejectsBadTokens(t *testing.T) {
	s, other := newSigner(t, "k1"), newSigner(t, "k1")
	c := &clock{now: t0}
	v := newVerifier(t, newJWKSServer(t, s), c)
	for name, raw := range map[string]string{
		"empty":         "",
		"garbage":       "not.a.jwt",
		"expired":       s.token(t, "acc-1", "customer", t0.Add(-2*time.Hour), time.Hour),
		"bad signature": other.token(t, "acc-1", "customer", t0, time.Hour),
		"unknown role":  s.token(t, "acc-1", "root", t0, time.Hour),
		"no subject":    s.token(t, "", "customer", t0, time.Hour),
	} {
		if _, err := v.Verify(context.Background(), raw); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Errorf("%s: err = %v, want ErrUnauthenticated", name, err)
		}
	}
}

func TestUnknownKidRefetchesAtMostOncePerMinute(t *testing.T) {
	s1, s2, s3 := newSigner(t, "k1"), newSigner(t, "k2"), newSigner(t, "k3")
	j := newJWKSServer(t, s1)
	c := &clock{now: t0}
	v := newVerifier(t, j, c)
	ctx := context.Background()
	if _, err := v.Verify(ctx, s1.token(t, "a", "customer", t0, time.Hour)); err != nil {
		t.Fatal(err)
	}
	// identity rotates to k2: the unknown kid triggers one refetch.
	j.set(s1, s2)
	c.Add(2 * time.Minute)
	if _, err := v.Verify(ctx, s2.token(t, "a", "customer", c.Now(), time.Hour)); err != nil {
		t.Fatalf("k2 after refetch: %v", err)
	}
	before := j.fetches.Load()
	// k3 is unknown and published only now; within the minute no refetch happens.
	j.set(s1, s2, s3)
	c.Add(30 * time.Second)
	if _, err := v.Verify(ctx, s3.token(t, "a", "customer", c.Now(), time.Hour)); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("k3 within a minute: err = %v, want ErrUnauthenticated", err)
	}
	if got := j.fetches.Load() - before; got != 0 {
		t.Errorf("refetches within a minute = %d, want 0", got)
	}
	c.Add(31 * time.Second)
	if _, err := v.Verify(ctx, s3.token(t, "a", "customer", c.Now(), time.Hour)); err != nil {
		t.Errorf("k3 after a minute: %v", err)
	}
}

func TestUnknownKidAfterRefetchIsUnauthenticated(t *testing.T) {
	s1, stranger := newSigner(t, "k1"), newSigner(t, "kx")
	c := &clock{now: t0}
	v := newVerifier(t, newJWKSServer(t, s1), c)
	if _, err := v.Verify(context.Background(), stranger.token(t, "a", "admin", t0, time.Hour)); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("err = %v, want ErrUnauthenticated", err)
	}
}

func TestCachedKeysSurviveIdentityOutage(t *testing.T) {
	s1, s2 := newSigner(t, "k1"), newSigner(t, "k2")
	j := newJWKSServer(t, s1)
	c := &clock{now: t0}
	v := newVerifier(t, j, c)
	ctx := context.Background()
	if _, err := v.Verify(ctx, s1.token(t, "a", "customer", t0, time.Hour)); err != nil {
		t.Fatal(err)
	}
	j.down.Store(true)
	c.Add(2 * time.Minute)
	// An unknown kid triggers a failed refetch; the cached k1 must still work.
	v.Verify(ctx, s2.token(t, "a", "customer", c.Now(), time.Hour))
	if _, err := v.Verify(ctx, s1.token(t, "a", "customer", c.Now(), time.Hour)); err != nil {
		t.Errorf("cached key after outage: %v", err)
	}
}

func TestFirstFetchRetriesSoonWhenIdentityStartsLate(t *testing.T) {
	s1 := newSigner(t, "k1")
	j := newJWKSServer(t, s1)
	j.down.Store(true)
	c := &clock{now: t0}
	v := newVerifier(t, j, c)
	ctx := context.Background()
	if _, err := v.Verify(ctx, s1.token(t, "a", "customer", t0, time.Hour)); err == nil {
		t.Fatal("verified with identity down and no cached keys")
	}
	j.down.Store(false)
	c.Add(2 * time.Second)
	if _, err := v.Verify(ctx, s1.token(t, "a", "customer", c.Now(), time.Hour)); err != nil {
		t.Errorf("after identity came up: %v", err)
	}
}

func TestCachedKeysDoNotWaitForASlowRefetch(t *testing.T) {
	s1, s2 := newSigner(t, "k1"), newSigner(t, "k2")
	j := newJWKSServer(t, s1)
	c := &clock{now: t0}
	v := newVerifier(t, j, c)
	ctx := context.Background()
	if _, err := v.Verify(ctx, s1.token(t, "a", "customer", t0, time.Hour)); err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	j.gate.Store(&gate)
	defer close(gate)
	c.Add(2 * time.Minute)
	go v.Verify(ctx, s2.token(t, "a", "customer", c.Now(), time.Hour)) // starts a blocked refetch
	time.Sleep(100 * time.Millisecond)
	done := make(chan error, 1)
	go func() { _, err := v.Verify(ctx, s1.token(t, "a", "customer", c.Now(), time.Hour)); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("cached key: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("verifying a cached key waited for the JWKS refetch")
	}
}

func TestCanceledCallerDoesNotSpoilTheRefetch(t *testing.T) {
	s1, s2 := newSigner(t, "k1"), newSigner(t, "k2")
	j := newJWKSServer(t, s1, s2)
	c := &clock{now: t0}
	v := newVerifier(t, j, c)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v.Verify(ctx, s1.token(t, "a", "customer", t0, time.Hour)) // first fetch under a canceled ctx
	if _, err := v.Verify(context.Background(), s2.token(t, "a", "customer", t0, time.Hour)); err != nil {
		t.Errorf("after a canceled caller: %v", err)
	}
}

func TestKeysUnavailableIsNotAnAuthFailure(t *testing.T) {
	s1 := newSigner(t, "k1")
	j := newJWKSServer(t, s1)
	j.down.Store(true)
	v := newVerifier(t, j, &clock{now: t0})
	_, err := v.Verify(context.Background(), s1.token(t, "a", "customer", t0, time.Hour))
	if !errors.Is(err, auth.ErrKeysUnavailable) || errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("err = %v, want ErrKeysUnavailable (not ErrUnauthenticated)", err)
	}
}

func TestMissingKidDoesNotFetch(t *testing.T) {
	s1 := newSigner(t, "")
	j := newJWKSServer(t, newSigner(t, "k1"))
	v := newVerifier(t, j, &clock{now: t0})
	if _, err := v.Verify(context.Background(), s1.token(t, "a", "customer", t0, time.Hour)); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("err = %v, want ErrUnauthenticated", err)
	}
	if j.fetches.Load() != 0 {
		t.Errorf("fetched the JWKS %d times for a token without kid", j.fetches.Load())
	}
}
