package auth

import (
	"context"
	"errors"
	"slices"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Policy says who may call each RPC (full method name, e.g.
// "/order.v1.OrderService/Checkout"). It fails closed: a non-public RPC needs
// a valid token (else UNAUTHENTICATED), and an RPC missing from Roles is
// PERMISSION_DENIED for every authenticated caller.
type Policy struct {
	// Public RPCs need no token; a valid token still puts claims in ctx.
	Public []string
	// Roles lists the roles allowed to call each non-public RPC.
	Roles map[string][]Role
}

const healthPrefix = "/grpc.health.v1.Health/"

// IsPublic reports whether method needs no token. Health checks are public.
func (p Policy) IsPublic(method string) bool {
	return strings.HasPrefix(method, healthPrefix) || slices.Contains(p.Public, method)
}

// authorize verifies the caller's metadata token for method and returns ctx
// with the claims.
func (p Policy) authorize(ctx context.Context, v *Verifier, method string) (context.Context, error) {
	raw := bearer(ctx)
	if p.IsPublic(method) {
		if raw != "" && v != nil {
			if c, err := v.Verify(ctx, raw); err == nil {
				ctx = WithClaims(ctx, c, raw)
			}
		}
		return ctx, nil
	}
	if raw == "" || v == nil {
		return nil, status.Error(codes.Unauthenticated, "missing token")
	}
	c, err := v.Verify(ctx, raw)
	if errors.Is(err, ErrKeysUnavailable) {
		return nil, status.Error(codes.Unavailable, "token keys unavailable")
	}
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
	}
	if !slices.Contains(p.Roles[method], c.Role) {
		return nil, status.Errorf(codes.PermissionDenied, "role %s may not call %s", c.Role, method)
	}
	return WithClaims(ctx, c, raw), nil
}

func bearer(ctx context.Context) string {
	md, _ := metadata.FromIncomingContext(ctx)
	for _, h := range md.Get("authorization") {
		if t, ok := BearerToken(h); ok {
			return t
		}
	}
	return ""
}

// BearerToken extracts the token from an Authorization header value. The
// scheme is case-insensitive (RFC 7235).
func BearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

// UnaryInterceptor enforces p on unary RPCs.
func (p Policy) UnaryInterceptor(v *Verifier) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		ctx, err := p.authorize(ctx, v, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return h(ctx, req)
	}
}

// StreamInterceptor enforces p on streaming RPCs.
func (p Policy) StreamInterceptor(v *Verifier) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		ctx, err := p.authorize(ss.Context(), v, info.FullMethod)
		if err != nil {
			return err
		}
		return h(srv, &ctxStream{ServerStream: ss, ctx: ctx})
	}
}

type ctxStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *ctxStream) Context() context.Context { return s.ctx }

// ForwardToken sends the caller's JWT from ctx (see WithClaims) as
// "authorization: Bearer <jwt>" metadata, so services re-verify it.
func ForwardToken() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if raw, ok := TokenFromContext(ctx); ok {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+raw)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
