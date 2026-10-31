package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
)

// maxAuthBody bounds login and register bodies.
const maxAuthBody = 4 << 10

// authTimeout bounds the identity call behind login and register.
const authTimeout = 5 * time.Second

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// RegisterAuth adds the public POST /api/auth/register and /api/auth/login.
// Registration always asks identity for a Customer Account (FR14).
func RegisterAuth(mux *http.ServeMux, identity identityv1.IdentityServiceClient) {
	mux.HandleFunc("POST /api/auth/register", func(w http.ResponseWriter, r *http.Request) {
		c, ok := readCredentials(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), authTimeout)
		defer cancel()
		resp, err := identity.Register(ctx, &identityv1.RegisterRequest{
			Email: c.Email, Password: c.Password, Role: identityv1.Role_ROLE_CUSTOMER,
		})
		if err != nil {
			WriteGRPCError(w, err)
			return
		}
		writeToken(w, resp.GetToken())
	})
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		c, ok := readCredentials(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), authTimeout)
		defer cancel()
		resp, err := identity.Login(ctx, &identityv1.LoginRequest{Email: c.Email, Password: c.Password})
		if err != nil {
			WriteGRPCError(w, err)
			return
		}
		writeToken(w, resp.GetToken())
	})
}

func readCredentials(w http.ResponseWriter, r *http.Request) (credentials, bool) {
	var c credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthBody)).Decode(&c); err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "body must be JSON {email, password}")
		return c, false
	}
	return c, true
}

func writeToken(w http.ResponseWriter, t *identityv1.Token) {
	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: t.GetAccessToken(), TokenType: t.GetTokenType(), ExpiresIn: t.GetExpiresIn()})
}
