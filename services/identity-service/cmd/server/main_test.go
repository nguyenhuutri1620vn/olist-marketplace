package main

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// schemaDSN returns DBMIGRATE_TEST_DSN pointed at a fresh schema.
func schemaDSN(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()
	base := os.Getenv("DBMIGRATE_TEST_DSN")
	if base == "" {
		t.Skip("DBMIGRATE_TEST_DSN not set")
	}
	admin, err := pgxpool.New(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("identity_main_%d", time.Now().UnixNano())
	if _, err := admin.Exec(context.Background(), `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	pool, err := pgxpool.New(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return u.String(), pool
}

// start runs identity-service until the returned stop is called.
func start(t *testing.T) (grpcAddr, httpAddr string, stop func()) {
	t.Helper()
	grpcAddr, httpAddr = freeAddr(t), freeAddr(t)
	t.Setenv("GRPC_ADDR", grpcAddr)
	t.Setenv("HTTP_ADDR", httpAddr)
	t.Setenv("OPS_ADDR", freeAddr(t))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, identityService()) }()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("Run: %v", err)
			}
		})
	}
	t.Cleanup(stop)
	deadline := time.Now().Add(15 * time.Second)
	for app.Healthcheck(context.Background()) != nil {
		select {
		case err := <-done:
			t.Fatalf("identity-service stopped during startup: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("identity-service did not become ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return grpcAddr, httpAddr, stop
}

type jwks struct {
	Keys []struct{ Kid, N, E string } `json:"keys"`
}

func fetchJWKS(t *testing.T, httpAddr string) jwks {
	t.Helper()
	resp, err := http.Get("http://" + httpAddr + "/.well-known/jwks.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("JWKS status %d", resp.StatusCode)
	}
	var set jwks
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil || len(set.Keys) != 1 {
		t.Fatalf("JWKS: %v, %+v", err, set)
	}
	return set
}

func verify(t *testing.T, set jwks, raw string) jwt.MapClaims {
	t.Helper()
	k := set.Keys[0]
	n, err1 := base64.RawURLEncoding.DecodeString(k.N)
	e, err2 := base64.RawURLEncoding.DecodeString(k.E)
	if err1 != nil || err2 != nil {
		t.Fatalf("JWKS n/e encoding: %v %v", err1, err2)
	}
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(raw, claims, func(tk *jwt.Token) (any, error) {
		if tk.Header["kid"] != k.Kid {
			return nil, fmt.Errorf("kid %v not in JWKS", tk.Header["kid"])
		}
		return pub, nil
	}, jwt.WithValidMethods([]string{"RS256"})); err != nil {
		t.Fatalf("token does not verify against JWKS: %v", err)
	}
	return claims
}

func TestRegisterLoginAndKeySurvivesRestart(t *testing.T) {
	dsn, pool := schemaDSN(t)
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	ctx := context.Background()

	grpcAddr, httpAddr, stop := start(t)
	conn, err := grpcx.Dial(grpcAddr)
	if err != nil {
		t.Fatal(err)
	}
	c := identityv1.NewIdentityServiceClient(conn)
	reg, err := c.Register(ctx, &identityv1.RegisterRequest{Email: "Ana@Example.com", Password: "s3cret-pw"})
	if err != nil {
		t.Fatal(err)
	}
	set := fetchJWKS(t, httpAddr)
	claims := verify(t, set, reg.GetToken().GetAccessToken())
	if claims["role"] != "customer" || claims["kid"] != set.Keys[0].Kid {
		t.Errorf("claims = %v", claims)
	}
	var hash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM account WHERE email = 'ana@example.com' AND id::text = $1`, claims["sub"]).Scan(&hash); err != nil {
		t.Fatalf("stored account: %v", err)
	}
	if !strings.HasPrefix(hash, "$2a$") {
		t.Errorf("stored password %q is not a bcrypt hash", hash)
	}
	conn.Close()
	stop()

	grpcAddr, httpAddr, stop = start(t)
	defer stop()
	conn, err = grpcx.Dial(grpcAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	again := fetchJWKS(t, httpAddr)
	if again.Keys[0].Kid != set.Keys[0].Kid {
		t.Errorf("kid changed across restart: %s → %s", set.Keys[0].Kid, again.Keys[0].Kid)
	}
	verify(t, again, reg.GetToken().GetAccessToken())
	login, err := identityv1.NewIdentityServiceClient(conn).Login(ctx, &identityv1.LoginRequest{Email: "ana@example.com", Password: "s3cret-pw"})
	if err != nil {
		t.Fatal(err)
	}
	if verify(t, again, login.GetToken().GetAccessToken())["sub"] != claims["sub"] {
		t.Error("login token is for a different Account")
	}
}

// TestLiveRegisterAndLogin runs against a deployed identity-service when
// IDENTITY_LIVE_GRPC and IDENTITY_LIVE_HTTP name its addresses (for example
// from a container on the compose network).
func TestLiveRegisterAndLogin(t *testing.T) {
	grpcAddr, httpAddr := os.Getenv("IDENTITY_LIVE_GRPC"), os.Getenv("IDENTITY_LIVE_HTTP")
	if grpcAddr == "" || httpAddr == "" {
		t.Skip("IDENTITY_LIVE_GRPC / IDENTITY_LIVE_HTTP not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := grpcx.Dial(grpcAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := identityv1.NewIdentityServiceClient(conn)
	email := fmt.Sprintf("live-%d@example.com", time.Now().UnixNano())

	reg, err := c.Register(ctx, &identityv1.RegisterRequest{Email: email, Password: "s3cret-pw"})
	if err != nil {
		t.Fatal(err)
	}
	set := fetchJWKS(t, httpAddr)
	sub := verify(t, set, reg.GetToken().GetAccessToken())["sub"]
	login, err := c.Login(ctx, &identityv1.LoginRequest{Email: email, Password: "s3cret-pw"})
	if err != nil {
		t.Fatal(err)
	}
	if verify(t, set, login.GetToken().GetAccessToken())["sub"] != sub {
		t.Error("login token is for a different Account")
	}
	if _, err := c.Register(ctx, &identityv1.RegisterRequest{Email: email, Password: "s3cret-pw"}); status.Code(err) != codes.AlreadyExists {
		t.Errorf("second Register = %v, want AlreadyExists", err)
	}
	if _, err := c.Login(ctx, &identityv1.LoginRequest{Email: email, Password: "wrong-pw-1"}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("wrong password = %v, want Unauthenticated", err)
	}
	t.Logf("kid %s, sub %v", set.Keys[0].Kid, sub)
}
