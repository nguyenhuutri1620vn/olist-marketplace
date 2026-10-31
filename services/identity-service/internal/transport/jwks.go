package transport

import (
	"net/http"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/identity-service/internal/token"
)

// JWKSHandler serves the public signing keys at /.well-known/jwks.json.
func JWKSHandler(keys ...token.Key) (http.Handler, error) {
	doc, err := token.JWKS(keys...)
	if err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(doc)
	}), nil
}
