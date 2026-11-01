---
title: 'Story 1.8: Browse and filter Products'
type: 'feature'
created: '2026-11-01'
status: 'done'
baseline_commit: '6f9a2d1bde2748ab5678fd3ad8beb1d0142d9a9b'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
  - '{project-root}/_bmad-output/planning-artifacts/ux-designs/ux-olist-marketplace-2026-10-30/EXPERIENCE.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** The imported catalog cannot be seen: catalog-service has no read RPCs, the gateway has no catalog routes and `/` is a placeholder (FR13, UX-DR8).

**Approach:** catalog-service adds public gRPC `ListCategories` and `ListProducts(category, page, page_size)`; the gateway serves them without a token as `GET /api/categories` and `GET /api/products?category=&page=&page_size=`; the web Products page at `/` shows a paginated table with a Category filter.

## Boundaries & Constraints

**Always:** both RPCs public in catalog's policy and both routes public at the gateway (NFR7); Category key is `name_pt`, label is `name_en`; `ListCategories` returns every Category including "Uncategorized"; `ListProducts` sorts by `display_name, product_id`, returns the page plus the `total` matching count; `page < 1`, `page_size < 1`, `page_size > 100` or an unknown Category → `INVALID_ARGUMENT` (HTTP 400); a page past the last → empty list with the real `total`; gateway defaults `page=1`, `page_size=20`, non-integer → 400; table columns: name, Category, Seller, price (Money); 20 rows per page with "Previous"/"Next" and "Page X of Y"; filter and page live in the URL query so reload and Back keep them; filter `Select` defaults to "All Categories"; loading shows `TableSkeleton`, failure the error toast; filtered empty → "No Products in this Category.", unfiltered empty → "No Products yet."; decision: the Seller column shows the first 8 chars of the seller id plus location (`6560211a · catanduva, SP`); decision: Category labels are the raw dataset English names everywhere (`bed_bath_table`), matching the display name. Test first for every behavior.

**Never:** selection checkboxes or the Checkout bar (Story 2.5); product images; search; infinite scroll; SQL outside catalog-service.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Categories | `GET /api/categories`, no token | 200 `{categories:[{name_pt,name_en}]}` incl. Uncategorized | N/A |
| Products | `GET /api/products`, no token | 200 `{products:[{product_id,display_name,category_name_en,seller_id,seller_city,seller_state,price_centavos}],total,page,page_size}`, 20 rows | N/A |
| Filter | `category=Uncategorized` | only Products without a dataset Category | N/A |
| Bad input | page 0, page_size 0/101, `page=abc`, unknown Category | nothing listed | 400 `INVALID_ARGUMENT` |
| Past last | page beyond `total` | `products: []`, same `total` | N/A |
| Ties | equal display names | ordered by product id; consecutive pages never overlap | N/A |
| Page `/` | load | 5 skeleton rows, then 20 rows; Uncategorized Products show "Uncategorized" | toast, page usable |
| Filter UI | pick a Category | URL `?category=…`, page 1, only that Category | N/A |
| Empty Category | Category with no Products | "No Products in this Category." | N/A |

</frozen-after-approval>

## Code Map

- `proto/catalog/v1/catalog.proto` -- empty `CatalogService`; add the two RPCs and messages, regenerate with `make proto-generate`.
- `services/catalog-service/internal/domain/catalog.go` -- add `Page`/`ListQuery` validation (bounds, `ErrInvalid`) and read models (`ProductRow`, `Category` reused).
- `services/catalog-service/internal/repository/postgres.go` -- add `Categories` (Uncategorized last, then `name_en`) and `ListProducts` (join category + seller, `ORDER BY display_name, id`, `LIMIT/OFFSET`, count); unknown Category → `ErrInvalid`. Reuse `newRepo` test helper (`DBMIGRATE_TEST_DSN`).
- `services/catalog-service/migrations/000002_product_list.{up,down}.sql` -- indexes `(display_name, id)` and `(category, display_name, id)`.
- `services/catalog-service/internal/service` -- new `Browser` use case beside `Importer`; `internal/transport` -- new `catalog.go` handler reusing `toStatus`.
- `services/catalog-service/cmd/server/main.go` -- register both servers; add the two RPCs to `Policy.Public`; extend `main_test.go`.
- `services/api-gateway/internal/handler/` -- new `catalog.go` (`RegisterCatalog`, 5 s timeout like `auth.go`, `WriteGRPCError`, `writeJSON`); `internal/routing/routing.go` `Deps.Catalog`; `cmd/server/main.go` client from `conns["catalog-service"]`.
- `web/src/pages/Products.tsx` -- new page; `App.tsx` index route; shadcn `select` (add with `npx npm@11`-backed install); reuse `apiFetch`, `Money`, `TableSkeleton`, `toastRequestError`, test helpers in `src/test/gateway.ts`.
- `.github/workflows/ci.yml` smoke -- add an anonymous `GET /api/categories` check.

## Tasks & Acceptance

**Execution:**
- [x] `proto/catalog/v1/catalog.proto` -- `ListCategories`, `ListProducts` with the fields in the matrix.
- [x] `services/catalog-service/{migrations,internal/*,cmd/server}` -- validation, queries, indexes, handlers, public policy.
- [x] `services/api-gateway/{internal/handler,internal/routing,cmd/server}` -- public routes, query parsing, error mapping.
- [x] `web/src/pages/Products.tsx`, `web/src/App.tsx` -- table, filter, pagination, states.
- [x] Tests -- domain bounds, Postgres paging/tie-break/filter/past-last/unknown Category, transport mapping, gateway routes without a token, web page states from the matrix.

**Acceptance Criteria:**
- Given `make up` and `make import`, when `GET 127.0.0.1:8080/api/products?category=Uncategorized` is called without a token, then 200 with `total` 610 and only Uncategorized rows.
- Given the browser at `http://127.0.0.1:5173/` logged out, when a Category is picked and Next is clicked, then page 2 of that Category shows and Back returns to page 1.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD, as in Stories 1.1–1.7.
- Sort order: `lower(display_name) COLLATE "C", display_name COLLATE "C", id`. The compose Postgres (alpine, musl) sorts bytewise under `en_US.utf8`, so a plain `ORDER BY display_name` put the 610 "Uncategorized #…" rows on the first 31 pages; lower-casing keeps them in alphabetical place, and `COLLATE "C"` makes the order independent of the database locale. `migrations/000002_product_list` indexes exactly this order, with and without the Category prefix (EXPLAIN on the live data: index-only scans). Removing `lower()` fails the sort test.
- catalog: `domain.NewProductQuery` (bounds), `repository.Categories` (Uncategorized last) and `ListProducts` (count and page in one read-only repeatable-read snapshot; unknown Category → `ErrInvalid`), `service.Browser`, `transport.RegisterCatalog`; both RPCs added to the public policy.
- Gateway: `handler.RegisterCatalog` (`page`/`page_size` default 1/20, non-int32 → 400 before calling catalog, 5 s timeout, `products` never null), `routing.Deps.Catalog`. A stale token on a public route is ignored.
- Web: `pages/Products.tsx` (URL-driven filter and page, `TableSkeleton`, Money, Seller label, empty and past-last messages, toasts), shadcn `select` added; test gateway serves a fake catalog; jsdom polyfills for Radix Select.
- CI smoke: anonymous categories and products, page 0 → 400.
- Live check: `make up`; without a token `/api/categories` → 74 Categories (Uncategorized last), `?category=Uncategorized` → total 610, only Uncategorized rows; bad inputs → 400; page 99999 → empty with total 32951; walking all 330 pages of 100 returned 32951 unique ids. Browser: table on `/`, filter → `?category=ferramentas_jardim` "Page 1 of 38", Next → page 2, Back → page 1 with the filter kept, Uncategorized filter rows. A garbage bearer through nginx still gets 200.

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge, vgap | "Page X of Y" keeps the previous query's count while loading and after a failure; Previous is live while loading (from `/?page=9` it jumped to page 1); an aborted response could still set the count | low | Count was separate state set only on success; now derived from the current query's result, "Page X" alone until known, both buttons disabled while loading (tests failed first) | patch |
| 2 | blind, edge | Category in the URL that is unknown or not yet loaded leaves the filter blank | low | Radix shows nothing for a value without an item; an item for the unlisted value is now rendered (test failed first) | patch |
| 3 | blind, edge | `$1 = '' OR category = $1` keeps a cached generic plan off the Category index | low | `force_generic_plan` EXPLAIN on live data: filtered page used `product_list_idx` with a filter (9.7 ms); separate SQL with and without the Category now | patch |
| 4 | vgap | `pageParam` int32 bound untested; Next while loading untested | low | `2147483648` and `9999999999` cases; "disabled until loaded" test | patch |
| 5 | blind | Fake gateway accepts page bounds the real one rejects | low | Fake now answers 400 like the gateway | patch |
| 6 | blind | CI smoke never reaches catalog's own 400s | low | Smoke now also sends `page_size=101` and `category=nope` | patch |
| 7 | vgap | Migration embed test lists only 000001 | low | 000002 files added to the test | patch |
| 8 | blind | Prices not right-aligned | false | `Money` renders `block text-right`; the browser screenshot shows right-aligned prices | reject |
| 9 | blind | `total` counts Products the page JOIN drops | false | `product.category` and `product.seller_id` are NOT NULL foreign keys, so every Product joins | reject |
| 10 | blind | Failed load leaves an empty table with only a toast, no Retry | low | Spec: failure shows the error toast and the page stays usable; changing the filter or reloading retries | reject |
| 11 | blind | No `aria-busy` / `aria-live` for loading and pager | low | Spec requires labels and an aria-live toast region, both present; extra live regions add surface | reject |
| 12 | blind | Unbounded `count(*)` per request; huge pages accepted | low | 33k rows, count ~3 ms; a page past the last is a defined empty result | reject |
| 13 | blind, vgap | Down migration lacks `IF EXISTS`, never run; `CREATE INDEX` not `CONCURRENTLY` | low | Same choice as Story 1.4; `dbmigrate` has no down path; migrations run in a transaction where `CONCURRENTLY` is not allowed | reject |
| 14 | blind | Diff omits `select.tsx` and the spec | false | Generated shadcn file and spec deliberately left out of the review diff | reject |

## Design Notes

- `total` comes from a separate `count(*)` with the same filter; 33k rows is cheap and the UI needs "Page X of Y".
- The live dataset has no empty Category, so the empty state is covered by tests only.

## Verification

**Commands:**
- `make proto && make build && make test` -- expected: exit 0, DB tests run.
- `make web-test` -- expected: exit 0.
- `make up && make import` then curl both routes without a token -- expected: matrix above.

**Manual checks (if no CLI):**
- Browser pane on `http://127.0.0.1:5173/`: table, filter, pagination, Back, Uncategorized rows.
