package domain_test

import (
	"errors"
	"math"
	"testing"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
)

func TestNewPageChecksBounds(t *testing.T) {
	p, err := domain.NewPage(3, 20)
	if err != nil || p.Limit() != 20 || p.Offset() != 40 {
		t.Fatalf("page = %+v, %v", p, err)
	}
	if p, err := domain.NewPage(math.MaxInt32, domain.MaxPageSize); err != nil || p.Offset() != int64(math.MaxInt32-1)*domain.MaxPageSize {
		t.Errorf("largest page: %+v, %v", p, err)
	}
	for _, tc := range []struct{ page, size int32 }{{0, 20}, {1, 0}, {1, domain.MaxPageSize + 1}, {-1, 5}} {
		if _, err := domain.NewPage(tc.page, tc.size); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("page %d size %d: %v, want ErrInvalid", tc.page, tc.size, err)
		}
	}
}

func TestParseOrderID(t *testing.T) {
	if id, err := domain.ParseOrderID("0192F000-0000-7000-8000-0000000000AA"); err != nil || id != "0192f000-0000-7000-8000-0000000000aa" {
		t.Errorf("ParseOrderID = %q, %v", id, err)
	}
	if _, err := domain.ParseOrderID("abc"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("abc: %v, want ErrInvalid", err)
	}
}
