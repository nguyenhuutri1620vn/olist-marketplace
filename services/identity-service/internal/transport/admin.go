package transport

import (
	"context"

	"google.golang.org/grpc"

	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/service"
)

// admin serves IdentityAdminService (FR15, AD-12).
type admin struct {
	identityv1.UnimplementedIdentityAdminServiceServer
	seeder *service.Seeder
}

// RegisterAdmin registers IdentityAdminService on s.
func RegisterAdmin(s grpc.ServiceRegistrar, seeder *service.Seeder) {
	identityv1.RegisterIdentityAdminServiceServer(s, &admin{seeder: seeder})
}

func (a *admin) ImportAccounts(ctx context.Context, req *identityv1.ImportAccountsRequest) (*identityv1.ImportAccountsResponse, error) {
	seeds := make([]domain.Seed, 0, len(req.GetAccounts()))
	for _, acc := range req.GetAccounts() {
		// An unspecified or unknown role maps to "" and fails validation.
		seed, err := domain.NewSeed(acc.GetAccountId(), acc.GetEmail(), roles[acc.GetRole()])
		if err != nil {
			return nil, toStatus(ctx, err)
		}
		seeds = append(seeds, seed)
	}
	n, err := a.seeder.ImportAccounts(ctx, seeds, req.GetPassword())
	if err != nil {
		return nil, toStatus(ctx, err)
	}
	return &identityv1.ImportAccountsResponse{Upserted: int32(n)}, nil
}
