package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/nguyenhuutri1620vn/olist-marketplace/proto/gen/go/identity/v1"
	"github.com/nguyenhuutri1620vn/olist-marketplace/services/api-gateway/internal/handler"
)

// fakeIdentity records requests; err, when set, is returned by every call.
type fakeIdentity struct {
	reg   *identityv1.RegisterRequest
	login *identityv1.LoginRequest
	err   error
}

var tok = &identityv1.Token{AccessToken: "jwt", TokenType: "Bearer", ExpiresIn: 3600}

func (f *fakeIdentity) Register(_ context.Context, r *identityv1.RegisterRequest, _ ...grpc.CallOption) (*identityv1.RegisterResponse, error) {
	f.reg = r
	if f.err != nil {
		return nil, f.err
	}
	return &identityv1.RegisterResponse{Token: tok}, nil
}

func (f *fakeIdentity) Login(_ context.Context, r *identityv1.LoginRequest, _ ...grpc.CallOption) (*identityv1.LoginResponse, error) {
	f.login = r
	if f.err != nil {
		return nil, f.err
	}
	return &identityv1.LoginResponse{Token: tok}, nil
}

func serveAuth(f *fakeIdentity, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	handler.RegisterAuth(mux, f)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestLoginAndRegisterReturnTheToken(t *testing.T) {
	for _, path := range []string{"/api/auth/login", "/api/auth/register"} {
		f := &fakeIdentity{}
		rec := serveAuth(f, http.MethodPost, path, `{"email":"ana@example.com","password":"s3cret-pw"}`)
		if rec.Code != 200 || rec.Body.String() != `{"access_token":"jwt","token_type":"Bearer","expires_in":3600}`+"\n" {
			t.Errorf("%s → %d %s", path, rec.Code, rec.Body)
		}
	}
	f := &fakeIdentity{}
	serveAuth(f, http.MethodPost, "/api/auth/register", `{"email":"ana@example.com","password":"s3cret-pw","role":"admin"}`)
	if f.reg.GetEmail() != "ana@example.com" || f.reg.GetPassword() != "s3cret-pw" || f.reg.GetRole() != identityv1.Role_ROLE_CUSTOMER {
		t.Errorf("register forwarded %v, want a Customer registration whatever the body says", f.reg)
	}
}

func TestAuthErrorsUseTheErrorBody(t *testing.T) {
	for name, tc := range map[string]struct {
		f    *fakeIdentity
		path string
		body string
		want int
	}{
		"bad json":         {&fakeIdentity{}, "/api/auth/login", `{"email":`, 400},
		"wrong password":   {&fakeIdentity{err: status.Error(codes.Unauthenticated, "invalid email or password")}, "/api/auth/login", `{"email":"a@b.co","password":"x"}`, 401},
		"duplicate email":  {&fakeIdentity{err: status.Error(codes.AlreadyExists, "email already registered")}, "/api/auth/register", `{"email":"a@b.co","password":"s3cret-pw"}`, 409},
		"invalid register": {&fakeIdentity{err: status.Error(codes.InvalidArgument, "too short")}, "/api/auth/register", `{"email":"a@b.co","password":"x"}`, 400},
	} {
		rec := serveAuth(tc.f, http.MethodPost, tc.path, tc.body)
		if rec.Code != tc.want || !strings.HasPrefix(rec.Body.String(), `{"error":{"code":`) {
			t.Errorf("%s → %d %s, want %d with the error body", name, rec.Code, rec.Body, tc.want)
		}
	}
	if rec := serveAuth(&fakeIdentity{}, http.MethodGet, "/api/auth/login", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET login → %d, want 405", rec.Code)
	}
}
