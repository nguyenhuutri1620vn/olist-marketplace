package handler_test

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/nguyenhuutri1620vn/olist-marketplace/services/api-gateway/internal/handler"
)

func TestGRPCErrorsMapToHTTP(t *testing.T) {
	for code, want := range map[codes.Code]int{
		codes.InvalidArgument:    400,
		codes.Unauthenticated:    401,
		codes.PermissionDenied:   403,
		codes.NotFound:           404,
		codes.AlreadyExists:      409,
		codes.FailedPrecondition: 409,
		codes.ResourceExhausted:  429,
		codes.Unavailable:        503,
		codes.DeadlineExceeded:   504,
		codes.Internal:           500,
		codes.Unknown:            500,
		codes.DataLoss:           500,
		codes.Unimplemented:      500,
	} {
		rec := httptest.NewRecorder()
		handler.WriteGRPCError(rec, status.Error(code, "boom"))
		if rec.Code != want {
			t.Errorf("%v → %d, want %d", code, rec.Code, want)
		}
		var body struct {
			Error struct{ Code, Message string } `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error.Code == "" || body.Error.Message == "" {
			t.Errorf("%v body = %s (%v), want {error:{code,message}}", code, rec.Body, err)
		}
	}
}

func TestErrorCodeNamesAndInternalDetailsHidden(t *testing.T) {
	rec := httptest.NewRecorder()
	handler.WriteGRPCError(rec, status.Error(codes.AlreadyExists, "email already registered"))
	if body := rec.Body.String(); body != `{"error":{"code":"ALREADY_EXISTS","message":"email already registered"}}`+"\n" {
		t.Errorf("body = %s", body)
	}
	rec = httptest.NewRecorder()
	handler.WriteGRPCError(rec, errors.New("dial tcp 10.0.0.5:9090: secret detail"))
	if rec.Code != 500 || rec.Body.String() != `{"error":{"code":"INTERNAL","message":"internal error"}}`+"\n" {
		t.Errorf("non-status error → %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	handler.WriteGRPCError(rec, status.Error(codes.Internal, "pq: relation missing"))
	if rec.Body.String() != `{"error":{"code":"INTERNAL","message":"internal error"}}`+"\n" {
		t.Errorf("INTERNAL detail leaked: %s", rec.Body)
	}
}
