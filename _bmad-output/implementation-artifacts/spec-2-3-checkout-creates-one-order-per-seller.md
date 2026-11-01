---
title: 'Story 2.3: Checkout creates one Order per Seller'
type: 'feature'
created: '2026-11-01'
status: 'done'
baseline_commit: 'bb3d4511b4d92878e5e315da85aea72e996dd0e7'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-2-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** A Customer cannot buy anything: order-service has no Orders, no Checkout RPC and the gateway's `/api/checkout` is a placeholder (FR1).

**Approach:** order-service owns Order, Order Item, status timeline and `checkouts` tables and serves a Customer-only gRPC `Checkout(product_ids, checkout_idempotency_key)`. It snapshots each Product from catalog over gRPC (new public `CatalogService.GetProducts`), groups the items by Seller and, in one transaction, writes one `created` Order per Seller, their first timeline entries and one `OrderCreated` outbox row each, sharing a `checkout_id`. The gateway serves it as `POST /api/checkout`. `OrderCreated` is the first real type in `contracts/events/v1`.

## Boundaries & Constraints

**Always:** Order id UUIDv7; `customer_id` = the caller's token `sub`; columns as in the story (Order: id, customer_id, checkout_id, status, purchased_at, historical=false, voucher_amount=0, cancel_reason NULL, voucher_restored=false; Order Item: order_id, product_id, seller_id, seller_state, category_en, price, freight; timeline: order_id, status, at; `checkouts(customer_id, idempotency_key, request_hash, checkout_id)` PK `(customer_id, idempotency_key)`); status values are the six dataset words with a check constraint; `order_total = Σ(price + freight)` (int64 centavos); `request_hash` = SHA-256 of the sorted product ids, so the same set in another order is the same request; same Customer + key + hash → the original Orders, nothing written; same key, other hash → `ALREADY_EXISTS`; keys scoped per Customer; product list 1..100 ids, each a 32-hex dataset id, no repeats, all known to catalog, else `INVALID_ARGUMENT`; catalog call deadline from `CATALOG_TIMEOUT` (default 2s), catalog unreachable or late → `UNAVAILABLE`, nothing written; `OrderCreated` v1.0 on `order.events`, keyed by order id, carries `from_status:""`, `to_status:"created"` and the AD-10 snapshot (order_id, customer_id, checkout_id, purchased_at, historical, status, voucher_amount, items[product_id, seller_id, seller_state, category_en, price, freight]); `Checkout` is Customer-only at order-service and the gateway; response lists the Checkout's Orders (id, seller_id, status, total, items) sorted by seller id. Test first for every behavior.

**Never:** start the Saga or write any payment command (Story 2.7); Order reads (Story 2.4); web screens (Story 2.5); quantities; Vouchers (Story 2.6).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Two Sellers | 3 products, 2 Sellers | 2 `created` Orders, items split by Seller, totals = Σ(price+freight), one timeline row and one `OrderCreated` each, one `checkout_id` | N/A |
| Replay | same Customer, key, same set (any order) | same `checkout_id` and Orders; no new rows | N/A |
| Key reuse | same key, other set | nothing written | `ALREADY_EXISTS` (HTTP 409) |
| Other Customer | another Customer, same key | a new Checkout of their own | N/A |
| Bad list | empty, duplicate id, > 100 ids, not 32-hex, unknown to catalog | nothing written | `INVALID_ARGUMENT` (400) |
| No key | empty `checkout_idempotency_key` | nothing written | `INVALID_ARGUMENT` |
| Catalog down/slow | unreachable or past `CATALOG_TIMEOUT` | nothing written | `UNAVAILABLE` (503) |
| Wrong role | Seller/Admin token, or none | not called | 403 / 401 |

</frozen-after-approval>

## Code Map

- `proto/catalog/v1/catalog.proto` -- add `GetProducts(product_ids) → products[{product_id, seller_id, seller_state, category_name_en, price_centavos, freight_centavos}]` (unknown ids simply absent). `services/catalog-service` repository/service/transport mirror `ListProducts` (`browse.go`, `transport/catalog.go`); add to `catalogService()` public policy.
- `proto/order/v1/order.proto` -- empty `OrderService`; add `Checkout`.
- `contracts/events/v1/catalog.go` -- `catalog` list is empty; add `OrderCreated` (new `order.go`) and regenerate `schema/` (`go generate`).
- `services/order-service/` -- empty `internal/{domain,service,repository,transport}`; `migrations/000001_outbox` exists (keep), add `000002_orders`; `cmd/server/main.go` `orderService()` sets Outbox — add `Policy` (Checkout → customer), `RegisterGRPC`, catalog client (`grpcx.Dial(CATALOG_ADDR)`, default `catalog-service:9090`). Follow catalog/identity's hexagonal split, `toStatus`, per-test-schema repository tests (`DBMIGRATE_TEST_DSN`).
- `pkg/outbox.Write` -- write `OrderCreated` in the Checkout transaction.
- `pkg/auth` -- `FromContext` for the caller's `sub` and role; `Policy{Roles}`.
- `services/api-gateway/internal/routing/routing.go` -- `/api/checkout` is a gated placeholder; add `handler.RegisterCheckout` for `POST /api/checkout` behind `RequireRole(customer)`, `Deps.Orders`; JWT forwarded by the existing conns.
- `deployments/docker-compose.yml` -- order-service `CATALOG_ADDR`.

## Tasks & Acceptance

**Execution:**
- [x] `proto/catalog/v1`, `services/catalog-service` -- `GetProducts`, public.
- [x] `contracts/events/v1` -- `OrderCreated` + schema.
- [x] `proto/order/v1`, `services/order-service` -- migration, domain (validation, grouping, totals, hash), repository (idempotent Checkout transaction), catalog adapter, service, transport, wiring.
- [x] `services/api-gateway` -- `POST /api/checkout`.
- [x] `deployments/docker-compose.yml`, `README.md`, CI smoke -- env, docs, CI smoke checks the checkout route rejects no token (401) and a bad body (400; CI has no dataset).
- [x] Tests -- matrix rows at domain, repository (Postgres), service (fake catalog), transport and gateway levels; outbox rows decoded as `OrderCreated`.

**Acceptance Criteria:**
- Given `make up` and `make import`, when a demo Customer posts 3 Products from 2 Sellers to `POST 127.0.0.1:8080/api/checkout`, then 2 Orders come back, the order DB holds 2 Orders, 2 timeline rows and 2 `OrderCreated` outbox rows with one `checkout_id`, and both events appear on `order.events`.
- Given the same request again, then the same Orders return and the row counts are unchanged; with catalog-service stopped, a new key returns 503 and nothing is written.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD.
- catalog-service: `domain.NewProductIDs` (1..100 distinct dataset ids), `repository.GetProducts` (known ids, sorted), `Browser.Snapshots`, `GetProducts` handler, public in the policy.
- contracts: `OrderCreated{from_status, to_status, order: OrderSnapshot{…, items[OrderItem]}}` v1.0 on `order.events`, the first catalog type; `schema/OrderCreated.v1.json` generated.
- order-service: `000002_orders` (`checkouts`, `orders` without a seller column — the Seller comes from its items —, `order_items` keyed by `(order_id, line)`, `order_status_timeline` keyed by `(order_id, status)`); `domain.NewCheckout`/`Hash`/`Plan`/`Order.Total`, `domain.Placed`, errors `ErrInvalid`, `ErrKeyReused`, `ErrUnavailable`, `ErrCheckoutExists`; `repository.CreateCheckout` (checkout row `ON CONFLICT DO NOTHING` → `ErrCheckoutExists`, Orders, items, timeline, `outbox.Write(OrderCreated)` in one transaction) and `FindCheckout`; `internal/catalog` adapter (`CATALOG_TIMEOUT`, any failure but INVALID_ARGUMENT → `ErrUnavailable`); `service.Orders.Checkout` reads an existing key before calling catalog, so a replay works while catalog is down, and returns the winner after a lost race; `transport` takes the Customer from the verified claims; `orderService()` builds the use case in `Init` (bad `CATALOG_TIMEOUT` stops startup) with a Customer-only policy.
- Product and Seller ids stay in their 32-hex dataset form in the API and events; Order, Customer and Checkout ids are canonical uuids.
- Gateway: `POST /api/checkout` behind `RequireRole(customer)` (`handler.Checkout`, 10 s timeout); `Deps.Catalog` narrowed to `handler.CatalogBrowser` and `Deps.Orders` is `handler.Checkouter`, so adding RPCs does not break the fakes.
- Also fixed: `pkg/app` consumer test read `/metrics` before the DLQ watcher's first poll (flaky); it now waits for both lines. order-service's go.mod gained the proto replace (`make tidy` failed without it).
- Review fixes: see the triage log; migration 000002 gained the Checkout FK before release (local order DB reset to version 1 to re-apply it).
- Live check (`make up`, imported catalog, demo Customer): 3 Products from 2 Sellers → 2 `created` Orders with totals equal to catalog's Σ(price+freight), 2 timeline rows, 2 outbox rows sent and 2 `OrderCreated` on `order.events`; same key with the ids reversed → identical response, no new rows; same key with another set 409; unknown id, repeated id, empty list 400; Seller 403; no token 401; another Customer with the same key gets a new Checkout; catalog stopped → 503 and no new Orders, while a replay still returns 200; catalog back → 200.

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge | A lost race whose winner cannot be read returns 200 with no Orders; `ON CONFLICT DO NOTHING` without a target also swallows a `checkout_id` clash | medium | `found` was ignored; now an error (test failed first), conflict target `(customer_id, idempotency_key)` | patch |
| 2 | vgap | order-service repository tests never run in CI | medium | `DBMIGRATE_TEST_DSN` set only for pkg, catalog, identity; integration step added | patch |
| 3 | vgap, blind | Multi-item Orders and concurrent same-key creates untested | medium | New Postgres tests; breaking the grouping fails the multi-item test | patch |
| 4 | vgap | `GetProducts` freight never checked (fixture all 0) | low | Fixture freight 37 asserted | patch |
| 5 | edge | NUL byte or invalid UTF-8 in the key gives 500 | low | Rejected as INVALID_ARGUMENT (test failed first) | patch |
| 6 | edge | Non-canonical Customer uuid stored/emitted as given | low | Canonicalised (test failed first) | patch |
| 7 | edge, blind | `purchased_at` in the event has nanoseconds and the caller's zone; replays read local time | low | UTC, truncated to µs in `Plan`, UTC on read (tests failed first) | patch |
| 8 | blind | A catalog snapshot breaking the contract (bad seller id/state, empty category, negative money) reaches Postgres as an opaque error | low | `Plan` rejects it as a system error (test failed first) | patch |
| 9 | blind | Every catalog failure, including Unimplemented/PermissionDenied and the caller's cancel, becomes 503 | low | Only Unavailable/DeadlineExceeded/ResourceExhausted are `ErrUnavailable`; caller cancel returns its ctx error (test failed first) | patch |
| 10 | blind | Catalog outage cause not logged | low | Logged at warn before UNAVAILABLE | patch |
| 11 | blind, edge | Gateway accepts unknown fields and trailing data (a typo becomes an empty list) | low | Rejected with 400 (tests failed first) | patch |
| 12 | blind | No FK from `orders.checkout_id` to `checkouts` | low | Added (nullable for Historical Orders) | patch |
| 13 | blind | Two 100-id limits defined separately; `GetProducts` documented as browsing | low | catalog `MaxSnapshotIDs` with a note; policy comment says internal | patch |
| 14 | blind | No end-to-end proof the gateway forwards the Customer's JWT | low | Live check: the Orders' `customer_id` is the demo Customer's account id, which order-service reads only from the forwarded token | reject |
| 15 | blind | `OrderCreated` schema closed and missing cancel_reason/voucher_restored/total | low | AD-10 fixes the snapshot fields; additions are minor versions (AD-9) and Go consumers ignore unknown fields | reject |
| 16 | blind | Oversized body gets 400 not 413; Content-Type unchecked | low | Same convention as the auth routes | reject |

## Design Notes

- `GetProducts` is public like the other catalog reads: product prices are public data, and order-service then calls catalog without depending on the Customer's token.
- Concurrent requests with one key: the `checkouts` insert waits on the primary key; the loser sees `ON CONFLICT DO NOTHING`, rolls back and returns the winner's Orders (or `ALREADY_EXISTS` if the hash differs).

## Verification

**Commands:**
- `make proto && make build && make test` with `DBMIGRATE_TEST_DSN` -- expected: exit 0.
- `cd contracts && go generate ./... && git diff --exit-code` -- expected: no drift.
- `make up && make import`, then curl the checkout matrix through the gateway -- expected: as in the matrix and AC.
