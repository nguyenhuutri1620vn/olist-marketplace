---
title: 'Story 2.4: Customer sees own Orders and status timeline'
type: 'feature'
created: '2026-11-01'
status: 'done'
baseline_commit: '2fdbcfa368feb2420d6804c67b20e2fa014f4d89'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-2-context.md'
  - '{project-root}/_bmad-output/planning-artifacts/ux-designs/ux-olist-marketplace-2026-10-30/DESIGN.md'
  - '{project-root}/_bmad-output/planning-artifacts/ux-designs/ux-olist-marketplace-2026-10-30/EXPERIENCE.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** A Customer who checked out cannot see the Orders: order-service has no read RPCs, the gateway's `/api/orders` routes are placeholders and `/orders`, `/orders/:id` are placeholder pages (FR7, UX-DR2, UX-DR11, UX-DR12).

**Approach:** order-service serves Customer-only `ListMyOrders(page, page_size)` and `GetOrder(order_id)`, scoped to the caller's `sub`; the gateway exposes them as `GET /api/orders?page=&page_size=` and `GET /api/orders/{id}`; the web app gets My Orders, Order detail and a shared status badge.

## Boundaries & Constraints

**Always:** only Orders with `customer_id` = caller; another Customer's or an unknown id → `NOT_FOUND` (same answer, 404); a non-uuid id → `INVALID_ARGUMENT`; list sorted `purchased_at DESC, id DESC` with `total` count, page bounds as Story 1.8 (page ≥ 1, page_size 1..100, gateway defaults 1/20, non-integers 400); list rows: order id, purchased_at, status, total (Σ price+freight), seller id, item count; `GetOrder`: those plus voucher_amount, items (product id, category, price, freight), timeline oldest first (`at`, then status order) and, when `canceled`, `cancel_reason` and `voucher_restored`; timestamps RFC 3339 UTC in the API; the web shows dates as `YYYY-MM-DD HH:mm` in São Paulo time; status badge = shadcn `Badge` pill filled with the DESIGN.md token for its status, white text with the exact dataset word; My Orders columns Date, Order (last 8 chars of the id — decision by the human: UUIDv7 ids start with their timestamp, so the first 8 match for Orders made together — a link to the detail), Seller (first 8 chars), Total (Money), Status; 20 per page, Previous/Next and "Page X of Y" in the URL like Products; a Refresh button re-fetches (no polling); empty list → "No Orders yet."; Order detail is titled "Order <last 8 chars>" and shows status badge, purchased date, items table (name `<category> #<first 4 of product id>`, price, freight as Money), total, timeline list, Refresh, and "Order not found." for 404; loading skeletons and error toasts as elsewhere. Test first for every behavior.

**Never:** Seller or Admin Order views (Epic 3); paying (Stories 2.7, 2.10); live push or auto-refresh; editing Orders.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| List | Customer with 3 Orders from 2 Checkouts | newest first, ties by id desc, totals, `total` 3 | N/A |
| Paging | page_size 2, pages 1 and 2 | no Order repeated or missing | page 0 / size 101 → 400 |
| Isolation | Customer B lists, or gets A's Order id | B sees none of A's Orders | 404 for A's id |
| Detail | own Order | items, total, timeline `created` with its time | unknown id 404, `abc` 400 |
| Canceled | Order with status canceled, reason, voucher restored | both fields returned and shown | N/A |
| Roles | Seller/Admin token, no token | not called | 403 / 401 |
| Badge | each of the six statuses | pill, token background, white exact word | N/A |
| Refresh | status changed in the DB after load | shown after Refresh only | N/A |
| Empty | Customer with no Orders | "No Orders yet." | N/A |

</frozen-after-approval>

## Code Map

- `proto/order/v1/order.proto` -- `Checkout`, `Order`, `OrderItem` exist; add `ListMyOrders`, `GetOrder`, `OrderSummary`, `OrderDetail` (with timeline, cancel fields, `google.protobuf.Timestamp`).
- `services/order-service/internal/domain/checkout.go` -- `Order`, `Item`, `Statuses`, errors; add `ErrNotFound`, page query (mirror catalog's `NewProductQuery`), `TimelineEntry`, cancel fields on `Order`.
- `services/order-service/internal/repository/postgres.go` -- `FindCheckout` shows the read pattern (`replace(...,'-','')` for dataset ids, UTC); add `ListOrders(customer, page)` (count + page in one read snapshot like catalog's `ListProducts`) and `GetOrder(customer, id)`.
- `services/order-service/internal/{service,transport}` and `cmd/server/main.go` -- add use cases, handlers, Customer-only policy entries.
- `services/api-gateway/internal/handler/{catalog.go,checkout.go}` -- `intParam`, `WriteGRPCError`, narrow client interfaces; add `orders.go`; `routing.go` routes `GET /api/orders` and `GET /api/orders/{id}` behind `RequireRole(customer)` (`Deps.Orders` widens to the three RPCs).
- `web/src/pages/Products.tsx` -- pattern for URL paging, skeleton, toasts; `web/src/test/gateway.ts` -- fake gateway to extend with orders; `src/index.css` -- add status color tokens; add shadcn `badge`; routes in `src/App.tsx`.

## Tasks & Acceptance

**Execution:**
- [x] `proto/order/v1` -- read RPCs and messages.
- [x] `services/order-service` -- domain, repository, service, transport, policy.
- [x] `services/api-gateway` -- the two GET routes.
- [x] `web/` -- `StatusBadge`, `MyOrders`, `OrderDetail`, date formatting, routes, fake gateway.
- [x] Tests -- matrix rows at repository (Postgres), transport, gateway and web levels.

**Acceptance Criteria:**
- Given `make up` and two demo Customers, when Customer A checks out twice and lists `/api/orders`, then A sees exactly those Orders newest first, and Customer B gets 404 for A's Order id.
- Given the browser on `/orders` as Customer A, when a row is opened, then the detail shows items, total and a `created` timeline entry; after the Order's status is changed in the DB, Refresh shows the new badge.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD.
- order-service: `domain.NewPage`, `ParseOrderID`, `StatusRank`, `Summary`/`SummaryPage`, `StatusChange`, `Detail`, `ErrNotFound`; `repository.ListOrders` (count + page in one read-only snapshot, totals and item counts from `order_items`) and `GetOrder` (owner check in the WHERE clause, items by line, timeline `ORDER BY at, array_position(statuses, status)`); `service.MyOrders`/`Order`; `ListMyOrders`/`GetOrder` handlers (uuid check before the use case, NOT_FOUND mapping); both Customer-only in the policy.
- Gateway: `handler.ListOrders` (`intParam` defaults 1/20) and `handler.GetOrder`, RFC 3339 UTC timestamps; `routing.Orders` combines `Checkouter` and `OrderReader`; routes behind `RequireRole(customer)`. The Story 1.6 role-group table no longer lists `/api/orders` (now real routes, covered by their own role test).
- Web: `StatusBadge` (shadcn `badge`, `bg-status-*` from `@theme` tokens, white text, unknown words uncoloured), `lib/dates.formatSaoPaulo`, `lib/orders` (types, `itemName`, `shortID`, `orderRef`), `lib/paging.pageParam`, `components/Pager` (now also used by Products), pages `MyOrders` and `OrderDetail` (Refresh re-fetches via a counter in the request key; 404/400 show "Order not found."); fake gateway serves 25 Orders for the demo Customer and checks the bearer.
- Also fixed (Story 2.2 code): `inbox.WatchDLQDepth` waited a full 15 s after a failed poll, leaving the gauge missing; on a freshly started broker the first poll fails while the group coordinator loads, which made `pkg/app`'s consumer test flaky. Failed polls now retry after 1 s; the test passes right after a broker restart.
- Mutation checks: `id ASC` as the list tie-break and `status DESC` for timeline ties each fail their repository test.
- Live check (`make up`): Customer A checked out twice → `GET /api/orders` total 4, newest first, totals and item counts right; `GET /api/orders/{id}` items and a `created` timeline; Customer B gets 404 on A's Order and an empty list; `abc` 400, `page_size=101` 400. Browser as A: My Orders rows with São Paulo dates, Money and badges; Order detail; after cancelling the Order in the DB, Refresh showed the red `canceled` badge, "Canceled: CARD_DECLINED. Voucher restored." and the `created → canceled` timeline; after the id change the Order column shows distinct references.

## Spec Change Log

- 2026-11-01, browser check: the first 8 characters of UUIDv7 Order ids are their timestamp, so every Order of a Checkout showed `01a1967e`. The human chose the last 8 characters for the Order column and the detail title (frozen block amended by that decision). Seller ids keep their first 8 (dataset ids are random). KEEP: everything else.

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge | An Order without items (some Historical ones) makes `ListOrders` fail for the whole page | medium | Test reproduced `cannot scan NULL into *string`; one lateral join with `coalesce` now | patch |
| 2 | edge | List and detail can name different Sellers for a multi-Seller Order | low | Test showed `min` vs first line; detail now uses the lowest id too | patch |
| 3 | blind, edge | A canceled Order without a reason shows "Canceled: ." | low | Now "Canceled." (test failed first) | patch |
| 4 | blind | The Voucher amount is never shown; the total looks like what was paid | medium | Detail shows Voucher and the card remainder when a Voucher applies (test failed first) | patch |
| 5 | blind, edge | Gateway turns an empty detail into a 200 with 1970 dates | low | Now INTERNAL 500 (test failed first) | patch |
| 6 | blind | Detail title built from the raw URL id | low | Uses the canonical id once loaded (test failed first) | patch |
| 7 | vgap, blind | OrderDetail 400 and outage paths, MyOrders past-the-last page, `?page=` fallback and the dataset's DST dates untested | low | Tests added; the fake gateway answers 400 for malformed ids | patch |
| 8 | blind | Other methods on `/api/orders` lost their gate coverage | low | New routing test for POST/DELETE as Customer and Seller | patch |
| 9 | blind, vgap | `domain.StatusRank` unused; the tie-break lives in SQL | low | Removed | patch |
| 10 | blind | Three `order_items` subqueries per row | low | Folded into the lateral join of #1 | patch |
| 11 | vgap, blind | WatchDLQDepth's fast retry untested; unrelated to the story | low | Needs a fault-injection seam; kept here because it fixed this story's flaky verification | defer |
| 12 | blind | Failed loads leave an empty table with only a toast | low | Spec: failures are toasts, pages stay usable (as Products) | reject |

## Design Notes

- NOT_FOUND for another Customer's Order (not PERMISSION_DENIED) so ids cannot be probed.
- Timeline ties (`approved` and `processing` in one transaction share `at`) are broken by status order.

## Verification

**Commands:**
- `make proto && make build && make test` with `DBMIGRATE_TEST_DSN` -- expected: exit 0.
- `make web-test` -- expected: exit 0.
- `make up`, then curl the matrix and drive the browser -- expected: as in the AC.
