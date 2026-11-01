// Package routing builds the gateway's HTTP API: public auth and browsing
// routes and the role-gated route groups (FR16).
package routing

import (
	"errors"
	"net/http"

	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/api-gateway/internal/handler"
)

// Deps are what the routes call.
type Deps struct {
	Verifier *auth.Verifier
	Identity identityv1.IdentityServiceClient
	Catalog  handler.CatalogBrowser
	Orders   handler.Checkouter
	// Upstreams are traced gRPC connections by service name (debug routes).
	Upstreams map[string]grpc.ClientConnInterface
}

// groups are the role route groups. Their handlers arrive with Epics 2–5;
// until then an authorized request gets a JSON 404 behind the gate.
var groups = map[auth.Role][]string{
	auth.RoleCustomer: {"/api/checkout", "/api/orders", "/api/orders/", "/api/vouchers/{code}"},
	auth.RoleSeller:   {"/api/seller", "/api/seller/"},
	auth.RoleAdmin:    {"/api/admin", "/api/admin/"},
}

// New returns the gateway API.
func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	handler.RegisterAuth(mux, d.Identity)
	handler.RegisterCatalog(mux, d.Catalog)
	handler.RegisterGRPCHealth(mux, d.Upstreams)
	notYet := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.WriteError(w, http.StatusNotFound, "NOT_FOUND", "no route "+r.Method+" "+r.URL.Path)
	})
	// Anything unmatched still gets the error body.
	mux.Handle("/", notYet)
	mux.Handle("POST /api/checkout", RequireRole(d.Verifier, auth.RoleCustomer)(handler.Checkout(d.Orders)))
	for role, patterns := range groups {
		for _, p := range patterns {
			mux.Handle(p, RequireRole(d.Verifier, role)(notYet))
		}
	}
	return mux
}

// RequireRole lets through requests whose bearer token verifies and carries
// one of roles, with the caller and token in the request context so upstream
// calls forward the JWT. Otherwise it answers 401 or 403 and calls nothing.
func RequireRole(v *auth.Verifier, roles ...auth.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := auth.BearerToken(r.Header.Get("Authorization"))
			if !ok {
				handler.WriteError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "missing bearer token")
				return
			}
			c, err := v.Verify(r.Context(), raw)
			if errors.Is(err, auth.ErrKeysUnavailable) {
				handler.WriteError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "token keys unavailable")
				return
			}
			if err != nil {
				handler.WriteError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "invalid or expired token")
				return
			}
			for _, role := range roles {
				if c.Role == role {
					next.ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), c, raw)))
					return
				}
			}
			handler.WriteError(w, http.StatusForbidden, "PERMISSION_DENIED", "role "+string(c.Role)+" may not use this route")
		})
	}
}
