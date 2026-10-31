package main

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/password"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/repository"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/service"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/token"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/transport"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/migrations"
)

func main() {
	app.Main(identityService())
}

// identityService wires the key, Accounts, gRPC and the JWKS endpoint.
func identityService() app.Service {
	var (
		key      token.Key
		accounts *service.Accounts
		seeder   *service.Seeder
	)
	return app.Service{
		Name:       "identity-service",
		Migrations: migrations.FS,
		GRPC:       true,
		// Login and Register are how a caller gets a token. ImportAccounts is
		// reachable only on the internal network (the gateway never routes it):
		// the importer has no token before the Admin exists.
		Policy: auth.Policy{Public: []string{
			identityv1.IdentityService_Register_FullMethodName,
			identityv1.IdentityService_Login_FullMethodName,
			identityv1.IdentityAdminService_ImportAccounts_FullMethodName,
		}},
		Init: func(ctx context.Context, pool *pgxpool.Pool) error {
			repo := repository.New(pool)
			var err error
			if key, err = repo.SigningKey(ctx, token.GenerateKey); err != nil {
				return err
			}
			seeder = service.NewSeeder(repo, password.Bcrypt{})
			accounts, err = service.NewAccounts(repo, password.Bcrypt{}, token.NewIssuer(key, time.Now), service.NewV7ID)
			return err
		},
		RegisterGRPC: func(s *grpc.Server, _ *pgxpool.Pool) {
			transport.RegisterIdentity(s, accounts)
			transport.RegisterAdmin(s, seeder)
		},
		// Internal only (not published): the gateway and services fetch the
		// JWKS here to verify tokens.
		API: func() (http.Handler, func(), error) {
			jwks, err := transport.JWKSHandler(key)
			if err != nil {
				return nil, nil, err
			}
			mux := http.NewServeMux()
			mux.Handle("GET /.well-known/jwks.json", jwks)
			return mux, func() {}, nil
		},
	}
}
