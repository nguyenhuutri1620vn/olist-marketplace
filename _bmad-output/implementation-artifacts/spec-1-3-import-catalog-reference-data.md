---
title: 'Story 1.3: Import catalog reference data'
type: 'feature'
created: '2026-10-31'
status: 'done'
baseline_commit: 'cd695a476df6c222ff3168f1f0baeca17e3a6d4d'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** catalog-service has no tables and no RPCs, and `tools/olist-importer` is a stub, so the catalog holds no real Categories, Sellers or Products (FR17, FR13).

**Approach:** catalog-service gets migrations for Category, Seller and Product plus admin gRPC `ImportCategories`, `ImportSellers`, `ImportProducts` (idempotent upserts keyed by dataset id). `make import` runs the importer in a container on the compose network; it reads the CSVs, picks each Product's most recent Order Item, and calls only those RPCs.

## Boundaries & Constraints

**Always:** money is int64 centavos parsed from the decimal string (no floats); Olist ids are stored as `uuid`; display name is `<Category English name> #<first 4 chars of product id>`; the most recent Order Item is `ORDER BY order_purchase_timestamp DESC, order_id DESC, order_item_id DESC LIMIT 1`; "Uncategorized" is a real Category row (pt and en "Uncategorized"); an untranslated category uses its Portuguese name as English name; batches stay well under gRPC's 4 MB limit. Test first for every behavior.

**Never:** SQL from the importer against any service DB; publish catalog's gRPC port; load Orders, Order Items, Customers or Accounts; bind-mount `data/` (Docker here cannot mount under `~/Documents`, so copy files into the container).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Price parse | `"58.90"`, `"7.6"`, `"1050"` | 5890, 760, 105000 centavos | `"1.234"`, `"-1"`, `"abc"` rejected |
| Several Order Items | Product in 3 orders | price/freight/seller of the newest purchase; ties broken by order_id then order_item_id DESC | N/A |
| No category | empty `product_category_name` | category Uncategorized, name `Uncategorized #abcd` | N/A |
| Untranslated | `pc_gamer` not in translation CSV | Category en = `pc_gamer`, name `pc_gamer #abcd` | N/A |
| Product without Order Item | not in items CSV | skipped and counted in the importer summary | warn log |
| Re-import | same CSVs twice | row counts unchanged, no duplicates | N/A |
| Bad RPC input | id not 32 hex, unknown category or seller, negative price | whole batch rejected | gRPC `INVALID_ARGUMENT`, importer exits 1 |

</frozen-after-approval>

## Code Map

- `proto/catalog/v1/catalog.proto` -- empty `CatalogService`; add the admin RPCs here (buf STANDARD lint: unique Request/Response per RPC). Regenerate with `make proto-generate`; never hand-edit `proto/gen/go`.
- `services/catalog-service/cmd/server/main.go` -- `app.Main(app.Service{... GRPC: true})`; needs a hook to register handlers.
- `pkg/app/app.go` -- `Service` struct; `Run` builds `grpcx.NewServer(reg)` and opens the pgx pool only when `Migrations` is set. Add a `RegisterGRPC func(*grpc.Server, *pgxpool.Pool)` field; keep everything else.
- `pkg/grpcx` -- `Dial(target)` for the importer client (tracing included).
- `services/catalog-service/migrations/` -- embedded FS; add `000001_catalog.{up,down}.sql`.
- `services/catalog-service/internal/{domain,service,repository,transport}` -- empty `.gitkeep` dirs; hexagonal rules: domain imports nothing internal.
- `pkg/dbmigrate/dbmigrate_test.go` `testDSN` -- per-test-schema pattern to copy for repository tests (`DBMIGRATE_TEST_DSN`).
- `tools/olist-importer/` -- stub main + go.mod (in `go.work` and Makefile `MODULES`); no Dockerfile yet.
- `deployments/docker-compose.yml`, `Makefile` -- add an importer service under a `tools` profile and an `import` target.
- `data/` -- gitignored; 32,951 products (610 without category), 3,095 sellers, 71 translations (`pc_gamer`, `portateis_cozinha_e_preparadores_de_alimentos` missing), 112,650 order items; translation CSV starts with a UTF-8 BOM.

## Tasks & Acceptance

**Execution:**
- [x] `services/catalog-service/migrations/000001_catalog.{up,down}.sql` -- `category(name_pt PK, name_en)` with the Uncategorized row, `seller(id uuid PK, city, state)`, `product(id uuid PK, category FK, display_name, price, freight bigint ≥ 0, seller_id FK)`.
- [x] `proto/catalog/v1/catalog.proto` -- `CatalogAdminService` with the three Import RPCs returning the upserted count.
- [x] `services/catalog-service/internal/domain` -- validation (32-hex id, state, non-negative money), English-name fallback, display name.
- [x] `services/catalog-service/internal/service` -- import use cases over a repository port.
- [x] `services/catalog-service/internal/repository` -- batch upserts in one transaction (`ON CONFLICT DO UPDATE`); display name derived from the Category row.
- [x] `services/catalog-service/internal/transport` -- gRPC handler mapping domain errors to `INVALID_ARGUMENT`.
- [x] `pkg/app` + catalog `main.go` -- `RegisterGRPC` hook wired with the pool.
- [x] `tools/olist-importer` -- CSV readers, `ParseCentavos`, most-recent-item selection, batching (1,000), `import` command; Dockerfile (distroless, empty `/data`).
- [x] `deployments/docker-compose.yml`, `Makefile` -- `olist-importer` service (profile `tools`, depends on catalog healthy); `make import` creates it, copies the five CSVs in with `docker cp`, runs it attached.
- [x] Tests -- unit tests for the I/O matrix; Postgres-backed repository and gRPC tests; scaffold test that the importer is not in `make up` and publishes no port.

**Acceptance Criteria:**
- Given `make up` then `make import`, when it finishes, then catalog holds 32,951 Products, 3,095 Sellers and 74 Categories (73 dataset categories plus Uncategorized), and the importer exits 0.
- Given Import has run, when `make import` runs again, then the counts are unchanged and order-service's database has no rows.
- Given any Product, then its display name never starts with an empty Category name.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD (red run before every behavior), as in Stories 1.1 and 1.2.
- `pkg/app.Service.RegisterGRPC(*grpc.Server, *pgxpool.Pool)` registers handlers before serving; catalog's `main.go` wires transport → service → repository.
- Admin RPCs live in a new `proto/catalog/v1/catalog_admin.proto` (`CatalogAdminService`); the empty public `CatalogService` stays for Story 1.8.
- Repository upserts use `unnest` arrays in one statement per batch; display name is computed in SQL from the Category row (`name_en || ' #' || left(id::text, 4)`), and re-importing a Category renames its Products. FK, check, cardinality (same id twice) and bad-uuid errors map to `domain.ErrInvalid` → `INVALID_ARGUMENT`.
- Importer: `internal/olist` (CSV by header name, BOM stripped, `ParseCentavos`, most-recent-item selection) and `internal/importer` (dependency order Categories → Sellers → Products, batches of 1,000, stops at first failure). `TestLoadRealDataset` runs against the Kaggle files when `OLIST_DATA_DIR` is set.
- `make import`: macOS `tar` adds `com.apple.quarantine` xattrs that `docker cp` rejects, so the tar uses `--no-xattrs`.
- Mutation checks: reversing any of the three tie-break comparisons fails the most-recent-item test; dropping the category-rename update fails its test; dropping the empty-batch guard fails the service test.
- Live check: two `make import` runs each upserted 73 categories, 3,095 sellers, 32,951 products in about 7 s; catalog holds 74 categories (with Uncategorized), 610 Uncategorized products, 0 malformed display names; order DB has no tables besides migrations. An independent Python recomputation of every Product's category, display name, price, freight and Seller matched all 32,951 rows.

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge | Importer RPCs have no deadline; a stalled catalog hangs `make import` | medium | Only the signal ctx was passed; each batch now gets a 1-minute timeout (test failed first) | patch |
| 2 | blind | Admin service has no access control | medium | Registered on the shared gRPC server; only the unpublished port limits callers; auth needs Story 1.6 JWKS | defer |
| 3 | blind, edge | Rejected batch does not say which row failed | low | `classify` dropped `pg.Detail`; now included (test failed first) | patch |
| 4 | blind | Re-imports rewrite every row; counts cannot show changes | low | Correct result; 33k-row rewrite takes seconds locally | reject |
| 5 | blind | Constructors normalise inconsistently (uppercase hex rejected, untrimmed seller fields) | low | Dataset ids are lowercase and fields clean; adds rules for unseen input | reject |
| 6 | blind | Seeded Uncategorized row can be renamed by ImportCategories | low | Needs a caller sending `name_pt = Uncategorized`; dataset has none | reject |
| 7 | blind | Schema lacks CHECKs mirroring Go rules | low | catalog is the only writer (AD-2) | reject |
| 8 | blind | `classify` codes 22P02/23514 and seller/category failures untested | low | Domain validation makes them unreachable from the RPC path | reject |
| 9 | blind | Skipped products logged only after success, one line each | low | Moved before batches as one line with count and ids | patch |
| 10 | blind, edge | `RegisterGRPC` without `GRPC` ignored; nil pool without Migrations | low | Single caller sets both; guard adds branches | reject |
| 11 | blind, vgap | CI never builds the importer image | medium | `make up` skips the tools profile; smoke now builds it | patch |
| 12 | blind | Stopped importer container left behind | low | Kept for `docker logs`; removed at next run | reject |
| 13 | blind | Rows dropped from CSVs are never deleted | low | Upsert-only import is the intent (AD-12) | reject |
| 14 | blind | Diff omits generated code and go.sum | false | Deliberately excluded from the review diff; files exist and `make proto` regenerates cleanly | reject |
| 15 | blind | Importer env vars undocumented | low | README now lists `DATA_DIR`, `CATALOG_ADDR`, `OLIST_DATA_DIR` | patch |
| 16 | edge | `Run` with batch size ≤ 0 loops or panics | low | Only caller passes the 1,000 constant | reject |
| 17 | edge | Duplicate ids or empty translation name in CSVs abort the import | low | Real dataset has neither (`TestLoadRealDataset`); failure is loud | reject |
| 18 | edge | Concurrent product import and category rename can leave a stale name | low | Import is a single sequential CLI | reject |
| 19 | edge | `app_test` reads hook results without synchronisation | low | Now passed over a channel; `-race` clean | patch |
| 20 | edge | Host CSVs with mode 0600 unreadable by nonroot | low | Kaggle files are 0644; bsdtar has no `--mode` | reject |
| 21 | edge | Spec names `catalog.proto` / domain display-name rule; code differs | low | Fix is a spec edit; Implementation Notes record both choices | reject |
| 22 | vgap | No test calls a service registered through `RegisterGRPC` on the served port | medium | App test now registers `test.Ping` and calls it over `GRPC_ADDR`; registering on another server fails it | patch |
| 23 | vgap | Makefile `OLIST_FILES` not tied to files `olist.Load` reads | low | Scaffold cannot import the importer's internal package | defer |
| 24 | vgap | `olist.Files` is dead code | low | Removed | patch |

## Design Notes

- The importer picks the Order Item (it is the only side that can read the CSVs); catalog derives the display name from its own Category row, so names stay consistent with stored categories.
- Category key is its Portuguese dataset name; it has no Olist id, and Story 1.8 filters by it.
- Admin RPCs are unauthenticated for now: the gRPC port is internal only. JWT re-verification arrives with Story 1.6.

## Verification

**Commands:**
- `make build && make test && make proto` -- expected: exit 0, DB tests run with `DBMIGRATE_TEST_DSN`.
- `make up && make import && make import` -- expected: both imports exit 0.
- `docker compose -f deployments/docker-compose.yml exec postgres psql -U postgres -d catalog -c "select count(*) from product"` -- expected: 32951 after both runs; same check on `order` DB shows no tables with rows.
