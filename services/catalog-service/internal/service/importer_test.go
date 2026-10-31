package service_test

import (
	"context"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/domain"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/catalog-service/internal/service"
)

// countingRepo counts calls; an empty batch must not reach the database.
type countingRepo struct{ calls int }

func (r *countingRepo) UpsertCategories(context.Context, []domain.Category) (int, error) {
	r.calls++
	return 0, nil
}
func (r *countingRepo) UpsertSellers(context.Context, []domain.Seller) (int, error) {
	r.calls++
	return 0, nil
}
func (r *countingRepo) UpsertProducts(context.Context, []domain.Product) (int, error) {
	r.calls++
	return 0, nil
}

func TestEmptyBatchesAreNoOps(t *testing.T) {
	repo := &countingRepo{}
	imp := service.NewImporter(repo)
	ctx := context.Background()
	imp.ImportCategories(ctx, nil)
	imp.ImportSellers(ctx, nil)
	imp.ImportProducts(ctx, nil)
	if repo.calls != 0 {
		t.Errorf("repository called %d times for empty batches, want 0", repo.calls)
	}
}
