---
title: 'Story 2.5: Select Products and check out in the web app'
type: 'feature'
created: '2026-11-01'
status: 'done'
baseline_commit: '78f53eadab197d7eccaed9d0fbcc7b59eca965ed'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-2-context.md'
  - '{project-root}/_bmad-output/planning-artifacts/ux-designs/ux-olist-marketplace-2026-10-30/EXPERIENCE.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Checkout exists only as an API (Story 2.3); in the browser a Customer cannot pick Products or place a Checkout, and `/checkout` is a placeholder (UX-DR9, UX-DR10).

**Approach:** catalog's product list and `GET /api/products` also carry `freight_centavos`. Products get a select checkbox per row; the selection (a snapshot of each picked Product) lives in a React context mirrored to `sessionStorage`; a sticky bar summarises it and leads to the Checkout review `/checkout`, which groups the selection by Seller and calls `POST /api/checkout` with one idempotency key per selection.

## Boundaries & Constraints

**Always:** decision: totals are price + freight everywhere (bar, Cards, grand total), so the review matches the Orders Story 2.3 creates; each review item shows its price and freight; checkboxes shown to visitors and Customers only (Sellers and Admins browse without them), labelled "Select <name>", kept ticked across pages and Category filters; bar sticky at the bottom, shown when ≥ 1 Product is selected: "N items from M Sellers · R$ total · [Check out]" (singular "1 item", "1 Seller"); "Check out" logged in as Customer → `/checkout`, logged out → Login, then back to `/checkout` with the same selection; `/checkout` with no stored selection (direct open, refresh, empty) → Products with toast "Select at least one Product to check out."; review: header "This will create M Orders." (singular "1 Order"), one `Card` per Seller (title = Seller label as on Products) listing its items and subtotal, a grand total, and "Place Checkout"; the idempotency key is created with the selection, stored with it and replaced whenever the selection changes, so retries and double clicks send the same key; the button is disabled while the request runs and never enabled with an empty selection; success → selection cleared, toast "M Orders created — one per Seller." (M from the response; "1 Order created." for one), navigate to `/orders`; failure → error toast, selection kept; corrupt or foreign `sessionStorage` content is ignored. Test first for every behavior.

**Never:** other catalog or gateway changes beyond adding freight to the product list; a server-side cart; quantities; Vouchers or payment (Stories 2.6–2.10); editing the selection on the review page beyond going back to Products.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Select | tick 2 Products of Seller A on page 1, 1 of Seller B in another Category | bar "3 items from 2 Sellers · R$ …" | N/A |
| Untick | untick all | bar hidden | N/A |
| Reload | reload Products | ticks and bar restored | N/A |
| Logged out | click Check out, log in as Customer | on `/checkout` with the same 3 items | N/A |
| Review | 3 items, 2 Sellers | "This will create 2 Orders.", 2 Cards with subtotals | N/A |
| Empty | open `/checkout` with nothing selected | Products + toast | N/A |
| Place | click Place Checkout (twice quickly) | one request, one key; toast "2 Orders created — one per Seller."; `/orders`; selection gone | API error → toast, stay |
| Retry | first request fails, click again | same key as the failed one | N/A |
| Roles | Seller on Products | no checkboxes, no bar | N/A |

</frozen-after-approval>

## Code Map

- `proto/catalog/v1/catalog.proto` `ProductSummary` (add `freight_centavos = 9`), `services/catalog-service` `domain.ProductSummary`, `repository/browse.go` `ListProducts`, `transport/catalog.go`; `services/api-gateway/internal/handler/catalog.go` `product` JSON.
- `web/src/pages/Products.tsx` -- table, `sellerLabel`, `ProductRow`; add the checkbox column and the bar.
- `web/src/App.tsx` -- `Providers` (session, toaster); add a `SelectionProvider`; `/checkout` route under `RequireRole customer` is a placeholder.
- `web/src/lib/session.ts` -- sessionStorage pattern (`olist.session` key, tolerant parsing) to mirror for `olist.selection`.
- `web/src/session/{SessionContext.tsx,context.ts}` -- `useSession()` for role and logged-in state; `lib/routes.ts` `loginURL`, `safeReturn` (already allows `/checkout` for Customers).
- `web/src/lib/api.ts` `apiFetch`, `components/requestError.ts` toasts, `components/Money.tsx`, `lib/orders.ts` helpers; shadcn `checkbox` to add (DESIGN.md), `card` exists.
- `web/src/test/gateway.ts` -- fake gateway; add `POST /api/checkout` (grouping by Seller, idempotency by key) and freight on products.

## Tasks & Acceptance

**Execution:**
- [x] `proto/catalog/v1`, `services/catalog-service`, `services/api-gateway` -- freight on the product list.
- [x] `web/src/lib/selection.ts`, `web/src/selection/` -- selection store, context, storage mirror, key.
- [x] `web/src/pages/Products.tsx` -- checkboxes, bar.
- [x] `web/src/pages/CheckoutReview.tsx`, `App.tsx` -- review page and route.
- [x] Tests -- matrix rows with the fake gateway; the login round trip through `/checkout`.

**Acceptance Criteria:**
- Given the browser on `make up`, when a visitor ticks 3 Products from 2 Sellers, checks out, logs in as a demo Customer and places the Checkout, then My Orders shows 2 new `created` Orders on top and the bar is gone.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD.
- Backend: `ProductSummary.freight_centavos` (proto field 9), catalog `ListProducts` selects `p.freight`, gateway `GET /api/products` returns `freight_centavos`.
- Web: `lib/selection` (snapshot items, key = `crypto.randomUUID()` replaced on every toggle, tolerant `loadSelection`, `summarize`, `groupBySeller`, storage key `olist.selection`); `selection/SelectionProvider` + `useSelection` inside `Providers`; `components/CheckoutBar` (sticky region "Checkout selection", singular/plural, logged out → `loginURL('/checkout')`); Products shows the checkbox column and bar to visitors and Customers only; `pages/CheckoutReview` (Cards per Seller with item price, freight and subtotal, grand total, "Place Checkout" guarded by a ref and the disabled state, success → toast, `/orders`, clear; failure → toast, key kept); `/checkout` route uses it; shadcn `checkbox` added.
- Test infrastructure: the fake gateway serves freight, a second Seller for Uncategorized Products and `POST /api/checkout` (idempotent per caller and key, new Orders on top of My Orders, optional delay); `test/setup.ts` now dismisses sonner toasts after each test (the module-wide store leaked toasts between tests and hid a real bug).
- Bug found in the browser: after a successful Checkout the cleared selection made the review fire "Select at least one Product to check out." before navigating; the empty toast now waits for no Checkout in flight (test failed first, after the toast-store fix).
- Mutation checks: removing both double-click guards fails the double-click test (with the fake delaying 100 ms); a fresh key per request fails the retry test.
- Review fixes: see the triage log. Live after the fixes: a Customer ticked a Product, logged out, and the bar and `olist.selection` were gone.
- Live check (`make up`, browser pane): logged out, ticked 3 Products from 2 Sellers → bar "3 items from 2 Sellers · R$ 398,18"; Check out → Login → `/checkout` "This will create 2 Orders." with two Cards and the same total; double-clicked Place Checkout → order DB went from 7 to 9 Orders and 1 new checkout row; My Orders shows them on top. After the fix, a one-Product Checkout showed only "1 Order created."; opening `/checkout` with nothing selected returned to Products with the toast.

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge | The selection survives logout, so the next person in the tab inherits it and its key | medium | Cleared on an explicit logout or an account switch, kept on expiry (Login returns to the review) and for a visitor who logs in; tests for logout and a real mid-Checkout 401 failed/guard each rule | patch |
| 2 | vgap | A logged-in Customer's tick → Check out path, and Admins without checkboxes, untested | medium | Tests added; changing either rule now fails them | patch |
| 3 | edge | Two toggles before a render lose one (stale closure) | low | Test failed first; functional state updates | patch |
| 4 | edge | Stored items with a repeated id or negative/fractional money are accepted | low | Rejected by `loadSelection` (tests failed first); `pick` stores only the known fields | patch |
| 5 | blind, edge | StrictMode shows the empty-selection toast twice | low | Toast id `empty-checkout` | patch |
| 6 | edge | A 2xx without `orders` throws after the Orders exist | low | Guarded | patch |
| 7 | blind | Three copies of the product field list; `selected()` copied unknown fields | low | `ProductRow = SelectedProduct & {category_name_pt}`, `pick` copies the listed fields only | patch |
| 8 | blind | Checkbox column header has no accessible name | low | sr-only "Select" | patch |
| 9 | blind, edge | Fake gateway Order ids collide for a Checkout with ≥ 10 Sellers | low | Running counter | patch |
| 10 | blind, edge | A Product removed from catalog after selection blocks the Checkout | low | Catalog data is imported by idempotent upserts and never deleted; the server still refuses with 400 and the toast says which id | reject |
| 11 | blind, edge | Review shows snapshot prices, not re-read ones | low | Reference prices come from the fixed dataset import; Orders snapshot the same catalog values | reject |
| 12 | edge | Lost response, then edit and restore the selection, sends a new key | low | Needs an edit after an unknown outcome; the spec replaces the key on every change | reject |
| 13 | blind | No select-all, no Clear in the bar | low | Not in the story; unticking works | reject |
| 14 | blind | Fake replays a key with other Products; import order; diff omits generated files | low | Real handler covers key reuse (Story 2.3); lint passes; generated files deliberately excluded | reject |

## Verification

**Commands:**
- `make web-test` -- expected: exit 0.
- `make up`, then the AC in the browser pane -- expected: as stated; the order DB holds exactly 2 new Orders after a double click.
