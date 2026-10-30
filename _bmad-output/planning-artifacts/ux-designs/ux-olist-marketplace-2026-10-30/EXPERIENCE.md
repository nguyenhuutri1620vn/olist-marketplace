---
name: Olist Marketplace
status: final
updated: 2026-10-30
sources:
  - ../../prds/prd-olist-marketplace-2026-10-29/prd.md
  - ../../architecture/architecture-olist-marketplace-2026-10-30/ARCHITECTURE-SPINE.md
  - ../../../specs/spec-olist-marketplace/SPEC.md
---

# Olist Marketplace — Experience Spine

## Foundation

This is a single-surface desktop web app: the `web/` React 19 + Vite SPA from the architecture. It talks only to `api-gateway` over REST/JSON. The UI system is shadcn/ui + Tailwind [ASSUMPTION], and `DESIGN.md` holds the visual delta. One app serves all three roles, and the navigation is scoped to the role in the logged-in user's JWT (`role` claim). The UI is simple by intent (PRD SM-C1). Every screen exists to drive or observe a backend flow for the four demos (SM-1…SM-4).

Domain words come from the PRD Glossary and are used exactly: Customer, Seller, Admin, Product, Category, Checkout, Order, Order Item, Order Status, Card Payment, Voucher, Historical Order, Seller Revenue, Replay.

## Information Architecture

| # | Surface | Route | Role | Reached from | Purpose | FR |
|---|---|---|---|---|---|---|
| 1 | Products | `/` | Anyone | App open, nav "Products" | Browse Products, filter by Category, select items for Checkout | FR-13, FR-1 |
| 2 | Login | `/login` | Anyone | Nav "Log in", any 401 | Log in; lists demo accounts | FR-15, NFR-7 |
| 3 | Register | `/register` | Anyone | Login page link | Create a Customer Account | FR-14 |
| 4 | Checkout review | `/checkout` | Customer | Products "Check out" bar | Confirm selection, grouped by Seller; place Checkout | FR-1 |
| 5 | My Orders | `/orders` | Customer | Nav, after Checkout | List own Orders with status | FR-7 |
| 6 | Order detail | `/orders/:id` | Customer | My Orders row | Status timeline, items, pay a `created` Order | FR-2, FR-3, FR-4, FR-7 |
| 7 | Seller Orders | `/seller/orders` | Seller | Nav (Seller home) | Own Orders, status filter, "Mark shipped" | FR-5, FR-8 |
| 8 | Seller Revenue | `/seller/revenue` | Seller | Nav | Seller Revenue for a date range | FR-9 |
| 9 | Ops Dashboard | `/admin` | Admin | Nav (Admin home) | Revenue by Seller state and by Category, Order counts by status; auto-refresh | FR-10, FR-11, FR-12 |
| 10 | Vouchers | `/admin/vouchers` | Admin | Nav | Create Vouchers, list with used/unused | FR-19 |
| 11 | Payment gateway | `/admin/gateway` | Admin | Nav | Set simulated card decline rate (0–100%) | supports FR-3 / SM-2 [ASSUMPTION] |

**Nav bar:** a logo-text "Olist Marketplace" on the left, then the role links. On the right is the account menu (shows the email and role, plus "Log out"). Logged out, it shows "Products" and "Log in".

**Role routing:** after login each role goes to its home: Customer → `/`, Seller → `/seller/orders`, Admin → `/admin`. A route for another role shows "You don't have access to this page." with a link home. The gateway's refusal stays the real guard (FR-16).

**No cart** (PRD non-goal). Products are selected for Checkout with checkboxes on the Products page. The selection lives in page state and is lost on reload. [ASSUMPTION]

## Voice and Tone

The copy is plain and literal, and it names the backend fact.

| Do | Don't |
|---|---|
| "2 Orders created — one per Seller." | "Yay! Your order is on its way 🎉" |
| "Card declined. Order canceled. Voucher restored." | "Oops, something went wrong." |
| "Status: processing" | "Being prepared with love" |
| "No Orders in this range." | "Nothing to see here!" |
| Show gateway error `message` verbatim under a short summary | Hide the error code |

## Component Patterns

| Component | Used on | Behaviour |
|---|---|---|
| Product table | Products | Columns: select checkbox · name (`<Category> #<id4>`) · Category · Seller · price. Category `Select` filters (FR-13). Paginated at 20 per page with server-side paging. |
| Checkout bar | Products | Sticky bottom bar, shown when ≥ 1 Product is selected: "N items from M Sellers · R$ total · [Check out]". Logged out, "Check out" goes to Login and then returns. |
| Seller group card | Checkout review | One `Card` per Seller, listing its items and subtotal. A header line says "This will create M Orders." |
| Status badge | Everywhere an Order appears | See DESIGN.md. Always the raw Order Status word. |
| Status timeline | Order detail | Vertical list of status changes, oldest first, with timestamps (FR-7). |
| Pay form | Order detail, only when `created` | Optional Voucher code `Input` · card token `Select` with values "Test card — approves" / "Test card — declines" [ASSUMPTION; values are simulated test tokens, no card numbers] · "Pay R$ X" button. When a Voucher is entered, the form shows "Voucher R$ V · Card R$ remainder". |
| Refresh button | My Orders, Order detail, Seller Orders | Re-fetches. There is no live push (FR-7). |
| Date range filter | Seller Revenue, Ops Dashboard | Two native date inputs (from/to) on purchase time. Defaults to the full dataset range 2016-09-01 → 2018-10-31 [ASSUMPTION]. |
| Ship button | Seller Orders row | Shown only on `processing` rows. Click → "Mark shipped" confirm `Dialog` → row updates to `shipped`. |
| Dashboard table + bar cell | Ops Dashboard | Three tables: by Seller state, by Category (with "Uncategorized"), and by Order Status. Each has a total row. |
| Freshness note | Ops Dashboard | "Updated Xs ago" next to the title, refreshed by each poll. |

## State Patterns

| State | Surface | Treatment |
|---|---|---|
| Loading | Any table | 5 `Skeleton` rows |
| Empty, pre-Replay | Seller Revenue, Ops Dashboard | "No data yet. Run `make replay` to stream Historical Orders." (FR-17) |
| Empty, filtered | Any list | "No Orders in this range." / "No Products in this Category." |
| Checkout placed | Checkout review → My Orders | Toast "M Orders created — one per Seller." Redirect to My Orders with new rows on top |
| Payment pending | Order detail | After "Pay", the button is disabled with "Processing payment…". The page polls the Order every 1 s for up to 10 s until the status leaves `created` [ASSUMPTION, because the saga is async]. On timeout: "Still processing. Refresh in a moment." |
| Paid | Order detail | Toast "Payment approved." The badge moves through `approved` → `processing` on refresh |
| Declined | Order detail | Destructive toast "Card declined. Order canceled." The line "Voucher restored." is added when a Voucher was used (FR-3). The pay form is hidden |
| Duplicate submit | Pay form | The button is disabled after the first click. The client sends one idempotency key per Order per page visit (FR-4) |
| Voucher invalid/used | Pay form | Inline error under the field showing the gateway message |
| Ship refused | Seller Orders | Toast with the gateway message (e.g. Order no longer `processing`) |
| Unauthenticated / token expired | Any | Redirect to Login with "Session expired. Log in again." and return to the original route afterwards |
| Forbidden | Any | "You don't have access to this page." |
| Service down | Any | Destructive toast "Request failed: {code} — {message}". The page stays usable. This is part of the SM-1 demo |

## Interaction Primitives

- Mouse and keyboard with standard form behaviour: `Enter` submits forms and `Esc` closes dialogs.
- Every list uses pagination. There is no infinite scroll.
- The only auto-refresh is the Ops Dashboard (every 2 s, AD-17) and the short post-Pay poll. Everything else refreshes manually or on navigation.
- Confirmation dialogs are used only for "Mark shipped". Pay has none, because the button itself is the confirmation.
- Banned: drag-and-drop, modals more than one level deep, hover-only actions.

## Accessibility Floor

- shadcn defaults (Radix primitives) provide focus rings, labelled inputs and keyboard-operable menus and dialogs.
- The status badge always shows its text, never colour alone.
- Toasts use an `aria-live` region (Sonner default).
- Tab order follows reading order. Every form field has a visible `Label`.
- The target is a WCAG 2.2 AA "reasonable effort" for a demo app. No formal audit. [ASSUMPTION]

## Key Flows

### Flow 1 — Split Checkout and pay (Ana, seeded Customer, demo SM-3)

1. Ana opens `/`, filters Category "Bed Bath Table", and ticks two Products from Seller A. She switches the filter to "Health Beauty" and ticks one Product from Seller B.
2. The Checkout bar reads "3 items from 2 Sellers". She clicks "Check out" and logs in with a demo account from the Login hint.
3. Checkout review shows two Seller cards and "This will create 2 Orders." She clicks "Place Checkout".
4. My Orders shows two new rows, both `created`.
5. She opens the first Order, picks "Test card — approves", and clicks "Pay R$ 129,90".
6. **Climax:** the button shows "Processing payment…" and then the toast says "Payment approved." On refresh the timeline shows `created → approved → processing`, and the second Order is still `created`. The builder opens Jaeger next to it and finds one trace across ≥ 4 services.

### Flow 2 — Voucher + declined card (the builder as Admin, then Customer; demo SM-2)

1. The builder logs in as Admin. On Vouchers they create a R$ 30 Voucher and copy its code. On Payment gateway they set the decline rate to 100%.
2. They log out and log in as Ana. They check out one Product and open the Order.
3. They enter the Voucher code (the form shows "Voucher R$ 30,00 · Card R$ 99,90") and click Pay.
4. **Climax:** the destructive toast says "Card declined. Order canceled. Voucher restored." The badge reads `canceled`. On a second Order the same Voucher code is accepted again.

### Flow 3 — Seller ships, delivery completes (Bruno, seeded Seller)

1. Bruno logs in and lands on Seller Orders, where his Historical Orders are already listed (FR-15).
2. He filters to `processing`. Ana's paid Order is there.
3. He clicks "Mark shipped" and confirms. The row shows `shipped`.
4. **Climax:** about 3 seconds later he clicks Refresh and the row reads `delivered`. On Seller Revenue the total for today's range has grown by the Order's item price minus any Voucher.

### Flow 4 — Watching Replay (the builder as Admin; demo SM-4)

1. The builder opens the Ops Dashboard. It shows "No data yet. Run `make replay`…".
2. In a terminal they run `make replay` with a 2017 range.
3. **Climax:** without a reload, the "Orders by status" counts and the revenue tables grow every 2 seconds, and "Updated 1s ago" stays fresh. The totals row always equals the sum of its rows.

## Open Items

- [ASSUMPTION] shadcn/ui + Tailwind as the UI system (the architecture only fixes React + Vite).
- [ASSUMPTION] Products are selected with checkboxes instead of a cart.
- [ASSUMPTION] The Admin "Payment gateway" screen exists to set the decline rate. Without it, SM-2 has to be driven through a CLI or env var.
- [ASSUMPTION] Card tokens are picked from two simulated test values. No card-number input.
- [ASSUMPTION] Polling after Pay: every 1 s, for up to 10 s.
- [ASSUMPTION] Demo account credentials are shown on the Login page. This is local-only; the seeding count is open (PRD OQ-2).
