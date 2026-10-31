package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/password"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/service"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/token"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/transport"
)

type memAccounts map[string]domain.Account

func (m memAccounts) CreateAccount(_ context.Context, a domain.Account) error {
	if _, ok := m[a.Email]; ok {
		return domain.ErrEmailTaken
	}
	m[a.Email] = a
	return nil
}

func (m memAccounts) AccountByEmail(_ context.Context, email string) (domain.Account, error) {
	if a, ok := m[email]; ok {
		return a, nil
	}
	return domain.Account{}, domain.ErrAccountNotFound
}

// failingStore fails every call with a non-domain error.
type failingStore struct{}

func (failingStore) CreateAccount(context.Context, domain.Account) error {
	return errors.New("db down")
}
func (failingStore) AccountByEmail(context.Context, string) (domain.Account, error) {
	return domain.Account{}, errors.New("db down")
}

func client(t *testing.T, store service.AccountStore, key token.Key) identityv1.IdentityServiceClient {
	t.Helper()
	svc, err := service.NewAccounts(store, password.Bcrypt{Cost: bcrypt.MinCost}, token.NewIssuer(key, time.Now), service.NewV7ID)
	if err != nil {
		t.Fatal(err)
	}
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	transport.RegisterIdentity(srv, svc)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return identityv1.NewIdentityServiceClient(conn)
}

func newKey(t *testing.T) token.Key {
	t.Helper()
	priv, err := token.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return token.NewKey(priv)
}

func TestRegisterThenLoginReturnTokens(t *testing.T) {
	c := client(t, memAccounts{}, newKey(t))
	ctx := context.Background()
	reg, err := c.Register(ctx, &identityv1.RegisterRequest{Email: "ana@example.com", Password: "s3cret-pw", Role: identityv1.Role_ROLE_CUSTOMER})
	if err != nil {
		t.Fatal(err)
	}
	login, err := c.Login(ctx, &identityv1.LoginRequest{Email: "ANA@example.com", Password: "s3cret-pw"})
	if err != nil {
		t.Fatal(err)
	}
	for name, tok := range map[string]*identityv1.Token{"register": reg.GetToken(), "login": login.GetToken()} {
		if tok.GetAccessToken() == "" || tok.GetTokenType() != "Bearer" || tok.GetExpiresIn() != 3600 {
			t.Errorf("%s token = %v", name, tok)
		}
	}
}

func TestStatusCodes(t *testing.T) {
	c := client(t, memAccounts{}, newKey(t))
	ctx := context.Background()
	if _, err := c.Register(ctx, &identityv1.RegisterRequest{Email: "ana@example.com", Password: "s3cret-pw"}); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		call func() error
		want codes.Code
	}{
		"duplicate email": {func() error {
			_, err := c.Register(ctx, &identityv1.RegisterRequest{Email: "Ana@Example.com", Password: "s3cret-pw"})
			return err
		}, codes.AlreadyExists},
		"seller role": {func() error {
			_, err := c.Register(ctx, &identityv1.RegisterRequest{Email: "bo@example.com", Password: "s3cret-pw", Role: identityv1.Role_ROLE_SELLER})
			return err
		}, codes.InvalidArgument},
		"admin role": {func() error {
			_, err := c.Register(ctx, &identityv1.RegisterRequest{Email: "bo@example.com", Password: "s3cret-pw", Role: identityv1.Role_ROLE_ADMIN})
			return err
		}, codes.InvalidArgument},
		"unknown role number": {func() error {
			_, err := c.Register(ctx, &identityv1.RegisterRequest{Email: "bo@example.com", Password: "s3cret-pw", Role: identityv1.Role(42)})
			return err
		}, codes.InvalidArgument},
		"short password": {func() error {
			_, err := c.Register(ctx, &identityv1.RegisterRequest{Email: "bo@example.com", Password: "short"})
			return err
		}, codes.InvalidArgument},
		"wrong password": {func() error {
			_, err := c.Login(ctx, &identityv1.LoginRequest{Email: "ana@example.com", Password: "wrong-pw-1"})
			return err
		}, codes.Unauthenticated},
		"unknown email": {func() error {
			_, err := c.Login(ctx, &identityv1.LoginRequest{Email: "nobody@example.com", Password: "s3cret-pw"})
			return err
		}, codes.Unauthenticated},
	} {
		if got := status.Code(tc.call()); got != tc.want {
			t.Errorf("%s: code = %v, want %v", name, got, tc.want)
		}
	}
}

func TestLoginErrorsDoNotRevealTheEmail(t *testing.T) {
	c := client(t, memAccounts{}, newKey(t))
	ctx := context.Background()
	c.Register(ctx, &identityv1.RegisterRequest{Email: "ana@example.com", Password: "s3cret-pw"})
	_, wrong := c.Login(ctx, &identityv1.LoginRequest{Email: "ana@example.com", Password: "wrong-pw-1"})
	_, unknown := c.Login(ctx, &identityv1.LoginRequest{Email: "nobody@example.com", Password: "wrong-pw-1"})
	if status.Convert(wrong).Message() != status.Convert(unknown).Message() {
		t.Errorf("messages differ: %q vs %q", status.Convert(wrong).Message(), status.Convert(unknown).Message())
	}
}

func TestStoreFailureIsInternalWithoutDetails(t *testing.T) {
	c := client(t, failingStore{}, newKey(t))
	_, err := c.Login(context.Background(), &identityv1.LoginRequest{Email: "ana@example.com", Password: "s3cret-pw"})
	if s := status.Convert(err); s.Code() != codes.Internal || s.Message() == "db down" {
		t.Errorf("status = %v %q, want Internal without the cause", s.Code(), s.Message())
	}
}

func TestJWKSHandler(t *testing.T) {
	key := newKey(t)
	h, err := transport.JWKSHandler(key)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /.well-known/jwks.json", h)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status %d content-type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var set struct {
		Keys []struct{ Kid string } `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &set); err != nil || len(set.Keys) != 1 || set.Keys[0].Kid != key.ID {
		t.Errorf("JWKS = %s, err %v; want one key with kid %s", rec.Body, err, key.ID)
	}
}

// ctxStore fails with the context's error, as pgx does on cancel or timeout.
type ctxStore struct{ err error }

func (c ctxStore) CreateAccount(context.Context, domain.Account) error { return c.err }
func (c ctxStore) AccountByEmail(context.Context, string) (domain.Account, error) {
	return domain.Account{}, c.err
}

func TestContextErrorsKeepTheirCode(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want codes.Code
	}{
		{context.Canceled, codes.Canceled},
		{fmt.Errorf("find account: %w", context.DeadlineExceeded), codes.DeadlineExceeded},
	} {
		c := client(t, ctxStore{tc.err}, newKey(t))
		_, err := c.Login(context.Background(), &identityv1.LoginRequest{Email: "ana@example.com", Password: "s3cret-pw"})
		if got := status.Code(err); got != tc.want {
			t.Errorf("store error %v: code = %v, want %v", tc.err, got, tc.want)
		}
	}
}

type memSeeds struct{ got []domain.Seed }

func (m *memSeeds) UpsertAccounts(_ context.Context, seeds []domain.Seed, _ string) (int, error) {
	m.got = seeds
	return len(seeds), nil
}

func adminClient(t *testing.T, store service.SeedStore) identityv1.IdentityAdminServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	transport.RegisterAdmin(srv, service.NewSeeder(store, password.Bcrypt{Cost: bcrypt.MinCost}))
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return identityv1.NewIdentityAdminServiceClient(conn)
}

func TestImportAccountsMapsRequest(t *testing.T) {
	store := &memSeeds{}
	resp, err := adminClient(t, store).ImportAccounts(context.Background(), &identityv1.ImportAccountsRequest{
		Password: "olist-demo-2026",
		Accounts: []*identityv1.ImportAccount{
			{AccountId: "6560211a19b47992c3666cc44a7e94c0", Email: "seller-6560211a@demo.olist.local", Role: identityv1.Role_ROLE_SELLER},
			{AccountId: "0192f000-0000-7000-8000-00000000ad01", Email: "admin@demo.olist.local", Role: identityv1.Role_ROLE_ADMIN},
		},
	})
	if err != nil || resp.GetUpserted() != 2 {
		t.Fatalf("ImportAccounts = %v, %v", resp, err)
	}
	if store.got[0].Role != domain.RoleSeller || store.got[0].ID != "6560211a-19b4-7992-c366-6cc44a7e94c0" || store.got[1].Role != domain.RoleAdmin {
		t.Errorf("seeds = %+v", store.got)
	}
}

func TestImportAccountsRejectsInvalidBatch(t *testing.T) {
	c := adminClient(t, &memSeeds{})
	for name, req := range map[string]*identityv1.ImportAccountsRequest{
		"outside domain": {Password: "olist-demo-2026", Accounts: []*identityv1.ImportAccount{{AccountId: "6560211a19b47992c3666cc44a7e94c0", Email: "x@example.com", Role: identityv1.Role_ROLE_SELLER}}},
		"no role":        {Password: "olist-demo-2026", Accounts: []*identityv1.ImportAccount{{AccountId: "6560211a19b47992c3666cc44a7e94c0", Email: "x@demo.olist.local"}}},
		"bad id":         {Password: "olist-demo-2026", Accounts: []*identityv1.ImportAccount{{AccountId: "x", Email: "x@demo.olist.local", Role: identityv1.Role_ROLE_SELLER}}},
		"short password": {Password: "short", Accounts: []*identityv1.ImportAccount{{AccountId: "6560211a19b47992c3666cc44a7e94c0", Email: "x@demo.olist.local", Role: identityv1.Role_ROLE_SELLER}}},
	} {
		if _, err := c.ImportAccounts(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: err = %v, want InvalidArgument", name, err)
		}
	}
}
