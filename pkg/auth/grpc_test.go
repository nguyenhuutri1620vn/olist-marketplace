package auth_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
)

// shop stands for the three role groups: one method per group, plus a public
// method and one the policy does not declare.
const (
	checkout   = "/test.Shop/Checkout"
	sellerList = "/test.Shop/SellerOrders"
	adminList  = "/test.Shop/AdminVouchers"
	public     = "/test.Shop/Browse"
	undeclared = "/test.Shop/Undeclared"
)

var policy = auth.Policy{
	Public: []string{public},
	Roles: map[string][]auth.Role{
		checkout:   {auth.RoleCustomer},
		sellerList: {auth.RoleSeller},
		adminList:  {auth.RoleAdmin},
	},
}

// recorder keeps the claims and incoming token each handler saw.
type recorder struct {
	mu     sync.Mutex
	claims map[string]auth.Claims
	tokens map[string]string
}

func shopDesc(rec *recorder) *grpc.ServiceDesc {
	desc := &grpc.ServiceDesc{ServiceName: "test.Shop", HandlerType: (*any)(nil)}
	for _, m := range []string{"Checkout", "SellerOrders", "AdminVouchers", "Browse", "Undeclared"} {
		name := "/test.Shop/" + m
		desc.Methods = append(desc.Methods, grpc.MethodDesc{
			MethodName: m,
			Handler: func(_ any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
				in := &emptypb.Empty{}
				if err := dec(in); err != nil {
					return nil, err
				}
				h := func(ctx context.Context, _ any) (any, error) {
					rec.mu.Lock()
					defer rec.mu.Unlock()
					if c, ok := auth.FromContext(ctx); ok {
						rec.claims[name] = c
					}
					if md, ok := metadata.FromIncomingContext(ctx); ok && len(md.Get("authorization")) > 0 {
						rec.tokens[name] = md.Get("authorization")[0]
					}
					return &emptypb.Empty{}, nil
				}
				return ic(ctx, in, &grpc.UnaryServerInfo{FullMethod: name}, h)
			},
		})
	}
	return desc
}

func shopConn(t *testing.T, v *auth.Verifier) (*grpc.ClientConn, *recorder) {
	t.Helper()
	rec := &recorder{claims: map[string]auth.Claims{}, tokens: map[string]string{}}
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(policy.UnaryInterceptor(v)))
	srv.RegisterService(shopDesc(rec), struct{}{})
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(auth.ForwardToken()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, rec
}

func call(conn *grpc.ClientConn, ctx context.Context, method string) error {
	return conn.Invoke(ctx, method, &emptypb.Empty{}, &emptypb.Empty{})
}

// asCaller returns a context carrying a token for role, as the gateway's
// middleware sets it up before calling a service.
func asCaller(t *testing.T, s signer, role auth.Role) context.Context {
	t.Helper()
	raw := s.token(t, "acc-"+string(role), string(role), time.Now(), time.Hour)
	return auth.WithClaims(context.Background(), auth.Claims{AccountID: "acc-" + string(role), Role: role}, raw)
}

func TestServiceRoleTable(t *testing.T) {
	s := newSigner(t, "k1")
	conn, _ := shopConn(t, auth.NewVerifier(auth.NewKeySet(newJWKSServer(t, s).URL)))
	groups := map[auth.Role]string{auth.RoleCustomer: checkout, auth.RoleSeller: sellerList, auth.RoleAdmin: adminList}
	for _, role := range []auth.Role{auth.RoleCustomer, auth.RoleSeller, auth.RoleAdmin} {
		for owner, method := range groups {
			want := codes.PermissionDenied
			if owner == role {
				want = codes.OK
			}
			if got := status.Code(call(conn, asCaller(t, s, role), method)); got != want {
				t.Errorf("%s calling %s: %v, want %v", role, method, got, want)
			}
		}
	}
}

func TestServiceRejectsMissingOrInvalidTokens(t *testing.T) {
	s, other := newSigner(t, "k1"), newSigner(t, "k1")
	conn, _ := shopConn(t, auth.NewVerifier(auth.NewKeySet(newJWKSServer(t, s).URL)))
	bad := auth.WithClaims(context.Background(), auth.Claims{}, other.token(t, "x", "admin", time.Now(), time.Hour))
	for name, ctx := range map[string]context.Context{"no token": context.Background(), "bad signature": bad} {
		for _, m := range []string{checkout, undeclared} {
			if got := status.Code(call(conn, ctx, m)); got != codes.Unauthenticated {
				t.Errorf("%s on %s: %v, want Unauthenticated", name, m, got)
			}
		}
	}
	if got := status.Code(call(conn, context.Background(), public)); got != codes.OK {
		t.Errorf("public method without token: %v, want OK", got)
	}
}

func TestUndeclaredMethodFailsClosed(t *testing.T) {
	s := newSigner(t, "k1")
	conn, _ := shopConn(t, auth.NewVerifier(auth.NewKeySet(newJWKSServer(t, s).URL)))
	if got := status.Code(call(conn, asCaller(t, s, auth.RoleAdmin), undeclared)); got != codes.PermissionDenied {
		t.Errorf("admin on undeclared method: %v, want PermissionDenied", got)
	}
}

func TestTokenIsForwardedAndClaimsReachTheHandler(t *testing.T) {
	s := newSigner(t, "k1")
	conn, rec := shopConn(t, auth.NewVerifier(auth.NewKeySet(newJWKSServer(t, s).URL)))
	ctx := asCaller(t, s, auth.RoleCustomer)
	raw, _ := auth.TokenFromContext(ctx)
	if err := call(conn, ctx, checkout); err != nil {
		t.Fatal(err)
	}
	if rec.tokens[checkout] != "Bearer "+raw {
		t.Errorf("service saw authorization %q, want the original JWT", rec.tokens[checkout])
	}
	if rec.claims[checkout] != (auth.Claims{AccountID: "acc-customer", Role: auth.RoleCustomer}) {
		t.Errorf("handler claims = %+v", rec.claims[checkout])
	}
}

func TestHealthIsAlwaysPublic(t *testing.T) {
	if !(auth.Policy{}).IsPublic("/grpc.health.v1.Health/Check") {
		t.Error("health check needs a token")
	}
}

func TestServiceReportsKeyOutageAsUnavailable(t *testing.T) {
	s := newSigner(t, "k1")
	j := newJWKSServer(t, s)
	j.down.Store(true)
	conn, _ := shopConn(t, auth.NewVerifier(auth.NewKeySet(j.URL)))
	if got := status.Code(call(conn, asCaller(t, s, auth.RoleCustomer), checkout)); got != codes.Unavailable {
		t.Errorf("identity down, nothing cached: %v, want Unavailable", got)
	}
}

func TestBearerToken(t *testing.T) {
	for header, want := range map[string]string{
		"Bearer abc":   "abc",
		"bearer abc":   "abc",
		"BEARER  abc ": "abc",
		"Basic abc":    "",
		"Bearer":       "",
		"Bearer ":      "",
		"":             "",
	} {
		if got, _ := auth.BearerToken(header); got != want {
			t.Errorf("BearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestStreamInterceptorEnforcesThePolicy(t *testing.T) {
	s := newSigner(t, "k1")
	v := auth.NewVerifier(auth.NewKeySet(newJWKSServer(t, s).URL))
	raw := s.token(t, "acc-1", "seller", time.Now(), time.Hour)
	for name, tc := range map[string]struct {
		md   metadata.MD
		want codes.Code
	}{
		"own role":   {metadata.Pairs("authorization", "Bearer "+raw), codes.OK},
		"no token":   {metadata.MD{}, codes.Unauthenticated},
		"other role": {metadata.Pairs("authorization", "Bearer "+s.token(t, "c", "customer", time.Now(), time.Hour)), codes.PermissionDenied},
	} {
		var seen auth.Claims
		stream := &fakeStream{ctx: metadata.NewIncomingContext(context.Background(), tc.md)}
		err := policy.StreamInterceptor(v)(nil, stream, &grpc.StreamServerInfo{FullMethod: sellerList},
			func(_ any, ss grpc.ServerStream) error {
				seen, _ = auth.FromContext(ss.Context())
				return nil
			})
		if status.Code(err) != tc.want {
			t.Errorf("%s: %v, want %v", name, status.Code(err), tc.want)
		}
		if tc.want == codes.OK && seen.AccountID != "acc-1" {
			t.Errorf("%s: handler stream ctx claims = %+v", name, seen)
		}
	}
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeStream) Context() context.Context { return f.ctx }
