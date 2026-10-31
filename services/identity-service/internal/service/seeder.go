package service

import (
	"context"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
)

// SeedStore upserts seeded Accounts, applying a batch whole or not at all.
type SeedStore interface {
	UpsertAccounts(ctx context.Context, seeds []domain.Seed, hash string) (int, error)
}

// Seeder imports demo Accounts that share one password (FR15).
type Seeder struct {
	store  SeedStore
	hasher Hasher
}

// NewSeeder wires the import use case.
func NewSeeder(store SeedStore, hasher Hasher) *Seeder {
	return &Seeder{store: store, hasher: hasher}
}

// ImportAccounts upserts seeds with password, hashed once for the batch. An
// empty batch is a no-op.
func (s *Seeder) ImportAccounts(ctx context.Context, seeds []domain.Seed, password string) (int, error) {
	if err := domain.ValidatePassword(password); err != nil {
		return 0, err
	}
	if len(seeds) == 0 {
		return 0, nil
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return 0, err
	}
	return s.store.UpsertAccounts(ctx, seeds, hash)
}
