package handler

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// httpErrors maps gRPC codes to HTTP status and the body's error code; any
// other code is a 500 (Conventions).
var httpErrors = map[codes.Code]struct {
	status int
	name   string
}{
	codes.InvalidArgument:    {http.StatusBadRequest, "INVALID_ARGUMENT"},
	codes.Unauthenticated:    {http.StatusUnauthorized, "UNAUTHENTICATED"},
	codes.PermissionDenied:   {http.StatusForbidden, "PERMISSION_DENIED"},
	codes.NotFound:           {http.StatusNotFound, "NOT_FOUND"},
	codes.AlreadyExists:      {http.StatusConflict, "ALREADY_EXISTS"},
	codes.FailedPrecondition: {http.StatusConflict, "FAILED_PRECONDITION"},
	codes.ResourceExhausted:  {http.StatusTooManyRequests, "RESOURCE_EXHAUSTED"},
	codes.Unavailable:        {http.StatusServiceUnavailable, "UNAVAILABLE"},
	codes.DeadlineExceeded:   {http.StatusGatewayTimeout, "DEADLINE_EXCEEDED"},
}

// WriteGRPCError writes a service error as `{"error":{"code","message"}}`.
// Client errors keep the service's message; server-side ones get a generic
// message so internal details stay inside.
func WriteGRPCError(w http.ResponseWriter, err error) {
	s, _ := status.FromError(err)
	e, ok := httpErrors[s.Code()]
	switch {
	case !ok:
		WriteError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
	case e.status >= 500:
		WriteError(w, e.status, e.name, http.StatusText(e.status))
	default:
		WriteError(w, e.status, e.name, s.Message())
	}
}

// WriteError writes `{"error":{"code","message"}}` with status.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
