// Package handler holds api-gateway HTTP handlers.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// RegisterGRPCHealth adds GET /debug/grpc-health/{service}, which calls the
// named service's grpc.health.v1 check over its traced client connection, so
// one request shows a gateway span and a child service span in Jaeger.
func RegisterGRPCHealth(mux *http.ServeMux, conns map[string]grpc.ClientConnInterface) {
	mux.HandleFunc("GET /debug/grpc-health/{service}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("service")
		conn, ok := conns[name]
		if !ok {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown service "+name)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			writeError(w, http.StatusBadGateway, "UNAVAILABLE", err.Error())
			return
		}
		code := http.StatusOK
		if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, map[string]string{"service": name, "status": resp.GetStatus().String()})
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
