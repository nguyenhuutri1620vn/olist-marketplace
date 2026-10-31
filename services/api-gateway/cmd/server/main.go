package main

import (
	"net/http"

	"google.golang.org/grpc"

	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/app"
	"github.com/nguyenhuutri1620vn/olist-marketplace/pkg/grpcx"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/api-gateway/internal/handler"
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
		conn, err := grpcx.Dial(name + ":9090")
		if err != nil {
			closeAll()
			return nil, nil, err
		}
		opened = append(opened, conn)
		conns[name] = conn
	}
	mux := http.NewServeMux()
	handler.RegisterGRPCHealth(mux, conns)
	return mux, closeAll, nil
}
