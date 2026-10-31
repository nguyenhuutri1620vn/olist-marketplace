---
title: 'Story 1.5: Seed demo Accounts'
type: 'feature'
created: '2026-10-31'
status: 'done'
baseline_commit: '4992ad7ca4d641f0c6da1d13f4e8152bb214083b'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Only self-registered Customers exist, so Seller and Admin flows cannot be demoed and no Account has dataset history (FR15).

**Approach:** identity-service exposes admin gRPC `ImportAccounts` (idempotent upsert keyed by Account id). `make import` also picks 20 dataset Customers and 20 dataset Sellers deterministically, adds one Admin with a fixed id from `contracts`, and sends them with one shared demo password; the README lists every demo login.

## Boundaries & Constraints

**Always:** Customers are the 20 `customer_unique_id`s with the most Orders (`ORDER BY order_count DESC, customer_unique_id ASC`); Sellers are the 20 `seller_id`s with the most Order Items (`ORDER BY item_count DESC, seller_id ASC`); emails `customer-<first 8 of id>@demo.olist.local`, `seller-<first 8 of id>@demo.olist.local`, `admin@demo.olist.local`; Account id = dataset id (uuid) and the Admin id is a fixed UUID constant in `contracts`; demo password from the importer's `DEMO_PASSWORD` env (documented default `olist-demo-2026`), validated by Story 1.4's password rules and stored only as a bcrypt hash; the importer still calls only admin gRPC. Test first for every behavior.

**Never:** SQL from the importer; seed Accounts outside `@demo.olist.local`; publish identity's ports; build the web Login hint (it arrives with the web Login page in Story 1.7).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| First import | empty identity DB | 20 customer, 20 seller, 1 admin Accounts; each logs in with the demo password | N/A |
| Re-import | same CSVs and password | still 41 seeded Accounts, exactly one Admin, same ids and emails | N/A |
| Password changed | `DEMO_PASSWORD` differs on re-import | every seeded Account logs in with the new password only | N/A |
| Ties | equal order or item counts | lower id wins, same pick on every run | N/A |
| Registered Customer | self-registered Accounts exist | untouched by import | N/A |
| Bad batch | email outside the reserved domain, bad id, unknown role, short password | whole batch rejected | `INVALID_ARGUMENT`, importer exits 1 |

</frozen-after-approval>

## Code Map

- `proto/identity/v1/identity.proto` -- add `IdentityAdminService.ImportAccounts` in a new `identity_admin.proto` (mirrors `catalog_admin.proto`).
- `contracts/` -- module with only `events/v1/doc.go`; add `contracts/accounts/accounts.go` with `AdminAccountID`, `AdminEmail`, `DemoDomain`. Add require/replace in the importer go.mod.
- `services/identity-service/internal/domain/account.go` -- `NewRegistration`, `NormalizeEmail`, `ReservedDomain`, password limits, `Role`; add a `NewSeed` constructor reusing the same rules.
- `services/identity-service/internal/repository/postgres.go` -- `CreateAccount`, `AccountByEmail`; add `UpsertAccounts` (one statement, `ON CONFLICT (id) DO UPDATE`, unique-email clash → `ErrInvalid`), following catalog's `classify`.
- `services/identity-service/internal/{service,transport}` -- `Accounts` use cases with `Hasher`; add the import use case and an admin gRPC handler.
- `services/identity-service/cmd/server/main.go` -- `identityService()` wiring and the in-process main test.
- `tools/olist-importer/internal/{olist,importer}`, `main.go` -- CSV loading, catalog batching; add demo-Account selection (needs `olist_customers_dataset.csv`) and the identity call (`IDENTITY_ADDR`, default `identity-service:9090`).
- `Makefile` (`OLIST_FILES`, `import`), `deployments/docker-compose.yml` (importer env and `depends_on`), `tests/scaffold` (`OLIST_FILES` list), `README.md`.

## Tasks & Acceptance

**Execution:**
- [x] `contracts/accounts/accounts.go` -- Admin id, Admin email, demo domain constants.
- [x] `proto/identity/v1/identity_admin.proto` -- `ImportAccounts(accounts[{account_id, email, role}], password) → upserted`.
- [x] `services/identity-service/internal/domain` -- seeded Account validation (uuid id, reserved-domain email, role, password rules).
- [x] `services/identity-service/internal/repository` -- `UpsertAccounts` in one transaction.
- [x] `services/identity-service/internal/service` + `transport` + `cmd/server/main.go` -- import use case (hash once per call), admin handler, wiring.
- [x] `tools/olist-importer` -- `olist.DemoAccounts(dir)` selection, `importer.ImportAccounts`, `DEMO_PASSWORD` / `IDENTITY_ADDR` in `main.go`.
- [x] `Makefile`, `deployments/docker-compose.yml`, `tests/scaffold` -- customers CSV copied; importer waits for identity healthy; `DEMO_PASSWORD` passed through.
- [x] `README.md` -- demo password and all 41 demo emails per role.
- [x] Tests -- unit tests for the I/O matrix; Postgres-backed upsert tests; real-dataset selection test (`OLIST_DATA_DIR`).

**Acceptance Criteria:**
- Given `make up` then `make import` twice, when Accounts are counted, then there are 41 Accounts in `@demo.olist.local` and exactly one Admin.
- Given the seeded Accounts, when one Customer, one Seller and the Admin log in with the demo password through identity's gRPC, then each receives a token with the matching `role` and `sub` = its dataset id (or the Admin constant).
- Given the README, then it lists every seeded email per role and the demo password.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD, as in Stories 1.1–1.4.
- `contracts/accounts`: `DemoDomain`, `AdminAccountID` (`0192f000-0000-7000-8000-00000000ad01`), `AdminEmail`; identity's `ReservedDomain` now comes from it.
- identity: `domain.NewSeed` (uuid.Parse accepts 32-hex dataset ids, stores canonical form; email must be exactly in the reserved domain; role required) and `ValidatePassword` shared with registration; `repository.UpsertAccounts` (one `unnest` statement, `ON CONFLICT (id) DO UPDATE`, unique-email clash or repeated id → `ErrInvalid`); `service.Seeder` hashes once per batch; `IdentityAdminService` in `identity_admin.proto`.
- Importer: `olist.DemoAccounts` / `TopIDs` (count DESC, id ASC) reading the customers CSV, `importer.ImportAccounts` (one 41-row call with the batch timeout); `main.go` reads both selections before any RPC.
- Live check: two `make import` runs each reported 41 demo Accounts; identity holds admin 1, customer 20, seller 20 in the reserved domain. The live test binary logged in a Customer, a Seller and the Admin with the demo password (correct `sub`/`role`); after `DEMO_PASSWORD=changed-pass-42 make import` the new password worked and the old one returned UNAUTHENTICATED; the default was restored. The seeded emails in the DB matched an independent Python selection; the README table was generated from that selection.

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | edge, blind | ImportAccounts with a registered Account's id takes it over (email, password, role → admin) | high | New repository test reproduced it; upsert now updates only reserved-domain rows and rejects the batch when any id is skipped | patch |
| 2 | vgap | CI smoke runs `TestLiveDemoAccounts` without `make import`, so it would fail | high | `-test.run Live` matched both live tests; CI now runs `^TestLiveRegisterAndLogin$` only | patch |
| 3 | blind | IdentityAdminService has no access control | medium | Same trust boundary as catalog's admin RPCs (Story 1.6) | defer |
| 4 | blind, edge | "Hashes once" test never counted hashes | low | Hasher now counts `Hash`; test asserts 1 | patch |
| 5 | blind, vgap | Same-id-twice (21000) mapping untested | low | Repository test added; removing the code label fails it | patch |
| 6 | blind, edge | Colliding 8-char email prefixes fail with an opaque unique violation | low | `DemoAccounts` now names the clash (test failed first) | patch |
| 7 | blind, edge, vgap | Repository tests ignore scan errors | low | Scan, `rows.Err` and count errors now checked | patch |
| 8 | vgap | Importer `run()` wiring (env key, identity call) untested | medium | Needs a fixture-based importer integration test; catalog half has the same gap | defer |
| 9 | blind | Old demo rows linger if the picks change | low | Dataset is fixed; a clash fails loudly | reject |
| 10 | blind, edge | No cap on ImportAccounts batch size, multiple Admins possible | low | Internal-only RPC; the importer sends 41 rows | reject |
| 11 | blind | `uuid.Parse` accepts `urn:uuid:` and braces | low | Harmless extra formats | reject |
| 12 | blind | Importer re-reads orders CSVs; dials identity after catalog import | low | Adds about 0.2 s; both CSV picks run before any RPC | reject |
| 13 | blind | README table not tied to code by a test | low | Generated from an independent selection that matched the seeded DB rows | reject |
| 14 | edge | Empty ids counted; `TopIDs` with negative n panics | low | Dataset has no empty ids (real-data test); only caller passes 20 | reject |

## Design Notes

- The importer sends the demo password with each `ImportAccounts` call over the internal network; identity hashes it once per call and never stores it in clear. Re-import rewrites the hash, so a changed `DEMO_PASSWORD` takes effect.
- Seeded and registered Accounts never collide on email: registration rejects the reserved domain (Story 1.4).
- In the real dataset the top 20 Customers and Sellers have no 8-character id-prefix collisions; a collision would fail the batch loudly on the unique email index.

## Verification

**Commands:**
- `make build && make test && make proto` -- expected: exit 0, DB tests run with `DBMIGRATE_TEST_DSN`.
- `make up && make import && make import` -- expected: exit 0 both times.
- `docker compose -f deployments/docker-compose.yml exec postgres psql -U postgres -d identity -c "select role, count(*) from account where email like '%@demo.olist.local' group by role"` -- expected: admin 1, customer 20, seller 20.
- Live login test binary on the `olist` network (as in Story 1.4) for one Customer, one Seller and the Admin.
