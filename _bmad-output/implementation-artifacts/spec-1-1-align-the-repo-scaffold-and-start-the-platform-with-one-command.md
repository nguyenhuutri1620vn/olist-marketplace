---
title: 'Story 1.1: Align the repo scaffold and start the platform with one command'
type: 'chore'
created: '2026-10-31'
status: 'done'
baseline_commit: '822cebf3d19c4994ee793b5783f3293353476814'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** The scaffold still has `customer-service` and `review-service`, no `shipping-simulator`, no per-service databases or migrations runner, no event contracts or buf pipeline, and `make up` starts only Postgres 16 and the gateway.

**Approach:** Reshape the repo to the seven architecture services, give each data-owning service its own Postgres 18 database/user and embedded golang-migrate at startup, seed `contracts/`, `pkg/{outbox,inbox}` and `proto/` with buf, and make one `docker compose` start everything with only gateway and observability UIs on host ports.

## Boundaries & Constraints

**Always:** exactly seven services: api-gateway, identity-service, catalog-service, order-service, payment-service, shipping-simulator, analytics-service. Data services keep `cmd/server`, `internal/{domain,service,repository,transport}`, `migrations/`. Go 1.27, Postgres 18, Kafka 4.3 KRaft, versions per architecture stack. Write a failing test before each testable behavior.

**Never:** add the `web` container or port (Story 1.7); add logging/tracing/health/metrics or gRPC servers (Story 1.2); implement outbox/inbox/event types (Epic 2); publish service or Postgres ports to the host; let one service user reach another service's database.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Empty migrations | service `migrations/` has no `.sql` | startup succeeds, migrate reports no change | N/A |
| Re-run | migrations already applied | no-op, service keeps running | N/A |
| Bad DSN / DB down | `DATABASE_URL` unreachable or missing | process exits non-zero with a clear error; compose restarts it | error names the missing/failed setting |
| Cross-DB access | `catalog` user connects to `order` DB | connection refused (permission denied) | N/A |

</frozen-after-approval>

## Code Map

- `go.work`, `Makefile`, `.github/workflows/ci.yml`, `deployments/docker-compose.yml` -- still list customer/review services; Postgres 16 publishes 5432.
- `services/*/Dockerfile` -- build from service dir only (`COPY . .`), cannot see `pkg`/`proto`; must build from repo root using the workspace.
- `services/*/cmd/server/main.go` -- one-line `log.Println` stubs; module path `github.com/nguyenhuutri1620vn/olist-marketplace/...`, `go 1.23`.
- `proto/{catalog,order}/v1/*.proto` -- empty services; `go_package` points at `proto/<svc>/v1`, must move to `proto/gen/go/<svc>/v1`.
- `pkg/{eventbus,logger,tracing}/doc.go` -- doc-only seeds; follow this style for `outbox`, `inbox`.
- `services/api-gateway/internal/{handler,middleware,routing}` -- keep; gateway has no DB.
- `services/order-service/internal/{outbox,saga}` -- keep untouched.

## Tasks & Acceptance

**Execution:**
- [x] `tests/scaffold/` (new module) -- repo-shape test: go.work, Makefile SERVICES, CI filters, compose services/ports, service layouts, proto dirs, contracts, pkg dirs -- red first, guards AC 1/2/4.
- [x] `services/identity-service` (git mv from customer-service), delete `services/review-service`, new `services/shipping-simulator` -- AD-1 service set.
- [x] `pkg/config` -- `Require(keys...)` returns values or one error naming all missing env vars; unit-tested.
- [x] `pkg/dbmigrate` -- `Up(ctx, dsn, fs.FS)` via golang-migrate iofs + pgx/v5; empty source and `ErrNoChange` are success; unit + Postgres integration test (gated by `DBMIGRATE_TEST_DSN`).
- [x] `services/<data-svc>/migrations/migrations.go` -- `//go:embed` FS that compiles with zero `.sql` files.
- [x] `services/*/cmd/server/main.go` -- data services: load config, run migrations, block until SIGTERM; gateway: HTTP listener on `:8080`.
- [x] `services/*/go.mod`, `go.work` -- Go 1.27, require+replace shared modules; add `contracts`, `proto/gen/go`, `tests`.
- [x] `services/*/Dockerfile`, `.dockerignore` -- root build context, `golang:1.27.3-alpine` builder, distroless runtime.
- [x] `deployments/postgres/init/01-databases.sh` + `deployments/postgres/check-isolation.sh` -- per-service DB/user, `REVOKE CONNECT … FROM PUBLIC`; check script written first.
- [x] `deployments/docker-compose.yml`, `deployments/prometheus/prometheus.yml` -- seven services, postgres:18.6, apache/kafka:4.3.2 KRaft, jaeger 2.22.0, prometheus v3.15.1, grafana 13.3.0, kafka-ui v1.5.0; healthchecks; only gateway 8080, jaeger 16686, prometheus 9090, grafana 3000, kafka-ui 8081 published.
- [x] `proto/buf.yaml`, `proto/buf.gen.yaml`, `proto/{identity,catalog,order,payment,analytics}/v1/*.proto`, `proto/gen/go` -- buf v2, STANDARD lint, FILE breaking (except go_package), pinned local Go plugins.
- [x] `contracts/events/v1/doc.go`, `pkg/outbox/doc.go`, `pkg/inbox/doc.go` -- seeds.
- [x] `Makefile`, `.github/workflows/ci.yml` -- new SERVICES; `test` covers services + shared modules; `proto` = generate + lint + breaking via pinned `go run` buf; CI jobs for scaffold test, buf lint/breaking/generate-diff, Go 1.27.
- [x] `README.md` -- update structure table and commands.

**Acceptance Criteria:**
- Given a clean checkout with Docker running, when `make up` runs, then all seven services plus Postgres, Kafka, Jaeger, Prometheus, Grafana and kafka-ui are running on one network, and `docker compose ps` shows host ports only for gateway and observability UIs.
- Given Postgres initialised, when `check-isolation.sh` runs, then each service user connects to its own DB only.
- Given the repo, when `make build`, `make test` and `make proto` run, then all pass and generated code is unchanged.

## Implementation Notes

- Implemented directly in the orchestrating session (not a subagent) to apply TDD as the user requested; every behavior had a red run first.
- Added `pkg/bootstrap.Migrate` (requires `DATABASE_URL`, then `dbmigrate.Up`) so each `main.go` stays wiring-only.
- Docker Desktop on this machine cannot bind-mount paths under `~/Documents`, so Postgres init and Prometheus config are baked into small images (`deployments/{postgres,prometheus}/Dockerfile`) instead of bind mounts; this also works on any remote Docker host.
- `buf.gen.yaml` has no `clean: true`: it would delete `proto/gen/go/go.mod`.
- Mutation checks: removing the `os.ErrNotExist` handling fails the empty-migrations test; a cross-DB grant fails `check-isolation.sh`; renaming a service fails `buf breaking`.
- README kept in Vietnamese to match the existing file.

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge, vgap | `dbmigrate.Up` swallows every `os.ErrNotExist`, hiding "no migration found for version N" | medium | golang-migrate `versionExists` wraps `os.ErrNotExist`; new test `TestUpFailsWhenDatabaseVersionIsMissingFromSource` failed before the fix | patch |
| 2 | blind, edge | Misnamed `.sql` migration is skipped silently | medium | iofs `Init` `continue`s on parse errors; `TestUpRejectsMisnamedSQLFiles` failed before the fix | patch |
| 3 | blind, vgap | DB-backed tests never run in CI | medium | No job set `DBMIGRATE_TEST_DSN`; added `integration` job with a Postgres service | patch |
| 4 | blind, vgap | `make up` and isolation check never run in CI | medium | No compose job existed; added `smoke` job (`make up`, `make check-db-isolation`, `down -v`) | patch |
| 5 | edge | dbmigrate and bootstrap tests race on one database | medium | Packages run in parallel and dbmigrate dropped `schema_migrations`; tests now use a per-test schema via `search_path` | patch |
| 6 | edge | Kafka writes to `/tmp/kafka-logs`, volume unused | medium | Live logs and `ls` in the container confirmed; set `KAFKA_LOG_DIRS` | patch |
| 7 | blind | Admin UIs published on 0.0.0.0 without auth | medium | kafka-ui has no auth, Grafana default admin; ports now bound to 127.0.0.1 | patch |
| 8 | blind, edge | api-gateway `main` returns before `Shutdown` finishes; error discarded | low | `ListenAndServe` returns `ErrServerClosed` immediately on `Shutdown`; now waits on a done channel and logs | patch |
| 9 | blind, edge, vgap | Scaffold test checks DB names by substring; compose DSN wiring untested | medium | Mutating order-service DSN to catalog passed before; new tests parse `DATABASES=` lists and each `DATABASE_URL` (mutation now fails) | patch |
| 10 | edge, blind | Isolation check skips `template1` | low | Init revokes it but check never probed it; added | patch |
| 11 | edge | Init script fails half-way on a later missing password | low | Passwords were checked per loop iteration; now validated before any change | patch |
| 12 | blind | Containers run as root | low | distroless default user is root; switched to `:nonroot` | patch |
| 13 | blind | `fs.Glob` assertion in per-service tests cannot fail | low | Glob errors only on bad patterns; assertion removed | patch |
| 14 | blind, edge | Init runs once; password change needs volume reset | low | Documented `down -v` in README | patch |
| 15 | blind | `--wait` does not prove data services migrated | medium | No healthcheck until Story 1.2 `/readyz` | defer |
| 16 | blind | Kafka `CLUSTER_ID` not a 22-char base64 UUID will fail | false | Live broker logged "Using provided cluster id" and is healthy | reject |
| 17 | blind | `m.Close` never called; ctx not wired to `GracefulStop`; no `ErrDirty` hint | low | `m.Close` added (direct); ctx/GracefulStop adds wiring for a one-shot startup; `ErrDirty` message already says to force | partial patch / reject |
| 18 | edge | Isolation check counts connection errors as passes | false | If the container is down, every own-DB check fails, so the script exits 1 | reject |
| 19 | blind, edge | Socket `trust` auth means passwords untested | low | Services connect over TCP with password and log "migrations applied"; smoke job covers it | reject |
| 20 | blind, edge | Unencoded passwords in `DATABASE_URL` | low | Defaults are plain words; only custom overrides with reserved chars break | reject |
| 21 | blind, edge | `buf breaking` against `github.event.before` fails for zero/force-pushed SHA | low | Push trigger is only develop/main; rare, fix adds branching | reject |
| 22 | edge | `BUF_AGAINST` needs a local `develop` branch | low | Overridable variable; normal clones have develop | reject |
| 23 | edge | Stale `.pb.go` after deleting a `.proto` | low | No proto deletions expected soon; fix adds cleanup logic | reject |
| 24 | blind | Version skew across module go.mods | false | Workspace MVS picks max; protobuf runtime newer than generator is supported | reject |
| 25 | blind | CI paths filter misses Dockerfiles/deployments | low | `smoke` job now runs on every push/PR | reject |
| 26 | blind | MODULES / CI loop / go.work not cross-checked | low | Rare drift; adds test code | reject |
| 27 | edge | Ping can hang without `connect_timeout` | low | Blackholed host unlikely on a compose network | reject |
| 28 | edge | Whitespace-only env var passes `Require` | low | Unlikely in compose env | reject |
| 29 | edge | SIGTERM during startup exits 1 | low | Cosmetic exit code during a stop | reject |

## Design Notes

- Generated proto code is committed in its own module so Docker builds need no buf.
- `buf breaking` uses FILE except `FILE_SAME_GO_PACKAGE`: WIRE_JSON (planned) missed a deleted service in a mutation test; FILE catches it, and the exception tolerates the one-time `go_package` move to `proto/gen/go`.
- Embed with `//go:embed *` so the package compiles with no `.sql`; iofs skips names that are not migrations.
- No gRPC server or ops port in 1.1; Story 1.2 adds them via shared helpers.

## Verification

**Commands:**
- `make build && make test` -- expected: all modules build, all tests pass (DB tests skip without DSN).
- `make proto && git diff --exit-code proto/gen` -- expected: lint/breaking clean, no diff.
- `make up && sh deployments/postgres/check-isolation.sh && docker compose -f deployments/docker-compose.yml ps` -- expected: all containers up/healthy, isolation passes, ports as specified.
- `DBMIGRATE_TEST_DSN=… go test ./pkg/dbmigrate/...` -- expected: integration tests pass against compose Postgres.
