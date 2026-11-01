// Package catalog is order-service's adapter for catalog-service: the
// Checkout snapshot over gRPC (AD-3, AD-15).
package catalog

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/order-service/internal/domain"
)

// DefaultTimeout bounds one catalog call when CATALOG_TIMEOUT is unset.
const DefaultTimeout = 2 * time.Second

// Timeout reads CATALOG_TIMEOUT (a positive Go duration, default 2s).
func Timeout() (time.Duration, error) {
	v := os.Getenv("CATALOG_TIMEOUT")
	if v == "" {
		return DefaultTimeout, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("CATALOG_TIMEOUT=%q: want a positive duration", v)
	}
	return d, nil
}

// Client snapshots Products from catalog-service.
type Client struct {
	api     catalogv1.CatalogServiceClient
	timeout time.Duration
}

// New returns a client whose calls end after timeout.
func New(api catalogv1.CatalogServiceClient, timeout time.Duration) *Client {
	return &Client{api: api, timeout: timeout}
}

// Snapshots returns the known Products among ids. catalog unreachable, busy
// or slower than the timeout is domain.ErrUnavailable; a request it rejects
// is domain.ErrInvalid; a caller that went away gets its context error; any
// other failure (a misconfigured or outdated catalog) is a plain error.
func (c *Client) Snapshots(ctx context.Context, ids []string) ([]domain.Item, error) {
	cctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	resp, err := c.api.GetProducts(cctx, &catalogv1.GetProductsRequest{ProductIds: ids})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		switch status.Code(err) {
		case codes.InvalidArgument:
			return nil, fmt.Errorf("%w: catalog: %s", domain.ErrInvalid, status.Convert(err).Message())
		case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
			return nil, fmt.Errorf("%w: catalog: %v", domain.ErrUnavailable, err)
		}
		return nil, fmt.Errorf("catalog GetProducts: %w", err)
	}
	items := make([]domain.Item, len(resp.GetProducts()))
	for i, p := range resp.GetProducts() {
		items[i] = domain.Item{
			ProductID:   p.GetProductId(),
			SellerID:    p.GetSellerId(),
			SellerState: p.GetSellerState(),
			CategoryEN:  p.GetCategoryNameEn(),
			Price:       p.GetPriceCentavos(),
			Freight:     p.GetFreightCentavos(),
		}
	}
	return items, nil
}
