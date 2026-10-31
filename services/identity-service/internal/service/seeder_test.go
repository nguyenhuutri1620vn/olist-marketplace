package service_test

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/service"
)

type recordingSeeds struct {
	calls int
	seeds []domain.Seed
	hash  string
}

func (r *recordingSeeds) UpsertAccounts(_ context.Context, seeds []domain.Seed, hash string) (int, error) {
	r.calls++
	r.seeds, r.hash = seeds, hash
	return len(seeds), nil
}

func seed(t *testing.T) domain.Seed {
	t.Helper()
	s, err := domain.NewSeed("8d50f5eadf50201ccdcedfb9e2ac8455", "customer-8d50f5ea@demo.olist.local", domain.RoleCustomer)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestImportAccountsHashesThePasswordOnce(t *testing.T) {
	store, hasher := &recordingSeeds{}, &countingHasher{}
	n, err := service.NewSeeder(store, hasher).ImportAccounts(context.Background(), []domain.Seed{seed(t), seed(t)}, "olist-demo-2026")
	if err != nil || n != 2 {
		t.Fatalf("ImportAccounts = %d, %v", n, err)
	}
	if hasher.hashes != 1 {
		t.Errorf("hashed %d times, want once per batch", hasher.hashes)
	}
	if bcrypt.CompareHashAndPassword([]byte(store.hash), []byte("olist-demo-2026")) != nil {
		t.Errorf("stored hash %q does not match the demo password", store.hash)
	}
}

func TestImportAccountsRejectsBadPasswordBeforeStoring(t *testing.T) {
	store := &recordingSeeds{}
	_, err := service.NewSeeder(store, &countingHasher{}).ImportAccounts(context.Background(), []domain.Seed{seed(t)}, "short")
	if !errors.Is(err, domain.ErrInvalid) || store.calls != 0 {
		t.Errorf("err = %v, store calls = %d; want ErrInvalid and no store call", err, store.calls)
	}
}

func TestImportAccountsEmptyBatchIsANoOp(t *testing.T) {
	store := &recordingSeeds{}
	if n, err := service.NewSeeder(store, &countingHasher{}).ImportAccounts(context.Background(), nil, "olist-demo-2026"); err != nil || n != 0 || store.calls != 0 {
		t.Errorf("empty batch: %d, %v, store calls %d", n, err, store.calls)
	}
}
