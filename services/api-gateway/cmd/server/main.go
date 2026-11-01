package main

import (
	"net/http"
	"os"

	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/auth"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
	catalogv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/catalog/v1"
	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	orderv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/order/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/api-gateway/internal/routing"
)

// upstreams are the services the gateway calls over gRPC, on <name>:9090.
var upstreams = []string{"identity-service", "catalog-service", "order-service", "payment-service", "analytics-service"}

func main() {
	app.Main(app.Service{Name: "api-gateway", API: api})
}

func api() (http.Handler, func(), error) {
	conns := make(map[string]grpc.ClientConnInterface, len(upstreams))
	var opened []*grpc.ClientConn
	closeAll := func() {
		for _, c := range opened {
			c.Close()
		}
	}
	for _, name := range upstreams {
		// Every upstream call carries the caller's JWT, which the service re-verifies.
		conn, err := grpcx.Dial(name+":9090", grpc.WithChainUnaryInterceptor(auth.ForwardToken()))
		if err != nil {
			closeAll()
			return nil, nil, err
		}
		opened = append(opened, conn)
		conns[name] = conn
	}
	jwks := os.Getenv("JWKS_URL")
	if jwks == "" {
		jwks = app.DefaultJWKSURL
	}
	return routing.New(routing.Deps{
		Verifier:  auth.NewVerifier(auth.NewKeySet(jwks)),
		Identity:  identityv1.NewIdentityServiceClient(conns["identity-service"]),
		Catalog:   catalogv1.NewCatalogServiceClient(conns["catalog-service"]),
		Orders:    orderv1.NewOrderServiceClient(conns["order-service"]),
		Upstreams: conns,
	}), closeAll, nil
}
