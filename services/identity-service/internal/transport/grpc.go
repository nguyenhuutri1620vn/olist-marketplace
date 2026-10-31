// Package transport exposes identity's use cases over gRPC and serves the
// JWKS over HTTP.
package transport

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/service"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/token"
)

type identity struct {
	identityv1.UnimplementedIdentityServiceServer
	accounts *service.Accounts
}

// RegisterIdentity registers IdentityService on s.
func RegisterIdentity(s grpc.ServiceRegistrar, accounts *service.Accounts) {
	identityv1.RegisterIdentityServiceServer(s, &identity{accounts: accounts})
}

var roles = map[identityv1.Role]domain.Role{
	identityv1.Role_ROLE_UNSPECIFIED: "",
	identityv1.Role_ROLE_CUSTOMER:    domain.RoleCustomer,
	identityv1.Role_ROLE_SELLER:      domain.RoleSeller,
	identityv1.Role_ROLE_ADMIN:       domain.RoleAdmin,
}

func (i *identity) Register(ctx context.Context, req *identityv1.RegisterRequest) (*identityv1.RegisterResponse, error) {
	role, ok := roles[req.GetRole()]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown role %d", req.GetRole())
	}
	tok, err := i.accounts.Register(ctx, req.GetEmail(), req.GetPassword(), role)
	if err != nil {
		return nil, toStatus(ctx, err)
	}
	return &identityv1.RegisterResponse{Token: toProto(tok)}, nil
}

func (i *identity) Login(ctx context.Context, req *identityv1.LoginRequest) (*identityv1.LoginResponse, error) {
	tok, err := i.accounts.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, toStatus(ctx, err)
	}
	return &identityv1.LoginResponse{Token: toProto(tok)}, nil
}

func toProto(t token.Token) *identityv1.Token {
	return &identityv1.Token{AccessToken: t.AccessToken, TokenType: "Bearer", ExpiresIn: t.ExpiresIn}
}

// toStatus maps domain and context errors to gRPC codes. Unexpected errors are logged and
// returned as INTERNAL without their cause.
func toStatus(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrEmailTaken):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrBadCredentials):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	}
	slog.ErrorContext(ctx, "identity request failed", "err", err)
	return status.Error(codes.Internal, "internal error")
}
