package routing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth/authtest"
	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/api-gateway/internal/routing"
)

// groupPaths has one request path per role's route group.
var groupPaths = map[auth.Role][]string{
	// GET /api/orders and /api/orders/{id} are real routes now (TestOrderReadsAreCustomerOnly).
	auth.RoleCustomer: {"/api/checkout", "/api/vouchers/WELCOME10"},
	auth.RoleSeller:   {"/api/seller/orders", "/api/seller/revenue"},
	auth.RoleAdmin:    {"/api/admin/vouchers", "/api/admin/dashboard"},
}

func do(h http.Handler, path, authz string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRoleGroupTable(t *testing.T) {
	iss := authtest.NewIssuer(t)
	h := routing.New(routing.Deps{Verifier: iss.Verifier()})
	for _, caller := range []auth.Role{auth.RoleCustomer, auth.RoleSeller, auth.RoleAdmin} {
		bearer := "Bearer " + iss.Token(t, "acc-"+string(caller), caller)
		for owner, paths := range groupPaths {
			for _, p := range paths {
				rec := do(h, p, bearer)
				if owner == caller {
					// Through the gate; no handler exists yet for the group.
					if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"NOT_FOUND"`) {
						t.Errorf("%s on own %s → %d %s, want the JSON 404 behind the gate", caller, p, rec.Code, rec.Body)
					}
				} else if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"PERMISSION_DENIED"`) {
					t.Errorf("%s on %s's %s → %d %s, want 403", caller, owner, p, rec.Code, rec.Body)
				}
			}
		}
	}
}

func TestGroupsNeedAValidToken(t *testing.T) {
	iss, stranger := authtest.NewIssuer(t), authtest.NewIssuer(t)
	h := routing.New(routing.Deps{Verifier: iss.Verifier()})
	expired := iss.TokenAt(t, "a", auth.RoleCustomer, time.Now().Add(-2*time.Hour), time.Hour)
	for name, authz := range map[string]string{
		"none":         "",
		"not bearer":   "Basic YTpi",
		"empty bearer": "Bearer ",
		"garbage":      "Bearer not.a.jwt",
		"expired":      "Bearer " + expired,
		"other issuer": "Bearer " + stranger.Token(t, "a", auth.RoleCustomer),
	} {
		for _, paths := range groupPaths {
			rec := do(h, paths[0], authz)
			if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), `{"error":{"code":"UNAUTHENTICATED"`) {
				t.Errorf("%s on %s → %d %s, want 401 with the error body", name, paths[0], rec.Code, rec.Body)
			}
		}
	}
}

func TestRequireRolePutsCallerInContext(t *testing.T) {
	iss := authtest.NewIssuer(t)
	raw := iss.Token(t, "seller-1", auth.RoleSeller)
	var got auth.Claims
	var gotRaw string
	h := routing.RequireRole(iss.Verifier(), auth.RoleSeller)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = auth.FromContext(r.Context())
		gotRaw, _ = auth.TokenFromContext(r.Context())
	}))
	do(h, "/api/seller/orders", "Bearer "+raw)
	if got != (auth.Claims{AccountID: "seller-1", Role: auth.RoleSeller}) || gotRaw != raw {
		t.Errorf("context claims %+v token match %v", got, gotRaw == raw)
	}
}

type okIdentity struct{}

func (okIdentity) Register(context.Context, *identityv1.RegisterRequest, ...grpc.CallOption) (*identityv1.RegisterResponse, error) {
	return &identityv1.RegisterResponse{Token: &identityv1.Token{AccessToken: "t"}}, nil
}

func (okIdentity) Login(context.Context, *identityv1.LoginRequest, ...grpc.CallOption) (*identityv1.LoginResponse, error) {
	return &identityv1.LoginResponse{Token: &identityv1.Token{AccessToken: "t"}}, nil
}

func TestAuthRoutesArePublic(t *testing.T) {
	h := routing.New(routing.Deps{Verifier: authtest.NewIssuer(t).Verifier(), Identity: okIdentity{}})
	for _, p := range []string{"/api/auth/login", "/api/auth/register"} {
		req := httptest.NewRequest(http.MethodPost, p, strings.NewReader(`{"email":"a@b.co","password":"s3cret-pw"}`))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("POST %s without a token → %d, want 200", p, rec.Code)
		}
	}
}

func TestLowercaseBearerSchemeIsAccepted(t *testing.T) {
	iss := authtest.NewIssuer(t)
	h := routing.New(routing.Deps{Verifier: iss.Verifier()})
	if rec := do(h, "/api/admin/vouchers", "bearer "+iss.Token(t, "a", auth.RoleAdmin)); rec.Code != http.StatusNotFound {
		t.Errorf("lowercase bearer → %d, want the own-group 404", rec.Code)
	}
}

func TestIdentityOutageIs503(t *testing.T) {
	iss := authtest.NewIssuer(t)
	tok := iss.Token(t, "a", auth.RoleCustomer)
	// A verifier whose JWKS endpoint is down and never loaded.
	h := routing.New(routing.Deps{Verifier: auth.NewVerifier(auth.NewKeySet("http://127.0.0.1:1/jwks"))})
	rec := do(h, "/api/checkout", "Bearer "+tok)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"UNAVAILABLE"`) {
		t.Errorf("identity down → %d %s, want 503 UNAVAILABLE", rec.Code, rec.Body)
	}
}

func TestUnknownPathsGetTheErrorBody(t *testing.T) {
	iss := authtest.NewIssuer(t)
	h := routing.New(routing.Deps{Verifier: iss.Verifier()})
	for _, p := range []string{"/api/nope", "/", "/api/vouchers"} {
		rec := do(h, p, "")
		if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Body.String(), `{"error":{"code":"NOT_FOUND"`) {
			t.Errorf("%s → %d %s, want JSON 404", p, rec.Code, rec.Body)
		}
	}
	// Group roots without a trailing slash are gated, not redirected.
	for _, p := range []string{"/api/seller", "/api/admin"} {
		if rec := do(h, p, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without token → %d, want 401", p, rec.Code)
		}
	}
}

// okCatalog answers every browsing call with an empty result.
type okCatalog struct{}

func (okCatalog) ListCategories(context.Context, *catalogv1.ListCategoriesRequest, ...grpc.CallOption) (*catalogv1.ListCategoriesResponse, error) {
	return &catalogv1.ListCategoriesResponse{}, nil
}

func (okCatalog) ListProducts(context.Context, *catalogv1.ListProductsRequest, ...grpc.CallOption) (*catalogv1.ListProductsResponse, error) {
	return &catalogv1.ListProductsResponse{}, nil
}

// Browsing is public (NFR7): no token, or a stale one sent by a logged-in
// browser, still lists the catalog.
func TestBrowsingRoutesArePublic(t *testing.T) {
	iss := authtest.NewIssuer(t)
	h := routing.New(routing.Deps{Verifier: iss.Verifier(), Catalog: okCatalog{}})
	expired := iss.TokenAt(t, "a", auth.RoleCustomer, time.Now().Add(-2*time.Hour), time.Hour)
	for _, p := range []string{"/api/categories", "/api/products", "/api/products?category=Uncategorized&page=2"} {
		for _, authz := range []string{"", "Bearer " + expired} {
			if rec := do(h, p, authz); rec.Code != http.StatusOK {
				t.Errorf("GET %s with %q → %d %s, want 200", p, authz, rec.Code, rec.Body)
			}
		}
	}
}

type okOrders struct{}

func (okOrders) Checkout(context.Context, *orderv1.CheckoutRequest, ...grpc.CallOption) (*orderv1.CheckoutResponse, error) {
	return &orderv1.CheckoutResponse{CheckoutId: "k"}, nil
}

// Only Customers reach order-service's Checkout.
func TestCheckoutIsCustomerOnly(t *testing.T) {
	iss := authtest.NewIssuer(t)
	h := routing.New(routing.Deps{Verifier: iss.Verifier(), Orders: okOrders{}})
	post := func(authz string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/checkout", strings.NewReader(`{"product_ids":["p"],"checkout_idempotency_key":"k"}`))
		if authz != "" {
			req.Header.Set("Authorization", authz)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := post(""); got != http.StatusUnauthorized {
		t.Errorf("no token → %d", got)
	}
	for role, want := range map[auth.Role]int{auth.RoleCustomer: 200, auth.RoleSeller: 403, auth.RoleAdmin: 403} {
		if got := post("Bearer " + iss.Token(t, "acc", role)); got != want {
			t.Errorf("%s → %d, want %d", role, got, want)
		}
	}
}

func (okOrders) ListMyOrders(context.Context, *orderv1.ListMyOrdersRequest, ...grpc.CallOption) (*orderv1.ListMyOrdersResponse, error) {
	return &orderv1.ListMyOrdersResponse{}, nil
}

func (okOrders) GetOrder(context.Context, *orderv1.GetOrderRequest, ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	return &orderv1.GetOrderResponse{Order: &orderv1.OrderDetail{OrderId: "o"}}, nil
}

// Only Customers read Orders through the gateway.
func TestOrderReadsAreCustomerOnly(t *testing.T) {
	iss := authtest.NewIssuer(t)
	h := routing.New(routing.Deps{Verifier: iss.Verifier(), Orders: okOrders{}})
	for _, p := range []string{"/api/orders", "/api/orders/0192f000-0000-7000-8000-0000000000aa"} {
		if rec := do(h, p, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s no token → %d", p, rec.Code)
		}
		for role, want := range map[auth.Role]int{auth.RoleCustomer: 200, auth.RoleSeller: 403, auth.RoleAdmin: 403} {
			if rec := do(h, p, "Bearer "+iss.Token(t, "acc", role)); rec.Code != want {
				t.Errorf("%s as %s → %d, want %d", p, role, rec.Code, want)
			}
		}
	}
}

// Other methods on /api/orders stay gated: wrong role 403, Customer the
// JSON 404 of a route that does not exist yet.
func TestOtherOrderMethodsStayGated(t *testing.T) {
	iss := authtest.NewIssuer(t)
	h := routing.New(routing.Deps{Verifier: iss.Verifier(), Orders: okOrders{}})
	for _, tc := range []struct {
		method, path string
	}{{http.MethodPost, "/api/orders"}, {http.MethodDelete, "/api/orders/0192f000-0000-7000-8000-0000000000aa"}} {
		for role, want := range map[auth.Role]int{auth.RoleCustomer: 404, auth.RoleSeller: 403} {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+iss.Token(t, "acc", role))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != want {
				t.Errorf("%s %s as %s → %d, want %d", tc.method, tc.path, role, rec.Code, want)
			}
		}
	}
}
