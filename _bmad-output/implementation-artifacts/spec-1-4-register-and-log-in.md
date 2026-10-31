---
title: 'Story 1.4: Register and log in'
type: 'feature'
created: '2026-10-31'
status: 'done'
baseline_commit: '2d2b9b95c714e6b4cef126fb99b66ad390133769'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** identity-service has no Accounts, no signing key and no RPCs, so nobody can register or log in (FR14, NFR7).

**Approach:** identity-service owns an Account table and a persisted RS256 signing key, serves gRPC `Register` and `Login` that both return the same 1-hour JWT token response, and serves `GET /.well-known/jwks.json` on its internal HTTP port for later token verification.

## Boundaries & Constraints

**Always:** bcrypt (golang.org/x/crypto) for passwords, never stored or logged in clear; golang-jwt v5, RS256, claims `sub`, `role`, `kid` (also `iat`, `exp`), `kid` in the JWT header too; token lifetime exactly 1 h, no refresh token; new Account ids are UUIDv7; emails stored trimmed and lower-cased under a unique index; role values `customer`, `seller`, `admin`. Test first for every behavior.

**Never:** publish identity's HTTP or gRPC port; let registration create a Seller or Admin; reveal whether an email exists on failed login; add gateway routes or token verification (Story 1.6); seed Accounts (Story 1.5).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Register | new `Ana@Example.com ` + 8+ char password | Customer Account, email `ana@example.com`, UUIDv7 id; token response | N/A |
| Duplicate | `ANA@example.com` after the above | nothing created | `ALREADY_EXISTS` |
| Bad email | empty, `not-an-email`, `"Ana" <a@b.c>`, `x@demo.olist.local` | nothing created | `INVALID_ARGUMENT` |
| Bad password | 7 characters, or 73 bytes | nothing created | `INVALID_ARGUMENT` |
| Privileged role | role `seller` or `admin` | nothing created | `INVALID_ARGUMENT` |
| Login | correct email (any case) and password | JWT RS256, `exp = iat + 3600`, header and claim `kid` match JWKS | N/A |
| Wrong login | wrong password, or unknown email | identical error, similar timing | `UNAUTHENTICATED` |
| JWKS | `GET /.well-known/jwks.json` | `{"keys":[{kty RSA, alg RS256, use sig, kid, n, e}]}` that verifies issued tokens | N/A |
| Restart | identity-service restarts | same key and `kid`; earlier tokens still verify | N/A |

</frozen-after-approval>

## Code Map

- `proto/identity/v1/identity.proto` -- empty `IdentityService`; add `Register` and `Login` (buf STANDARD: `RegisterResponse` and `LoginResponse` each wrap one shared `Token` message). Regenerate with `make proto-generate`.
- `services/identity-service/cmd/server/main.go` -- `app.Main(app.Service{... GRPC: true})`.
- `pkg/app/app.go` -- has `RegisterGRPC(s, pool)` and `API func() (http.Handler, func(), error)`; gRPC is registered before the API is built. Add an `Init func(ctx, pool) error` hook, run once after the pool opens and before either, so identity loads its key once and shares it with both.
- `services/catalog-service/internal/*` -- reference for the hexagonal split, `classify` (pg error → `domain.ErrInvalid`), transport `toStatus`, and repository tests on a per-test schema (`DBMIGRATE_TEST_DSN`).
- `services/identity-service/migrations/` -- embedded FS, no SQL yet.
- `deployments/docker-compose.yml` -- identity has no `HTTP_ADDR`; app defaults it to `:8080` (internal, not published).
- `tools/olist-importer` -- unaffected (identity import is Story 1.5).

## Tasks & Acceptance

**Execution:**
- [x] `services/identity-service/migrations/000001_identity.{up,down}.sql` -- `account(id uuid PK, email text, password_hash text, role text CHECK in (customer, seller, admin), created_at timestamptz default now())` with unique index on `email`; `signing_key(kid PK, private_key bytea, created_at)`.
- [x] `proto/identity/v1/identity.proto` -- `Register(email, password, role)`, `Login(email, password)`, `Role` enum, `Token{access_token, token_type="Bearer", expires_in=3600}`.
- [x] `services/identity-service/internal/domain` -- email normalisation and validation (net/mail, no display name, reserved domain), password length rules, roles.
- [x] `services/identity-service/internal/service` -- `Register` / `Login` use cases over ports (account store, hasher, token issuer, clock); dummy bcrypt compare for unknown emails.
- [x] `services/identity-service/internal/repository` -- account insert (unique violation → `ErrEmailTaken`), lookup by email; signing key load-or-create under an advisory lock.
- [x] `services/identity-service/internal/token` (or `service`) -- RS256 signer, RFC 7638 thumbprint `kid`, JWKS document.
- [x] `services/identity-service/internal/transport` -- gRPC handlers with status mapping; HTTP handler for `/.well-known/jwks.json`.
- [x] `pkg/app` + identity `main.go` -- `Init` hook; identity wires key, repository, service, gRPC and API.
- [x] Tests -- unit tests for the I/O matrix; Postgres-backed repository and key-persistence tests; a gRPC round trip (Register → Login → verify with JWKS).

**Acceptance Criteria:**
- Given `make up`, when identity-service is healthy, then `GET http://identity-service:8080/.well-known/jwks.json` from inside the compose network returns one RS256 key.
- Given the running stack, when a client on the compose network registers and logs in, then both tokens verify against that JWKS, and the database holds only a bcrypt hash.
- Given identity-service restarts, then the JWKS `kid` is unchanged.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD, as in Stories 1.1–1.3.
- Packages: `internal/domain` (email/password/role rules), `internal/token` (RS256 issuer, RFC 7638 `kid`, JWKS), `internal/password` (bcrypt, default cost 10), `internal/service` (`Accounts.Register/Login`, dummy-hash compare for unknown emails), `internal/repository` (accounts; `SigningKey` load-or-create under `pg_advisory_xact_lock`), `internal/transport` (gRPC + JWKS handler).
- `pkg/app.Service.Init(ctx, pool)` runs once after migrations and before `RegisterGRPC`/`API`; identity loads its key there and both handlers share it.
- The account table also checks `email = lower(btrim(email))`, so any writer must store the normalized email.
- `cmd/server/main_test.go` runs the whole service in-process on Postgres: Register → JWKS verify → stored bcrypt hash → restart → same `kid`, old token still verifies, Login works. `TestLiveRegisterAndLogin` runs the same flow against a deployed service when `IDENTITY_LIVE_GRPC`/`IDENTITY_LIVE_HTTP` are set.
- Mutation checks: removing the advisory lock fails the concurrent-startup test; generating a fresh key at each start fails the restart test.
- Live check: `make up` healthy; JWKS from a `golang:1.27.3-alpine` container on `olist` returns one RS256 key; the live test (cross-compiled test binary copied into that container) passed Register, Login, ALREADY_EXISTS and UNAUTHENTICATED; after `docker compose restart identity-service` the JWKS is byte-identical; the DB holds a `$2a$10$` hash and one signing key.

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | edge | Login with a >72-byte password whose first 72 bytes match succeeds | high | Probe: bcrypt `Compare` of a 73-byte password against the 72-byte hash returned nil; Login now rejects >72 bytes after a dummy compare (test failed first) | patch |
| 2 | blind, edge | Unbounded email length surfaces as INTERNAL (btree limit) | medium | No length check; 254-byte RFC 5321 limit added (test failed first) | patch |
| 3 | edge | Subdomains of the reserved demo domain are registrable | low | `x@sub.demo.olist.local` passed; suffix check added (test failed first) | patch |
| 4 | blind, edge | Client cancel/timeout reported as INTERNAL and logged as error | low | Fell to default branch; mapped to CANCELED / DEADLINE_EXCEEDED (test failed first) | patch |
| 5 | blind, edge, vgap | Migration comment promises all keys in JWKS; only the newest is loaded | low | Comment corrected; rotation deferred | patch / defer |
| 6 | vgap, blind | Deployed-stack JWKS and Register/Login never checked in CI | medium | `TestLiveRegisterAndLogin` skipped everywhere; smoke now fetches JWKS in the network and runs the live test binary | patch |
| 7 | blind | Private key stored unencrypted | medium | Documented local-demo trade-off (Design Notes); hardening deferred with rotation | defer |
| 8 | blind | Register's ALREADY_EXISTS reveals that an email exists | low | Required by the story's acceptance criteria | reject |
| 9 | blind | No brute-force protection on Login | medium | Not in this story's intent | defer |
| 10 | blind | Go and Postgres email normalisation differ for exotic Unicode | low | Go output is already lower-case and trimmed, so `lower(btrim())` leaves it unchanged in practice; adds rules for unseen input | reject |
| 11 | blind | No `iss`/`aud` claims | low | AD-14 fixes the claim set (`sub`, `role`, `kid`); one issuer | reject |
| 12 | blind, vgap | Test robustness: `hash[:4]` panic, unchecked base64, unchecked JWKS status, unbounded channel reads in Init test | low | Direct fixes in tests | patch |
| 13 | edge | `start()` hides an early Run error and leaks on `t.Fatal` | low | Now fails fast with the cause and stops via `t.Cleanup` | patch |
| 14 | blind | Diff omits generated code, go.sum, spec | false | Deliberately excluded; `make proto` regenerates cleanly | reject |
| 15 | blind | Down migration lacks `IF EXISTS`; dummy compare result silently discarded | low | Down runs once; compare now `_ =` with a comment | patch (compare) / reject (down) |
| 16 | blind | JWKS "internal only" not enforced by a test | low | Scaffold test `TestComposePublishesOnlyGatewayAndObservabilityPorts` already fails if identity publishes a port | false |
| 17 | edge | Spec mentions a clock port on the service; clock lives in the issuer | low | Fix is a spec edit | reject |

## Design Notes

- The key lives in identity's own database so restarts and future replicas share it; the private key is stored unencrypted, acceptable for the local demo only.
- JWKS is on identity's API port (`:8080`, internal); the gateway and services will fetch it there in Story 1.6.
- Live checks run from a throwaway `golang:1.27.3-alpine` container on the `olist` network, since no identity port is published.

## Verification

**Commands:**
- `make build && make test && make proto` -- expected: exit 0, DB tests run with `DBMIGRATE_TEST_DSN`.
- `make up` -- expected: exit 0, identity healthy.
- `docker run --rm --network olist golang:1.27.3-alpine wget -qO- http://identity-service:8080/.well-known/jwks.json` -- expected: one RS256 key; same `kid` after `docker compose restart identity-service`.
