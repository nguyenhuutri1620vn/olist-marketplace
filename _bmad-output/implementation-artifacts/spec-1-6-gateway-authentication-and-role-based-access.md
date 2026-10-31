---
title: 'Story 1.6: Gateway authentication and role-based access'
type: 'feature'
created: '2026-10-31'
status: 'done'
baseline_commit: 'f9413d275cbed57c9e672249d465f730ffe3d3da'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Tokens from Story 1.4 are verified nowhere: the gateway has no auth, no login/register routes and no role groups, and services accept any caller (FR16, NFR7).

**Approach:** A shared `pkg/auth` verifies RS256 tokens against identity's cached JWKS, exposes the caller's id and role, and enforces a per-RPC role policy in a gRPC interceptor every service installs. The gateway adds `POST /api/auth/register|login`, guards the three role route groups, forwards the JWT in gRPC metadata, and maps gRPC errors to HTTP with the `{ "error": … }` body.

## Boundaries & Constraints

**Always:** route groups Customer (`/api/checkout`, `/api/orders/*`, `/api/vouchers/{code}`), Seller (`/api/seller/*`), Admin (`/api/admin/*`); missing/invalid/expired token → 401, wrong role → 403 without calling a service; services fail closed: an RPC absent from the policy is `PERMISSION_DENIED`, no token on a non-public RPC is `UNAUTHENTICATED`; JWKS from `JWKS_URL` (default `http://identity-service:8080/.well-known/jwks.json`), refetched on an unknown `kid` at most once per minute, last good keys kept when identity is unreachable; gRPC→HTTP: INVALID_ARGUMENT 400, UNAUTHENTICATED 401, PERMISSION_DENIED 403, NOT_FOUND 404, ALREADY_EXISTS/FAILED_PRECONDITION 409, RESOURCE_EXHAUSTED 429, UNAVAILABLE 503, DEADLINE_EXCEEDED 504, anything else 500. Test first for every behavior.

**Never:** business routes or RPCs for the groups (Epics 2–5 add them); route the admin import RPCs through the gateway; publish service ports. Decision: `Register`, `Login`, health and the admin import RPCs (`ImportCategories/Sellers/Products`, `ImportAccounts`) are public at the service; the import RPCs stay reachable only on the internal network (their auth is already deferred), since the importer has no token before the Admin exists.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Login/register | `POST /api/auth/login` `{email,password}` | 200 `{access_token, token_type, expires_in}` | identity codes mapped (401, 409, 400) |
| No token | non-public route, no or malformed `Authorization` | 401 `{"error":{"code":"UNAUTHENTICATED",…}}` | no service call |
| Expired / bad signature | token past `exp`, or signed by another key | 401 | no service call |
| Wrong role | each role × each other role's group (6 cases) | 403 `PERMISSION_DENIED` | no service call |
| Own role | each role × its own group | passes the gate (404 until routes exist) | N/A |
| Service gate | same 3×3 table on a gRPC method per group | allowed / `PERMISSION_DENIED`; no token → `UNAUTHENTICATED` | N/A |
| Forwarding | authorized gateway call | service sees the same JWT and reads id + role from ctx | N/A |
| Unknown kid | token with a kid not in cache | one refetch; still unknown → `UNAUTHENTICATED`; second unknown kid within 1 min → no refetch | N/A |
| JWKS down | identity unreachable after keys were cached | cached keys keep verifying | N/A |
| Error map | each listed gRPC code, plus INTERNAL/UNKNOWN | listed HTTP status, else 500, same body | N/A |

</frozen-after-approval>

## Code Map

- `pkg/grpcx/grpcx.go` -- `NewServer(reg, opts...)`, `Dial(target, opts...)`: pass the auth server interceptor and the token-forwarding client interceptor as options.
- `pkg/app/app.go` -- `Service` (`GRPC`, `RegisterGRPC`, `Init`, `API`); add `Policy auth.Policy` used to build the interceptor for the gRPC server (verifier built lazily from `JWKS_URL`).
- `services/identity-service/internal/token` -- issuer/JWKS format to verify against (claims `sub`, `role`, `kid`; header `kid`); reuse in tests via `token.NewIssuer`/`token.JWKS`.
- `services/*/cmd/server/main.go` -- add each service's policy: identity (Register, Login, ImportAccounts public), catalog (Import* public), order/payment/analytics (empty, fail closed).
- `services/api-gateway/cmd/server/main.go`, `internal/handler/grpchealth.go` (has `writeError`/`writeJSON`), empty `internal/middleware` and `internal/routing` dirs.
- `deployments/docker-compose.yml` -- gateway already `depends_on` identity healthy.

## Tasks & Acceptance

**Execution:**
- [x] `pkg/auth` -- `KeySet` (JWKS cache, 1/min refresh limit, keep last good), `Verifier` (RS256, exp, role in customer|seller|admin), `Claims` in ctx (`FromContext`), `Policy` + `UnaryServerInterceptor`/`StreamServerInterceptor`, `ForwardToken` client interceptor, HTTP `Authenticate`/`RequireRole` middleware.
- [x] `pkg/app` -- `Policy` field; install interceptors on the gRPC server.
- [x] `services/*/cmd/server/main.go` -- declare policies.
- [x] `services/api-gateway/internal/{routing,middleware,handler}` -- auth routes to identity, role groups, error mapping, JWT forwarding on upstream conns.
- [x] Tests -- 3×3 role tables at gateway and service, verifier and key-set edge cases, error mapping table, forwarding round trip, register/login through the gateway.

**Acceptance Criteria:**
- Given `make up` and the demo Accounts, when the Customer, Seller and Admin log in through `POST 127.0.0.1:8080/api/auth/login`, then each token gets 403 on the other two groups, passes its own, and no token gets 401.
- Given `POST /api/auth/register` with a new email, then 200 with a token; the same email again gives 409 with the error body.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD, as in Stories 1.1–1.5.
- `pkg/auth`: `KeySet` (lazy JWKS fetch; unknown kid refetches at most once per minute once keys are cached, once per second while none ever loaded; a failed fetch keeps the old keys), `Verifier` (RS256, `exp` required, subject and role checked), `WithClaims`/`FromContext`/`TokenFromContext`, `Policy` with unary and stream server interceptors (health always public, undeclared RPC → PERMISSION_DENIED, no/invalid token → UNAUTHENTICATED), `ForwardToken` client interceptor. `pkg/auth/authtest` signs test tokens and serves their JWKS.
- The HTTP role middleware lives in the gateway (`routing.RequireRole`) rather than `pkg/auth`, since only the gateway serves HTTP to users.
- `pkg/app.Service.Policy`: every gRPC server gets the policy interceptors with a verifier on `JWKS_URL` (default `app.DefaultJWKSURL`). identity and catalog declare their public RPCs; order, payment and analytics declare nothing yet, so all their non-health RPCs fail closed.
- Gateway: `routing.New` (auth routes, debug route, role groups behind `RequireRole`), `handler.WriteGRPCError` (5xx bodies carry a generic message), upstream conns forward the JWT.
- Live check: after `make up` and `make import`, the three demo Accounts logged in through `POST 127.0.0.1:8080/api/auth/login`; the 3×3 table gave 404 on the own group and 403 on the others, no token gave 401; register gave 200 then 409 for the same email in upper case; bad JSON gave 400. `make import` still works with the import RPCs declared public.

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge | KeySet holds its mutex during the HTTP fetch, stalling cached-kid lookups | medium | New test blocked >1 s; fetch now runs outside the lock, other unknown-kid callers wait on it | patch |
| 2 | blind, edge | Fetch uses the caller's ctx after `lastFetch` is set; one canceled request blocks refresh for a minute | medium | Test failed first; fetch now uses a detached ctx with its own 5 s timeout | patch |
| 3 | blind, edge | Identity outage reported as 401; "could not be verified" branch dead | medium | All errors wrapped ErrUnauthenticated; new `ErrKeysUnavailable` → gRPC UNAVAILABLE / HTTP 503 (tests failed first) | patch |
| 4 | edge | One malformed JWKS entry rejects the whole set; bad `e` not range-checked | low | Bad entries now skipped with a warning | patch |
| 5 | edge | Lowercase `bearer` scheme rejected | low | RFC 7235 case-insensitive; shared `auth.BearerToken` (test failed first) | patch |
| 6 | edge | Token without `kid` spends the refetch budget | low | Now rejected before lookup (test failed first) | patch |
| 7 | blind, edge | Unknown paths get plain-text 404; `/api/seller`, `/api/admin` redirect | low | Catch-all JSON 404 and slashless group roots added (test failed first) | patch |
| 8 | blind | `TestAuthRoutesArePublic` hit a 405 and never exercised the route | low | Now POSTs with a fake identity and expects 200 | patch |
| 9 | blind | Stream interceptor untested | low | Table test on `StreamInterceptor` added | patch |
| 10 | vgap | Catalog's public import RPCs not tested against its real policy | medium | `catalogService()` extracted; test checks all three are public | patch |
| 11 | blind, edge, vgap | Policy doc says undeclared RPCs are PERMISSION_DENIED for every caller | low | Doc now states UNAUTHENTICATED without a valid token | patch |
| 12 | blind | Auth routes have no timeout; health 502 leaks dial details | low | 5 s timeout added; health error message made generic | patch |
| 13 | blind | CI token step hides a bad response as `null` | low | `jq -er` fails the step on a missing token | patch |
| 14 | blind | No `iss`/`aud`/leeway checks | low | AD-14 fixes the claim set; one issuer on one host | reject |
| 15 | blind | Import RPCs protected only by the network | medium | Same boundary as already deferred (Stories 1.3, 1.5) | defer (existing) |
| 16 | blind | No login rate limit | medium | Already deferred from Story 1.4 | defer (existing) |
| 17 | blind | CI does not cover Seller/Admin tokens or service re-verification end to end | low | No dataset in CI; service re-verification is covered by the bufconn table; live run covered all roles | reject |
| 18 | blind, edge | Canceled/Aborted/Unimplemented map to 500 | low | Story fixes the table; others are 500 by its own rule | reject |
| 19 | edge | No stream client forwarding; duplicate `authorization` possible | low | Gateway makes no streaming calls and sets the header once | reject |
| 20 | edge | Nil token response or oversized body handling | low | Identity always returns a token; oversized bodies get 400 | reject |
| 21 | edge | HTTP middleware lives in the gateway, not `pkg/auth`; services with zero policy refuse all RPCs | low | Recorded in Implementation Notes; fail-closed is the intent | reject |
| 22 | blind | `JWKS_URL` default read in two places | low | Both use `app.DefaultJWKSURL` | reject |

## Design Notes

- Own-role requests pass the gate and hit a JSON 404 until Epics 2–5 add handlers; that proves the gate without fake routes.
- The verifier fetches the JWKS lazily on first use, so services start even before identity is up.

## Verification

**Commands:**
- `make build && make test && make proto` -- expected: exit 0, DB tests run.
- `make up && make import` -- expected: exit 0.
- curl the gateway with each demo role's token against `/api/checkout`, `/api/seller/orders`, `/api/admin/vouchers` -- expected: the 3×3 table above.
