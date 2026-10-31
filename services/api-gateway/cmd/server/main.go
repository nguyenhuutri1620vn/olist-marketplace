package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	// Routes arrive with Story 1.2 (ops endpoints) and Story 1.6 (REST API).
	srv := &http.Server{Addr: addr, Handler: http.NewServeMux(), ReadHeaderTimeout: 5 * time.Second}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("api-gateway: shutdown: %v", err)
		}
	}()

	log.Printf("api-gateway: listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("api-gateway: %v", err)
	}
	<-shutdownDone
	log.Println("api-gateway: shut down")
}
