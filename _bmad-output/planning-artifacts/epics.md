---
status: final
stepsCompleted: [1, 2, 3, 4]
inputDocuments:
  - _bmad-output/planning-artifacts/prds/prd-olist-marketplace-2026-10-29/prd.md
  - _bmad-output/planning-artifacts/prds/prd-olist-marketplace-2026-10-29/addendum.md
  - _bmad-output/planning-artifacts/architecture/architecture-olist-marketplace-2026-10-30/ARCHITECTURE-SPINE.md
  - _bmad-output/specs/spec-olist-marketplace/SPEC.md
  - _bmad-output/specs/spec-olist-marketplace/glossary.md
  - _bmad-output/planning-artifacts/ux-designs/ux-olist-marketplace-2026-10-30/DESIGN.md
  - _bmad-output/planning-artifacts/ux-designs/ux-olist-marketplace-2026-10-30/EXPERIENCE.md
---

# olist-marketplace - Epic Breakdown

## Overview

This document provides the complete epic and story breakdown for olist-marketplace, decomposing the requirements from the PRD, UX Design if it exists, and Architecture requirements into implementable stories.

## Requirements Inventory

### Functional Requirements

FR1: A Customer can check out Products from several Sellers in one action; Checkout creates exactly one Order per Seller, each containing only that Seller's Order Items and starting in `created`.
FR2: A Customer can pay one `created` Order with a Card Payment, a Voucher, or a Voucher plus a Card Payment for the remainder. An Order whose Payments cover its total moves to `approved`; a used Voucher cannot be applied again; paying one Order does not change the status of the other Orders from the same Checkout. (Out of scope: installments, boleto, one payment covering several Orders.)
FR3: When a Card Payment is declined, the system cancels the Order and restores any Voucher applied to it. With the gateway set to decline 100%, a Voucher + card Order ends `canceled` and the Voucher is usable again; no Order stays `created` after a decline.
FR4: A repeated payment request for the same Order (same idempotency key) charges at most once and yields one Payment record.
FR5: The system moves every `approved` Order to `processing` automatically (never stays `approved`). A Seller can mark one of their own `processing` Orders as `shipped`; a Seller cannot ship another Seller's Order or an Order that is not `processing`.
FR6: The system moves a `shipped` Order to `delivered` automatically: a simulated courier delivers it after `SHIPPING_DELAY` (default 3 seconds, configurable), and the Order shows `delivered` within about 1 second more.
FR7: A Customer can see each of their own Orders with its Order Status and timeline of status changes. Changes are visible after a page refresh (no live push). A Customer sees only their own Orders. (Out of scope: estimated delivery dates.)
FR8: A Seller can list their own Orders and filter by Order Status. A Seller never sees another Seller's Orders; for a multi-Seller Historical Order, a Seller sees only their own Order Items.
FR9: A Seller can see Seller Revenue for a from/to date range on purchase time: for `delivered` Orders only, the sum of the Seller's Order Item `price` (freight excluded) minus the Voucher value applied, floored at 0 per Order. `canceled`/undelivered contribute 0. Historical Orders ignore Vouchers (full price). Example: price 100 with a 30 Voucher adds 70.
FR10: An Admin can see Seller Revenue grouped by Seller state for a date range; the total across all states equals total Seller Revenue for the range.
FR11: An Admin can see Seller Revenue grouped by Category (English name) for the same date range; Products without a Category are grouped as "Uncategorized".
FR12: An Admin can see Order counts per Order Status for the date range; counts sum to the total Orders in range and change during Replay without a page reload (auto-refresh is enough).
FR13: A visitor can list Products and filter by Category. Each Product shows display name `<Category English name> #<first 4 chars of product id>` and a price equal to its most recent dataset Order Item `price`.
FR14: A visitor can register a Customer Account that can log in immediately. Seller and Admin Accounts cannot be created by registration.
FR15: The system provides seeded demo Accounts for a sample of dataset Customers and Sellers, plus exactly one Admin Account. Logging in as a seeded Seller shows that Seller's Historical Orders in FR8.
FR16: Role-based access: Customer → Checkout, payment, own Orders; Seller → own Orders and Seller Revenue; Admin → Ops Dashboard and Vouchers. A Customer calling a Seller or Admin function is refused.
FR17: The builder can load Products, Categories, Sellers and Customers from the Olist CSVs with one command. Running Import twice creates no duplicates. Import loads no Historical Orders; before the first Replay, Seller Revenue and the Ops Dashboard are empty.
FR18: The builder can run Replay with `make replay`, separately from the services, publishing each Historical Order's status changes as events in purchase-time order. Start/end dates are selectable; default speed is 1 day of data per 1 second; Seller Revenue and Ops Dashboard update while it runs; stopping and restarting does not double-count.
FR19: An Admin can create fixed-value Vouchers. Each has a unique code and starts unused; its value cannot change after use; it cannot be partially used.

### NonFunctional Requirements

NFR1: Reliability — no lost Orders. If any service is down while Orders are placed, every Order still leaves `created` (to `processing` or `canceled`) once it is back. No event is lost between a database write and its publication. (Demo SM-1)
NFR2: Idempotency — every event consumer tolerates duplicate delivery with no extra effect (no double charge, revenue, or count).
NFR3: Observability — one Checkout/Pay produces one distributed trace across ≥ 4 services; logs carry the trace id; each service exposes health and request metrics. (Demo SM-3)
NFR4: Read-model freshness — during Replay at default speed, the Ops Dashboard is ≤ 5 seconds behind published events. (Demo SM-4)
NFR5: Data ownership — each service owns its data and reads no other service's database; cross-service data only via APIs or events.
NFR6: One-command local run — the whole system, including Kafka and databases, starts locally with one command.
NFR7: Basic security — passwords stored hashed; login tokens expire; all functions except browsing require a valid token.

### Additional Requirements

**Starter / existing scaffold (affects Epic 1 Story 1):** No external starter template. The repo already has a Go workspace scaffold (`go.work`, `services/*` with `cmd/server` + hexagonal `internal/{domain,service,repository,transport}`, `pkg/{eventbus,logger,tracing}`, `proto/{catalog,order}/v1`, `tools/olist-importer`, `deployments/docker-compose.yml`, `Makefile`, CI workflow). It must be aligned to the architecture:
- Rename `customer-service` → `identity-service`; remove `review-service` (AD-1); add `shipping-simulator` service; update `Makefile` SERVICES list and compose.
- Add `web/` (React 19 + Vite 8 SPA), `contracts/events/v1/`, `pkg/{outbox,inbox}`, `proto/<service>/v1` for every service with generated code in `proto/gen/go`, checked by `buf` lint + breaking-change.

Architecture rules that shape implementation:
- AD-1 Service ownership: identity (Account, credentials, signing keys), catalog (Product incl. derived name/price/freight, Category, Seller profile incl. state), order (Order, Order Item, status timeline, saga state, payment requests), payment (Payment, Voucher, gateway settings), shipping-simulator (Shipment), analytics (read models only), api-gateway (no data).
- AD-2 One Postgres 18 instance, one database + one DB user per service with no cross-grants; per-service `migrations/` applied at startup via golang-migrate; no tool runs SQL against a service DB.
- AD-3 Communication matrix: Browser→gateway REST/JSON; gateway→services gRPC; order→catalog gRPC at Checkout only; all other service-to-service traffic via Kafka (`payment.commands`, `payment.events`, `order.events`, `shipping.events`, `replay.orders`); importer→catalog/identity via admin gRPC.
- AD-4 Order Saga orchestrated by order-service: Checkout does not start the saga; `OrderService.PayOrder` is the only pay entry (ownership + `created` check → `AuthorizePayment` via outbox); payment-service has no pay gRPC, only a read-only Customer `LookupVoucher`; Voucher redeemed with one conditional UPDATE (`VOUCHER_UNAVAILABLE` on 0 rows); on card decline payment-service restores the Voucher and emits `PaymentFailed{reason, voucher_restored}`; any `PaymentFailed` → `canceled` with `cancel_reason` stored.
- AD-5 Only order-service writes Order Status; allowed transitions `created→approved→processing→shipped→delivered` and `created→canceled`; Historical first event may be any status, later must be forward-reachable; unpaid `created` Orders never expire; every transition stored in a timeline with business timestamp.
- AD-6 Transactional outbox in every publisher: state + outbox row (`bigserial seq`, `traceparent`) in one transaction; one relay goroutine per service publishes in `seq` order (`FOR UPDATE SKIP LOCKED` only as a single-instance guard); idempotent producer, `acks=all`, franz-go ordered mode; relay poll ≤ 200 ms.
- AD-7 Inbox (`processed_events`) in every consumer; four handler outcomes `applied / duplicate / rejected / retryable` with DLQ `<topic>.dlq` after N retries; projectors upsert per `(order_id, status)` never below 0; `payment_requests(idempotency_key PK, order_id, request_hash)` with `ALREADY_EXISTS` / `FAILED_PRECONDITION`; one Payment row per Order (`UNIQUE(order_id)`) for every outcome; Voucher redemption only via unique `used_by_order`; undecodable messages `rejected`; DLQ depth metric + `make dlq-redrive`; new consumer groups start at earliest; Checkout carries `checkout_idempotency_key` scoped per Customer with a request hash, all Orders + outbox rows in one transaction sharing `checkout_id`.
- AD-8 Topics: facts on `<producer>.events`, commands on `<target>.commands`; partition key `order_id`; JSON envelope `event_id, event_type, event_version, aggregate_id, historical, occurred_at, published_at, origin_published_at, traceparent, payload`; `traceparent` also as Kafka header; consumers start child spans before any filtering.
- AD-9 Event catalog in `contracts/events/v1` (Go structs + JSON Schema via `pkg/eventbus`) is the only definition: `OrderCreated/Approved/Processing/Shipped/Delivered/Canceled` (with `from_status`, `to_status`, snapshot; approved→processing emits both in order), `AuthorizePayment`, `PaymentSucceeded{voucher_amount}`, `PaymentFailed{reason}`, `ShipmentDelivered`, `HistoricalOrderStatusRecorded`; minor bump to add a field, major to rename/remove.
- AD-10 Every `Order*` fact carries the full snapshot (order fields + `items[{product_id, seller_id, seller_state, category_en, price, freight}]`, "Uncategorized" when no Category); analytics builds projections from events only.
- AD-11 Query ownership: order-service serves Order lists/timelines; analytics-service serves Seller Revenue and dashboard; revenue per Order = `max(0, Σprice − voucher_amount)` new, `Σprice` Historical, `delivered` only.
- AD-12 Import via owners' idempotent admin gRPC (`catalog.ImportProducts/Sellers/Categories`, `identity.ImportAccounts`), catalog derives name/price/freight; replayer emits enriched `HistoricalOrderStatusRecorded` per derived status, sorted and paced by `--from/--to/speed`; dataset status mapping (purchase→created, approved_at→approved, invoiced/processing→processing at approved_at, delivered_carrier_date→shipped, delivered_customer_date→delivered, canceled/unavailable→canceled at last timestamp; unusable rows skipped and counted in a replay report); `event_id = UUIDv5(NS_OLIST_REPLAY, order_id:status)`; order-service ingests with `historical=true`, bypasses saga, republishes via outbox reusing `event_id`.
- AD-13 Process reactors (payment, shipping-simulator) drop `historical=true` envelopes before decoding.
- AD-14 identity-service signs RS256 JWTs (1 h, no refresh, claims `sub, role, kid`), serves JWKS; bcrypt passwords; gateway verifies token + role and forwards JWT in gRPC metadata; each service re-verifies via cached JWKS and applies ownership by role (customer → `customer_id`, seller → `item.seller_id`); `UNAUTHENTICATED` without token; service gRPC ports not published; seeded Accounts use dataset ids, new ones UUIDv7.
- AD-15 Checkout snapshots `price, freight, seller_id, seller_state, category_en` from catalog into each Order Item; `order_total = Σ(price + freight)`; `voucher_amount = min(face, order_total)`; card charged `order_total − voucher_amount`; money in int64 centavos.
- AD-16 shipping-simulator persists `shipments(order_id PK, deliver_at, delivered)` with inbox/outbox; poller (≤ 500 ms) emits `ShipmentDelivered` when due; delay from env `SHIPPING_DELAY` (default 3s, simulated courier); `delivered` within `SHIPPING_DELAY` + 1 s.
- AD-17 Freshness budget: relay poll ≤ 200 ms, analytics batch commit ≤ 500 ms, dashboard refresh ≤ 2 s, analytics lag histogram (`applied_at − origin_published_at`, 0 when idle).

Conventions and infrastructure:
- IDs `uuid` everywhere (Olist 32-hex stored as uuid, new ids UUIDv7); money int64 centavos; time `timestamptz` UTC (São Paulo times converted at import/replay).
- Errors: gRPC status codes between services; gateway returns `{ "error": { "code", "message" } }`.
- Logging `log/slog` JSON to stdout with trace id; OpenTelemetry SDK in every service, OTLP to Jaeger; `traceparent` through outbox and Kafka headers.
- Each service exposes `/healthz`, `/readyz` (DB + Kafka), `/metrics` on one ops port; Prometheus + Grafana.
- Config via env vars; payment decline rate also settable via admin gRPC.
- Local runtime: one `docker compose` with all services, one Postgres 18, one Kafka 4.3 (KRaft), Jaeger, Prometheus, Grafana, kafka-ui; only gateway, web and observability UIs publish ports.
- Stack versions: Go 1.27, Postgres 18, Kafka 4.3, franz-go 1.22 (+kotel), pgx v5, golang-migrate 4.20, golang-jwt v5, bcrypt, OTel Go 1.47, Jaeger 2.22, Prometheus 3, Grafana 13, React 19, Vite 8, kafka-ui 1.5.
- Kubernetes manifests in week 8 (deferred until then). Debezium CDC and OTel Collector deferred.
- Open item decided in epics: number of seeded demo Accounts and how their passwords are shared.

### UX Design Requirements

UX-DR1: Set up `web/` with shadcn/ui + Tailwind, inheriting shadcn defaults unchanged (no brand theme, custom fonts or imagery); top nav bar with single content column capped at 1152px; desktop-only target.
UX-DR2: Status badge component — pill filled with the per-status color token (`created` slate-500, `approved` blue-600, `processing` amber-600, `shipped` violet-600, `delivered` green-600, `canceled` red-600) with white text showing the exact dataset status word; never color alone.
UX-DR3: Money component — numeric monospace, right-aligned, formatted BRL `R$ 1.234,56` from int64 centavos; counts use the same numeric style.
UX-DR4: Bar cell component for dashboard tables — thin horizontal `muted-foreground` bar behind a number, width = share of column max; no chart library.
UX-DR5: App shell and role routing — nav "Olist Marketplace" + role-scoped links + account menu (email, role, Log out); logged-out shows Products and Log in; after login route to role home (Customer `/`, Seller `/seller/orders`, Admin `/admin`); other-role routes show "You don't have access to this page." with link home.
UX-DR6: Auth states — on 401/expired token, redirect to Login with "Session expired. Log in again." and return to the original route after login.
UX-DR7: Login page (`/login`) with a "Demo accounts" hint listing seeded credentials; Register page (`/register`) linked from Login, creating Customer Accounts only.
UX-DR8: Products page (`/`) — table with select checkbox, display name, Category, Seller, price; Category `Select` filter; server-side pagination 20/page; empty-filter message "No Products in this Category."
UX-DR9: Checkout bar — sticky bottom bar when ≥ 1 Product selected: "N items from M Sellers · R$ total · [Check out]"; selection in page state mirrored to `sessionStorage` (no server-side cart); logged out → Login then return with the selection kept.
UX-DR10: Checkout review (`/checkout`) — one Card per Seller with items and subtotal, header "This will create M Orders.", "Place Checkout" button; on success toast "M Orders created — one per Seller." and redirect to My Orders with new rows on top.
UX-DR11: My Orders (`/orders`) — list of own Orders with status badge, total and date; Refresh button; pagination; no live push.
UX-DR12: Order detail (`/orders/:id`) — items, status timeline (oldest first with timestamps), Refresh button.
UX-DR13: Pay form on Order detail, only when `created` — optional Voucher code input, card token Select ("Test card — approves" / "Test card — declines", simulated tokens, no card numbers), "Pay R$ X" button; shows "Voucher R$ V · Card R$ remainder" split when a Voucher is entered, using a Customer Voucher lookup; one idempotency key per Order per page visit; button disabled after first click.
UX-DR14: Payment states — "Processing payment…" while polling the Order every 1 s up to 10 s; success toast "Payment approved."; cancel destructive toast by reason ("Card declined. Order canceled." or "Voucher unavailable. Order canceled.") plus "Voucher restored." only when it was restored, pay form hidden; timeout "Still processing. Refresh in a moment."; Voucher unknown/used (from the lookup, before Pay) shown inline under the field with the gateway message.
UX-DR15: Seller Orders (`/seller/orders`) — own Orders (own Order Items only for Historical), Order Status filter, pagination, Refresh; "Mark shipped" only on `processing` rows with a confirm Dialog, row updates to `shipped`; refusal shows a toast with gateway message.
UX-DR16: Seller Revenue (`/seller/revenue`) — from/to native date inputs (default 2016-09-01 → today) and Seller Revenue total; pre-Replay empty state "No data yet. Run `make replay` to stream Historical Orders."
UX-DR17: Ops Dashboard (`/admin`) — date range filter (default 2016-09-01 → today); three tables (revenue by Seller state, revenue by Category incl. "Uncategorized", Order counts by status) each with a total row and bar cells; auto-refresh every 2 s; "Updated Xs ago" freshness note; pre-Replay empty state.
UX-DR18: Vouchers (`/admin/vouchers`) — create form (fixed value), list with code, value and used/unused.
UX-DR19: Payment gateway (`/admin/gateway`) — set simulated card decline rate 0–100% (drives the SM-2 demo).
UX-DR20: Global state patterns — `Skeleton` rows (5) while loading; destructive toast "Request failed: {code} — {message}" when a service is down while the page stays usable; plain literal microcopy naming backend facts; gateway error messages shown verbatim.
UX-DR21: Interaction/accessibility floor — pagination everywhere (no infinite scroll), confirm dialog only for Mark shipped, no drag-and-drop, no hover-only actions, max one modal level; visible labels on every field, tab order follows reading order, toasts in `aria-live` region, WCAG 2.2 AA reasonable effort.

### FR Coverage Map

FR1: Epic 2 - Checkout splits into one `created` Order per Seller
FR2: Epic 2 - Pay an Order by card, Voucher, or Voucher + card
FR3: Epic 2 - Declined card cancels the Order and restores the Voucher
FR4: Epic 2 - Idempotent payment request, no double charge
FR5: Epic 2 (auto `approved→processing`, Story 2.8) + Epic 3 (Seller marks own Order shipped)
FR6: Epic 3 - Simulated delivery `shipped→delivered` (default 3 s)
FR7: Epic 2 - Customer sees own Orders and status timeline
FR8: Epic 3 - Seller order list with status filter (Historical items in Epic 4)
FR9: Epic 5 - Seller Revenue for a date range
FR10: Epic 5 - Revenue by Seller state
FR11: Epic 5 - Revenue by Category incl. "Uncategorized"
FR12: Epic 5 - Order counts by status, auto-refreshing during Replay
FR13: Epic 1 - Browse and filter Products with derived name and price
FR14: Epic 1 - Register a Customer Account
FR15: Epic 1 - Seeded demo Accounts (Seller history check verified in Epic 4)
FR16: Epic 1 - Role-based access
FR17: Epic 1 - Import reference data, idempotent
FR18: Epic 4 - Replay Historical Orders with `make replay`
FR19: Epic 2 - Admin creates Vouchers

NFR coverage: NFR1 → Epic 2 · NFR2 → Epics 2, 4 · NFR3 → Epic 1 (baseline), Epic 2 (trace demo) · NFR4 → Epic 5 · NFR5, NFR6, NFR7 → Epic 1.
UX-DR coverage: Epic 1 → 1, 3, 5, 6, 7, 8, 20, 21 · Epic 2 → 2, 9, 10, 11, 12, 13, 14, 18, 19 · Epic 3 → 15 · Epic 5 → 4, 16, 17.

Decisions taken in epics: seeded demo Accounts = 20 Customers + 20 Sellers + 1 Admin sharing one demo password, emails in the reserved `@demo.olist.local` domain, all listed in the README with a short sample in the Login hint. Kubernetes manifests = last story of Epic 5.

Decisions from the 2026-10-31 review (applied to the PRD (FR-6), ARCHITECTURE-SPINE.md (AD-4, AD-7, AD-8, AD-9, AD-12, AD-16, AD-17) and EXPERIENCE.md on the same date):
- Voucher check before Pay: new Customer RPC `payment.LookupVoucher(code)` via `GET /api/vouchers/:code` (gateway→service gRPC, so AD-3 holds). It drives the split display and the inline error; `PayOrder` stays asynchronous, and a Voucher used in between still cancels with `VOUCHER_UNAVAILABLE` (Stories 2.6, 2.9, 2.10; UX-DR13, UX-DR14).
- `PaymentFailed` carries `voucher_restored`; the Order stores `cancel_reason` and `voucher_restored`, and the UI picks its message from them (Stories 2.3, 2.4, 2.9, 2.10).
- Voucher redemption state is only `used_by_order` (unique where not null); there is no redemption table. Voucher value is immutable. One Payment row per Order for every outcome (Stories 2.6, 2.8).
- Checkout idempotency keys are scoped per Customer with a request hash; duplicate product ids are rejected (Story 2.3).
- Product Seller, price and freight come from one deterministic "most recent Order Item" row; missing category translations fall back to the Portuguese name; "Uncategorized" is a real, filterable Category (Story 1.3, 1.8).
- Checkout selection is mirrored to `sessionStorage` to survive the login redirect (UX-DR9, Story 2.5).
- Historical Orders cannot be shipped by a Seller (Story 3.2).
- FR6's 3 s is the simulated courier delay (`SHIPPING_DELAY`), chosen so demos reach `delivered` quickly; the AC allows `SHIPPING_DELAY` + 1 s end to end (Story 3.3).
- The envelope gains `origin_published_at`; analytics lag (AD-17, NFR4) is `applied_at − origin_published_at`, 0 when idle (Stories 2.1, 5.1, 5.4).
- DLQ depth metric plus `make dlq-redrive`; demo scripts fail on a non-empty DLQ and use deadlines (Stories 2.2, 2.11, 3.4, 5.4); demo scripts support `RUNTIME=compose|k8s` (Story 5.5).
- Replay: dates are America/Sao_Paulo and inclusive, default is the full dataset; all events of in-range Orders are published; canceled Orders emit `created → canceled` only (Story 4.1).
- Revenue and dashboard date ranges default to 2016-09-01 → today (UX-DR16, UX-DR17, Stories 5.2, 5.3).
- `Register` returns a login token; the `web` container is added in Story 1.7, not 1.1.

## Epic List

### Epic 1: Browse the real Olist catalog and sign in
A visitor browses real Olist Products by Category, registers as a Customer or logs in with a seeded Customer, Seller or Admin Account, and each role sees only its own navigation. The repo scaffold is aligned to the architecture and the whole system starts with one command.
**FRs covered:** FR13, FR14, FR15, FR16, FR17

### Epic 2: Checkout and pay, with a saga that survives failure
A Customer checks out from several Sellers, pays each Order by card, Voucher or both, and follows its status. A declined card cancels the Order and returns the Voucher; a payment-service outage loses nothing. The Admin creates Vouchers and sets the card decline rate.
**FRs covered:** FR1, FR2, FR3, FR4, FR7, FR19

### Epic 3: Seller fulfilment and simulated delivery
A Seller lists and filters their own Orders and marks `processing` Orders shipped; a durable simulator delivers them and the Customer sees it in the timeline.
**FRs covered:** FR5, FR6, FR8

### Epic 4: Replay real Olist history
The builder runs `make replay` over a date range and Historical Orders stream in as if live; a seeded Seller sees their real order history.
**FRs covered:** FR18 (plus FR8 and FR15 Historical checks)

### Epic 5: Seller Revenue and the live Ops Dashboard
A Seller sees Seller Revenue for a date range; an Admin watches revenue by Seller state and by Category and Order counts by status update live during Replay. Ends demo-ready with Kubernetes manifests.
**FRs covered:** FR9, FR10, FR11, FR12

## Epic 1: Browse the real Olist catalog and sign in

A visitor browses real Olist Products by Category, registers as a Customer or logs in with a seeded Customer, Seller or Admin Account, and each role sees only its own navigation. The repo scaffold is aligned to the architecture and the whole system starts with one command.

### Story 1.1: Align the repo scaffold and start the platform with one command

As the builder,
I want the existing scaffold reshaped to the architecture's service set and started by one command,
So that every later story lands in the right service and I can run the whole system locally.

**Acceptance Criteria:**

**Given** the current scaffold with `customer-service` and `review-service`
**When** the story is done
**Then** `customer-service` is renamed `identity-service`, `review-service` is removed, and `shipping-simulator` exists with the same hexagonal layout (`cmd/server`, `internal/{domain,service,repository,transport}`, `migrations/`) (AD-1)
**And** `go.work`, the `Makefile` `SERVICES` list, the CI workflow and `deployments/docker-compose.yml` reference exactly: api-gateway, identity-service, catalog-service, order-service, payment-service, shipping-simulator, analytics-service

**Given** a clean checkout with Docker running
**When** the builder runs `make up`
**Then** one `docker compose` brings up all seven services, one Postgres 18, one Kafka 4.3 broker (KRaft), Jaeger, Prometheus, Grafana and kafka-ui on one internal network (NFR6)
**And** only api-gateway and the observability UIs publish host ports; service gRPC ports and Postgres are not published (AD-14); the `web` container and its port are added in Story 1.7

**Given** Postgres starts
**When** its init script runs
**Then** each data-owning service has its own database and DB user with no grants on any other database (AD-2)
**And** each service applies its own `migrations/` at startup with golang-migrate (embedded via iofs), even when the folder is empty

**Given** the repo layout
**When** the story is done
**Then** `contracts/events/v1/`, `pkg/{outbox,inbox}/`, `proto/<service>/v1/` for every gRPC service and `proto/gen/go` exist, and `make proto` runs `buf generate` with `buf lint` and `buf breaking` wired into CI
**And** `make build` and `make test` pass for every service

### Story 1.2: Observability baseline in every service

As the builder,
I want every service to log, trace and expose health and metrics the same way,
So that later stories inherit tracing and I can debug the system from day one (NFR3).

**Acceptance Criteria:**

**Given** `pkg/logger` and `pkg/tracing`
**When** any service starts
**Then** it logs with `log/slog` as JSON to stdout, and every log line written inside a request or span carries the trace id
**And** it initialises the OpenTelemetry SDK exporting OTLP to Jaeger, with the service name set

**Given** a running service
**When** its ops port is called
**Then** `/healthz` returns 200 while the process is alive, `/readyz` returns 200 only when its DB (and Kafka, for services that use it) are reachable, and `/metrics` exposes Prometheus request count and latency metrics

**Given** `make up`
**When** the builder opens Prometheus and Grafana
**Then** Prometheus scrapes every service's `/metrics` and Grafana has Prometheus and Jaeger provisioned as data sources

**Given** gRPC servers and clients built from shared helpers
**When** api-gateway calls a service's standard gRPC health check (`grpc.health.v1`) on startup or via a debug route
**Then** the call shows in Jaeger as one trace with a gateway span and a child service span

### Story 1.3: Import catalog reference data

As the builder,
I want to load Categories, Sellers and Products from the Olist CSVs with one command,
So that the catalog holds real data with derived display names and prices (FR17, FR13).

**Acceptance Criteria:**

**Given** catalog-service
**When** this story is done
**Then** it owns migrations for Category (Portuguese name, English name), Seller profile (seller id, city, state) and Product (product id, category, display name, price, freight, seller id), with money as int64 centavos and Olist ids stored as uuid (AD-1, Conventions)
**And** it exposes admin gRPC `ImportCategories`, `ImportSellers`, `ImportProducts` as idempotent upserts keyed by dataset id (AD-12)

**Given** the CSVs in `data/`
**When** the builder runs `make import`
**Then** `tools/olist-importer` reads the CSVs and calls only catalog's admin gRPC (no SQL against any service DB) (AD-2, AD-12)
**And** each Product's price, freight and Seller come from one row: its most recent dataset Order Item, chosen by `ORDER BY order_purchase_timestamp DESC, order_id DESC, order_item_id DESC LIMIT 1`, so re-imports pick the same row; and its display name is `<Category English name> #<first 4 chars of product id>` (FR13)
**And** a Product with no Category gets category "Uncategorized", which is a real Category row (English and Portuguese name "Uncategorized")
**And** a Portuguese category with no row in the translation CSV gets its Portuguese name as its English name, so no display name starts with an empty Category

**Given** Import has already run
**When** the builder runs `make import` again
**Then** row counts are unchanged and no duplicates exist (FR17)
**And** no Order or Order Item is loaded by Import (FR17)

### Story 1.4: Register and log in

As a visitor,
I want to register a Customer Account and log in,
So that I can act as a Customer (FR14, NFR7).

**Acceptance Criteria:**

**Given** identity-service
**When** this story is done
**Then** it owns an Account table (id, email, bcrypt password hash, role, created_at) and an RS256 signing key, and serves `GET /.well-known/jwks.json` with the key's `kid` (AD-14)

**Given** a visitor with a new email
**When** they register through gRPC `Register`
**Then** a Customer Account is created with a UUIDv7 id and the password stored as a bcrypt hash only
**And** the response is the same token response as `Login`, so the new Customer is logged in immediately (FR14)
**And** emails are stored trimmed and lower-cased with a unique index on the normalized email, so registering an existing email in any letter case returns `ALREADY_EXISTS`

**Given** a registration with an empty or malformed email, a password shorter than 8 characters or longer than 72 bytes (bcrypt's limit), or an email in the reserved seed domain `@demo.olist.local`
**When** it is processed
**Then** it returns `INVALID_ARGUMENT` and creates nothing

**Given** a registration request that names role Seller or Admin
**When** it is processed
**Then** it returns `INVALID_ARGUMENT`; no Seller or Admin can be created by registration (FR14)

**Given** a registered Account
**When** it logs in with the correct password
**Then** it receives a JWT signed RS256 with claims `sub`, `role`, `kid` that expires after 1 hour, with no refresh token (AD-14)
**And** a wrong password returns `UNAUTHENTICATED` without revealing whether the email exists

### Story 1.5: Seed demo Accounts

As the builder,
I want ready-made Customer, Seller and Admin Accounts from the dataset,
So that I can demo every role without registering (FR15).

**Acceptance Criteria:**

**Given** identity-service exposes admin gRPC `ImportAccounts` (idempotent upsert keyed by dataset id)
**When** the builder runs `make import`
**Then** 20 dataset Customers (id = `customer_unique_id`), 20 dataset Sellers (id = `seller_id`) and exactly 1 Admin Account are created (AD-14)
**And** all share one demo password, set by env with a documented default
**And** the 20 Sellers are the Sellers with the most Order Items (`ORDER BY item_count DESC, seller_id ASC`), and the 20 Customers are the `customer_unique_id`s with the most Orders (`ORDER BY order_count DESC, customer_unique_id ASC`), so both have rich history and the choice is the same on every run
**And** emails follow a fixed pattern in the reserved domain: `customer-<first 8 chars of id>@demo.olist.local`, `seller-<first 8 chars of id>@demo.olist.local`, `admin@demo.olist.local`
**And** the Admin's id is a fixed UUID constant in `contracts`, so its upsert is idempotent like the others

**Given** the seeded Accounts
**When** the builder reads the README
**Then** it lists every demo email per role and the demo password; the Login hint shows only two per role plus the Admin

**Given** Import runs twice
**When** Accounts are counted
**Then** there are still 41 seeded Accounts and exactly one Admin

### Story 1.6: Gateway authentication and role-based access

As the builder,
I want the gateway and every service to verify tokens and enforce roles,
So that each role reaches only its own functions (FR16, NFR7).

**Acceptance Criteria:**

**Given** api-gateway exposes REST/JSON for login, register and public browsing
**When** a request hits a non-public route without a valid, unexpired JWT
**Then** the gateway returns 401 with `{ "error": { "code", "message" } }` (Conventions)

**Given** the route groups Customer (`/api/checkout`, `/api/orders/*`, `/api/vouchers/:code`), Seller (`/api/seller/*`) and Admin (`/api/admin/*`)
**When** a valid JWT calls a route group of another role (every role × every other role's group)
**Then** the gateway returns 403 and does not call the service (FR16)
**And** each service also refuses the RPC with `PERMISSION_DENIED` for a role it does not serve, so Seller or Admin tokens can never place a Checkout or pay an Order
**And** a table-driven test covers all three roles against all three route groups at both the gateway and the service

**Given** an authorized request
**When** the gateway calls a service over gRPC
**Then** it forwards the original JWT in gRPC metadata, and the service re-verifies it against identity's JWKS, cached and refreshed on an unknown `kid` (AD-14)
**And** JWKS refreshes on unknown `kid` are rate-limited (at most once per minute); a token with an unknown `kid` after a refresh returns `UNAUTHENTICATED`, and when identity-service is unreachable the last good keys stay in use
**And** a non-public RPC called without a valid token returns `UNAUTHENTICATED`

**Given** a shared auth helper in `pkg`
**When** a service handles a request
**Then** it can read the caller's account id and role to apply ownership predicates (customer → `customer_id`, seller → `seller_id`)

**Given** a gRPC error from a service
**When** the gateway maps it
**Then** status codes map to HTTP consistently (e.g. `INVALID_ARGUMENT`→400, `UNAUTHENTICATED`→401, `PERMISSION_DENIED`→403, `NOT_FOUND`→404, `ALREADY_EXISTS`/`FAILED_PRECONDITION`→409, `RESOURCE_EXHAUSTED`→429, `UNAVAILABLE`→503, `DEADLINE_EXCEEDED`→504) with the `{ "error": … }` body
**And** any other code (`INTERNAL`, `UNKNOWN`, …) maps to 500 with the same body shape

### Story 1.7: Web app shell, login and register

As a visitor,
I want a simple web app where I can log in, register and see my role's navigation,
So that I can reach the screens for my role (UX-DR1, 3, 5, 6, 7, 20, 21).

**Acceptance Criteria:**

**Given** `web/` built with React 19, Vite 8, Tailwind and shadcn/ui defaults
**When** `make up` runs
**Then** the `web` container is in compose, publishes a host port, and the web app is served and talks only to api-gateway (UX-DR1)
**And** the layout is a top nav bar plus one content column capped at 1152px

**Given** a logged-out visitor
**When** they open the app
**Then** the nav shows "Olist Marketplace", "Products" and "Log in" (UX-DR5)

**Given** the Login page `/login`
**When** it renders
**Then** it shows email and password fields with visible labels, a "Demo accounts" hint listing two seeded emails per role plus the Admin and the demo password (full list in the README), and a link to `/register` (UX-DR7)
**And** a successful login routes Customer → `/`, Seller → `/seller/orders`, Admin → `/admin`, and the nav shows that role's links plus an account menu with email, role and "Log out"
**And** until a role's screens are built by later stories, its home route renders a minimal page with the page title and nav, so every route a login can land on exists

**Given** the Register page `/register`
**When** a visitor submits a new email and password
**Then** a Customer Account is created and the visitor is logged in with the token returned by `Register`

**Given** a logged-in user
**When** a request returns 401 or the token has expired
**Then** they are sent to Login with "Session expired. Log in again." and returned to the original route after logging in (UX-DR6)
**And** the return-to route is accepted only as a same-origin path starting with a single `/`; anything else, or a route belonging to a role other than the one now logged in, goes to that role's home
**And** opening another role's route shows "You don't have access to this page." with a link home

**Given** shared UI patterns
**When** any page loads data or a request fails
**Then** tables show 5 `Skeleton` rows while loading, failures show a destructive toast "Request failed: {code} — {message}" using the gateway message verbatim, and the page stays usable (UX-DR20)
**And** a shared Money component renders int64 centavos as right-aligned monospace `R$ 1.234,56` (UX-DR3)
**And** every field has a visible label, toasts use an `aria-live` region, and there is no infinite scroll or hover-only action (UX-DR21)

### Story 1.8: Browse and filter Products

As a visitor,
I want to list Products and filter them by Category,
So that I can see the real Olist catalog (FR13, UX-DR8).

**Acceptance Criteria:**

**Given** catalog-service exposes public gRPC `ListCategories` and `ListProducts(category?, page, page_size)`
**When** the gateway serves `GET /api/categories` and `GET /api/products?category=&page=`
**Then** both work without a token (NFR7: browsing is public)
**And** `ListCategories` includes "Uncategorized", and filtering by it returns the Products without a dataset Category
**And** `page < 1`, `page_size < 1` or `page_size > 100`, or an unknown Category, returns `INVALID_ARGUMENT`; a page past the last returns an empty list; results are sorted with a unique tie-break (`display_name, product_id`) so pages never overlap

**Given** the Products page `/`
**When** it loads
**Then** it shows a table with display name, Category, Seller and price (Money component), 20 per page with server-side pagination (UX-DR8)

**Given** a Category selected in the filter
**When** the list reloads
**Then** only Products in that Category are shown (FR13)
**And** a Category with no Products shows "No Products in this Category."

**Given** a Product without a Category in the dataset
**When** it is listed
**Then** its Category shows "Uncategorized"

## Epic 2: Checkout and pay, with a saga that survives failure

A Customer checks out from several Sellers, pays each Order by card, Voucher or both, and follows its status. A declined card cancels the Order and returns the Voucher; a payment-service outage loses nothing. The Admin creates Vouchers and sets the card decline rate.

### Story 2.1: Event envelope and transactional outbox relay

As the builder,
I want every service to publish events through one envelope and a transactional outbox,
So that no event is lost between a DB write and Kafka, and events for one Order stay in order (NFR1).

**Acceptance Criteria:**

**Given** `contracts/events/v1` and `pkg/eventbus`
**When** a producer publishes
**Then** every message uses the envelope `event_id, event_type, event_version, aggregate_id, historical, occurred_at, published_at, origin_published_at, traceparent, payload`, with `traceparent` also set as a Kafka header and the partition key = `order_id` (AD-8)
**And** `origin_published_at` equals `published_at` on a first publish, and is copied unchanged when a service republishes a fact derived from an incoming event (e.g. order-service republishing a replayed event), so freshness can be measured end to end (AD-17)
**And** only types defined in `contracts/events/v1` (Go structs + JSON Schema) can be published; this story defines the envelope and the schema tooling, and each later story adds the types it needs (AD-9)

**Given** `pkg/outbox`
**When** a use case changes state
**Then** it writes the state change and an outbox row (`bigserial seq`, current `traceparent`) in one DB transaction through a transaction-scoped API; there is no API to publish to Kafka directly from a use case (AD-6)

**Given** the outbox relay
**When** a service runs
**Then** one relay goroutine publishes unsent rows in `seq` order, polling every ≤ 200 ms (env), using `SELECT … FOR UPDATE SKIP LOCKED` only as a second-instance guard, never to parallelize (AD-6)
**And** each poll selects `WHERE sent_at IS NULL ORDER BY seq` and never tracks a high-water mark, so a lower `seq` that commits after a higher one is still published
**And** writes that touch the same Order lock that Order's row first, so one Order's outbox rows commit in `seq` order
**And** the producer is idempotent franz-go with `acks=all` in ordered mode, and `published_at` is set at relay time
**And** a relay metric exposes the count of unsent rows

**Given** an integration test with real Postgres and Kafka
**When** the relay is killed after the DB commit and before publishing, then restarted
**Then** the event is published, in `seq` order with the events after it
**And** a rolled-back transaction leaves no outbox row and publishes nothing

### Story 2.2: Idempotent inbox consumers

As the builder,
I want every consumer to apply each event exactly once and handle failures predictably,
So that redelivery never causes a double effect and a bad message never stalls a partition (NFR2).

**Acceptance Criteria:**

**Given** `pkg/inbox`
**When** a consumer receives an event
**Then** it records `event_id` in `processed_events` and applies its change in one transaction, and an already-known `event_id` is skipped and reported as `duplicate` (AD-7)

**Given** a handler result
**When** the consumer finishes a message
**Then** the outcome is one of `applied`, `duplicate`, `rejected` (domain-invalid: recorded with a reason, acked, counted in a metric) or `retryable` (infrastructure error: backed off, offset not committed)
**And** after N retryable attempts (env) the message goes to `<topic>.dlq` and the partition moves on
**And** a message that cannot be decoded, or has an unsupported major `event_version`, is `rejected` with a reason (never retried); an unknown `event_type` on a shared topic is acked as an `applied` no-op
**And** a new consumer group starts from the earliest offset (`auto.offset.reset = earliest`), so no event published before its first start is missed

**Given** a message in a `<topic>.dlq`
**When** the builder inspects or redrives it
**Then** a Prometheus metric exposes DLQ depth per topic, and `make dlq-redrive TOPIC=<topic>` republishes DLQ messages to the original topic; redrive is safe because consumers are idempotent (NFR1, NFR2)
**And** every demo script fails if any DLQ depth is > 0 at the end

**Given** a consumed event with `traceparent`
**When** the handler runs
**Then** it runs inside a child span of that trace, and its logs carry the trace id (AD-8, NFR3)
**And** the consumer span is started from the Kafka header for every consumed message, before any type filter or `historical=true` drop, so a consumer that takes no action still appears in the trace

**Given** an integration test
**When** the same event is delivered twice, and a handler fails transiently then succeeds
**Then** the effect is applied once, and the transient failure is retried without skipping the message

### Story 2.3: Checkout creates one Order per Seller

As a Customer,
I want to check out Products from several Sellers in one action,
So that I get one Order per Seller ready to pay (FR1).

**Acceptance Criteria:**

**Given** order-service
**When** this story is done
**Then** it owns migrations for Order (id UUIDv7, customer_id, checkout_id, status, purchased_at, historical, voucher_amount, cancel_reason nullable, voucher_restored), Order Item (product_id, seller_id, seller_state, category_en, price, freight in centavos), the status timeline (order_id, status, at) and `checkouts(customer_id, idempotency_key, request_hash, checkout_id)` with primary key `(customer_id, idempotency_key)` (AD-1, AD-5, AD-7)

**Given** a logged-in Customer sends `Checkout(product_ids, checkout_idempotency_key)` through the gateway
**When** the Products belong to 2 Sellers
**Then** exactly 2 Orders are created, each containing only that Seller's Order Items and each in `created` (FR1)
**And** order-service snapshots `price, freight, seller_id, seller_state, category_en` from catalog via gRPC into each Order Item, and `order_total = Σ(price + freight)` (AD-3, AD-15)

**Given** a Checkout
**When** it is saved
**Then** all its Orders, their first timeline entries and one `OrderCreated` outbox row per Order (with `from_status`, `to_status` and the full snapshot) are written in one transaction sharing one `checkout_id` (AD-7, AD-9, AD-10)
**And** the Checkout does not start the Order Saga (AD-4)

**Given** the same Customer sends the same `checkout_idempotency_key` twice with the same product list
**When** the second request arrives
**Then** no new Orders are created and the original Orders are returned
**And** the same key with a different product list (different `request_hash`) returns `ALREADY_EXISTS` and creates nothing
**And** keys are scoped per Customer, so another Customer using the same key gets their own new Checkout, never someone else's Orders

**Given** an unknown product id, an empty list, or the same product id more than once (there is no quantity field)
**When** Checkout is called
**Then** it returns `INVALID_ARGUMENT` and creates nothing

**Given** catalog-service is unavailable or slower than the call deadline (env)
**When** Checkout is called
**Then** it returns `UNAVAILABLE` and writes nothing

### Story 2.4: Customer sees own Orders and status timeline

As a Customer,
I want to see my Orders with their status and history,
So that I know where each one stands (FR7, UX-DR2, UX-DR11, UX-DR12).

**Acceptance Criteria:**

**Given** order-service gRPC `ListMyOrders(page)` and `GetOrder(id)`
**When** a Customer calls them
**Then** only Orders whose `customer_id` equals the caller are returned; another Customer's Order id returns `NOT_FOUND` (FR7, AD-11, AD-14)
**And** `GetOrder` returns `cancel_reason` and `voucher_restored` for a `canceled` Order
**And** page bounds are validated as in Story 1.8 (`page_size` ≤ 100)

**Given** My Orders `/orders`
**When** it loads
**Then** it lists my Orders newest first (`ORDER BY purchased_at DESC, id DESC`, so Orders from one Checkout never repeat or vanish across pages) with date, total (Money) and a status badge, with pagination and a Refresh button (UX-DR11)

**Given** the status badge component
**When** it shows any Order Status
**Then** it is a pill filled with that status's color token, with white text showing the exact dataset word (`created`, `approved`, `processing`, `shipped`, `delivered`, `canceled`) (UX-DR2)

**Given** Order detail `/orders/:id`
**When** it loads
**Then** it shows the items, the total and the status timeline oldest first with timestamps, plus a Refresh button (UX-DR12)
**And** a status change made elsewhere shows after Refresh; no live push (FR7)

### Story 2.5: Select Products and check out in the web app

As a Customer,
I want to tick Products and place a Checkout,
So that I can buy from several Sellers at once without a cart (UX-DR9, UX-DR10).

**Acceptance Criteria:**

**Given** the Products page
**When** I tick one or more Products (across pages and Category filters)
**Then** a sticky bottom bar shows "N items from M Sellers · R$ total · [Check out]"; the selection lives in page state mirrored to `sessionStorage` (no server-side cart) and is cleared after a successful Checkout (UX-DR9)

**Given** I am logged out
**When** I click "Check out"
**Then** I go to Login and return to the Checkout review afterwards with the same selection, restored from `sessionStorage`

**Given** the Checkout review `/checkout`
**When** it renders
**Then** it shows one Card per Seller with its items and subtotal, and a header "This will create M Orders." (UX-DR10)

**Given** `/checkout` is opened directly, refreshed, or with an empty selection
**When** no selection is found in `sessionStorage`
**Then** I am sent to Products with the toast "Select at least one Product to check out.", and "Place Checkout" is never enabled with an empty selection

**Given** I click "Place Checkout"
**When** the request succeeds
**Then** a toast says "M Orders created — one per Seller." and I land on My Orders
**And** double-clicking sends one idempotency key, so only M Orders exist

### Story 2.6: Admin manages Vouchers and the card decline rate

As an Admin,
I want to create Vouchers and set how often the simulated card declines,
So that I can test Voucher payment and drive the compensation demo (FR19, UX-DR18, UX-DR19).

**Acceptance Criteria:**

**Given** payment-service
**When** this story is done
**Then** it owns migrations for Voucher (code unique, face value centavos, used_by_order nullable with a unique index where not null, created_at) and gateway settings (decline rate 0–100, default from env); redemption state lives only in `used_by_order`, there is no separate redemption table
**And** it exposes admin gRPC `CreateVoucher(value)`, `ListVouchers(page)`, `GetDeclineRate`, `SetDeclineRate(percent)`, callable only by Admin (AD-4, FR16)

**Given** an Admin creates a Voucher with value R$ 30,00
**When** it is saved
**Then** it gets a unique random code and starts unused (FR19)
**And** a value ≤ 0 returns `INVALID_ARGUMENT`
**And** on a code collision the code is regenerated (bounded retries) instead of returning an error

**Given** any Voucher, used or not
**When** anything tries to change its value
**Then** it is refused: Voucher value is immutable and no RPC can change it (FR19)

**Given** `SetDeclineRate`
**When** it is called with a value that is not an integer from 0 to 100
**Then** it returns `INVALID_ARGUMENT` and the setting is unchanged

**Given** Vouchers `/admin/vouchers`
**When** an Admin opens it
**Then** it shows a create form (value) and a table of code, value and used/unused (UX-DR18)

**Given** Payment gateway `/admin/gateway`
**When** an Admin sets the decline rate to 100%
**Then** the setting is saved and shown, and a Customer or Seller calling these routes gets 403 (UX-DR19)

**Given** payment-service gRPC `LookupVoucher(code)`, callable by Customers through the gateway route `GET /api/vouchers/:code`
**When** a Customer looks up a code
**Then** it returns the face value and whether it is usable for an existing unused code, and `NOT_FOUND` for an unknown code; it changes nothing (AD-3: gateway→service gRPC)

### Story 2.7: Request payment for an Order

As a Customer,
I want to submit a payment for one of my `created` Orders,
So that the Order Saga starts safely, at most once per request (FR2, FR4).

**Acceptance Criteria:**

**Given** order-service exposes `OrderService.PayOrder(order_id, voucher_code?, card_token, idempotency_key)` and the gateway routes `POST /api/orders/:id/pay` for Customers
**When** the caller owns the Order and it is `created`
**Then** in one transaction order-service stores `payment_requests(idempotency_key PK, order_id, request_hash)` and writes `AuthorizePayment{order_id, amount_total, voucher_code?, card_token, idempotency_key}` to its outbox, published on `payment.commands` (AD-4, AD-7, AD-8)
**And** `amount_total` is the Order's `order_total` from the Checkout snapshot (AD-15), and the response is "accepted"; the Order stays `created` until the saga reply

**Given** the same `idempotency_key` with the same body is sent twice
**When** the second request arrives
**Then** it returns the same accepted response and no second `AuthorizePayment` is written (FR4)

**Given** the same `idempotency_key` with a different body
**When** it arrives
**Then** it returns `ALREADY_EXISTS` (AD-7)

**Given** an Order that is not `created`, or already has a payment request in flight with another key
**When** `PayOrder` is called
**Then** it returns `FAILED_PRECONDITION`
**And** the check runs after `SELECT … FOR UPDATE` on the Order row, so two concurrent calls with different keys write at most one `AuthorizePayment`

**Given** a `card_token` that is neither the "approves" nor the "declines" test token
**When** `PayOrder` is called
**Then** it returns `INVALID_ARGUMENT` and writes nothing

**Given** an Order owned by another Customer
**When** `PayOrder` is called
**Then** it returns `NOT_FOUND` and writes nothing (AD-14)

**Given** payment-service is not running
**When** a Customer pays
**Then** the request is still accepted and the command waits in Kafka (NFR1)

### Story 2.8: Authorize payment and approve the Order

As a Customer,
I want my payment authorized and my Order approved,
So that it moves on to the Seller (FR2, FR4, FR5 auto-processing).

**Acceptance Criteria:**

**Given** payment-service consumes `payment.commands` through the inbox (dropping `historical=true` envelopes before decoding, AD-13)
**When** it processes `AuthorizePayment` with a Voucher that is unused and a card token that approves
**Then** it redeems the Voucher with one conditional update (`SET used_by_order = $order WHERE code = $code AND used_by_order IS NULL`), records `voucher_amount = min(face, order_total)`, records a Card Payment of `order_total − voucher_amount`, and writes `PaymentSucceeded{voucher_amount}` to its outbox on `payment.events` (AD-4, AD-15)
**And** payment-service owns the Payment table: exactly one Payment row per Order (`UNIQUE(order_id)`) with `voucher_code?, voucher_amount, card_amount, status (succeeded | failed), reason?`, written for every outcome — Voucher-only (`card_amount = 0`), card-only, mixed, and failed; Voucher redemption uniqueness comes from the unique index on `used_by_order` (AD-1, AD-7)
**And** payment-service exposes no pay RPC (AD-4)

**Given** a simulated card
**When** a charge is attempted
**Then** it declines with probability equal to the current decline rate, or always for the "declines" test token; with rate 0% and the "approves" token it approves

**Given** a Voucher whose value covers the whole Order total
**When** it is processed
**Then** no card is charged, the Payment row has `card_amount = 0`, and `PaymentSucceeded` is emitted

**Given** order-service consumes `PaymentSucceeded`
**When** it applies it
**Then** in one transaction the Order moves `created → approved → processing`, both transitions are added to the timeline, `voucher_amount` is stored, and `OrderApproved` then `OrderProcessing` are written to the outbox in that order, each with the full snapshot (AD-5, AD-9, AD-10)
**And** the Order never stays in `approved` (FR5)
**And** other Orders from the same Checkout keep their status (FR2)

**Given** `PaymentSucceeded` for an Order that is unknown or not `created`
**When** order-service handles it
**Then** it is `rejected` with a reason and counted in a metric; the Order is unchanged and nothing is retried (AD-7)

**Given** `AuthorizePayment` is delivered twice
**When** both are processed
**Then** exactly one Payment record exists and the card is charged once (FR4, NFR2)

### Story 2.9: Compensation when payment fails

As a Customer,
I want a failed payment to cancel my Order and give my Voucher back,
So that I never lose a Voucher or get stuck in `created` (FR3).

**Acceptance Criteria:**

**Given** the decline rate is 100% and an Order is paid with Voucher + card
**When** payment-service processes `AuthorizePayment`
**Then** it redeems the Voucher, the card declines, it restores the Voucher (`SET used_by_order = NULL WHERE code = $code AND used_by_order = $order`) and emits `PaymentFailed{reason: CARD_DECLINED, voucher_restored: true}` (AD-4)
**And** order-service moves the Order `created → canceled`, stores `cancel_reason` and `voucher_restored` from the event, and writes `OrderCanceled` (AD-5)
**And** the same Voucher code is accepted on another Order afterwards (FR3)

**Given** a Voucher code that is unknown, or already used by another Order (e.g. used between the UI lookup and the payment)
**When** an Order is paid with it
**Then** the conditional update changes 0 rows, payment-service emits `PaymentFailed{reason: VOUCHER_UNAVAILABLE, voucher_restored: false}` without charging the card, and the Order is `canceled`

**Given** any `PaymentFailed` for a `created` Order
**When** order-service handles it
**Then** the Order moves to `canceled` and does not stay in `created` (FR3)
**And** a duplicate `PaymentFailed` delivery is a `duplicate` with no further effect (NFR2)
**And** a `PaymentFailed` for an Order that is unknown or not `created` is `rejected` with a reason and counted in a metric

### Story 2.10: Pay form and payment states in the web app

As a Customer,
I want to pay from the Order detail page and see the outcome clearly,
So that I can run the payment demos from the UI (UX-DR13, UX-DR14).

**Acceptance Criteria:**

**Given** Order detail for a `created` Order
**When** it renders
**Then** it shows an optional Voucher code input, a card token Select ("Test card — approves" / "Test card — declines", simulated tokens only, no card number field) and a "Pay R$ X" button (UX-DR13)
**And** when a Voucher code is entered, the page calls `GET /api/vouchers/:code` and shows "Voucher R$ V · Card R$ remainder" from the returned face value

**Given** a Voucher code that the lookup reports as unknown (`NOT_FOUND`) or used
**When** the lookup returns
**Then** the gateway message is shown inline under the Voucher field and Pay stays disabled until the code is fixed or cleared, so a mistyped code never reaches `PayOrder` (UX-DR14)

**Given** I click Pay
**When** the request is accepted
**Then** the button is disabled with "Processing payment…", the page sends one idempotency key per Order per page visit, and it polls the Order every 1 s for up to 10 s until it leaves `created` (UX-DR14)
**And** a 409 (payment already in flight, e.g. after a reload created a new key) shows "Payment already in progress." and starts the same polling

**Given** the Order reaches `processing`
**When** polling sees it
**Then** a toast says "Payment approved." and the timeline updates

**Given** the Order reaches `canceled`
**When** polling sees it
**Then** a destructive toast is chosen by `cancel_reason`: `CARD_DECLINED` → "Card declined. Order canceled."; `VOUCHER_UNAVAILABLE` → "Voucher unavailable. Order canceled."; "Voucher restored." is added only when `voucher_restored` is true; the pay form is hidden

**Given** polling times out
**When** 10 s pass without a status change
**Then** the page shows "Still processing. Refresh in a moment."

### Story 2.11: Prove no lost Orders when payment-service is down

As the builder,
I want a scripted demo that stops payment-service mid-flow,
So that I can show in an interview that every Order still leaves `created` (NFR1, NFR2, SM-1, SM-2).

**Acceptance Criteria:**

**Given** the system running via `make up`
**When** the builder runs the SM-1 demo script (e.g. `make demo-no-lost-orders`): stop payment-service, place a Checkout and pay several Orders, then start payment-service
**Then** every paid Order leaves `created` to `processing` or `canceled`, and none is stuck
**And** the script polls with a deadline (env, default 60 s), prints each Order's final status, and exits non-zero if any Order is still `created` at the deadline or any DLQ depth is > 0

**Given** the SM-2 demo script
**When** it sets decline rate 100%, pays an Order with Voucher + card, then reuses the Voucher
**Then** the Order ends `canceled` and the Voucher is accepted again

**Given** a consumer receives the same event twice (forced redelivery in a test)
**When** it processes both
**Then** there is one Payment, one status transition and one timeline entry (NFR2)

**Given** a Pay request
**When** the builder opens Jaeger
**Then** one trace links gateway → order-service → (Kafka) payment-service → (Kafka) order-service, with `traceparent` carried through outbox rows and Kafka headers (NFR3; the ≥ 4-service check completes in Epic 3)

## Epic 3: Seller fulfilment and simulated delivery

A Seller lists and filters their own Orders and marks `processing` Orders shipped; a durable simulator delivers them and the Customer sees it in the timeline.

### Story 3.1: Seller lists own Orders by status

As a Seller,
I want to list my Orders and filter them by Order Status,
So that I can see what is waiting to ship (FR8, UX-DR15).

**Acceptance Criteria:**

**Given** order-service gRPC `ListSellerOrders(status?, page)` and the gateway route `GET /api/seller/orders?status=&page=` for Sellers only
**When** a Seller calls it
**Then** it returns only Orders that contain at least one Order Item with `seller_id` = the caller, newest first (`ORDER BY purchased_at DESC, id DESC`), paginated (FR8, AD-11, AD-14)
**And** an unknown `status` value or out-of-bounds page returns `INVALID_ARGUMENT` (bounds as in Story 1.8)
**And** each Order shows only the caller's own Order Items and their subtotal, never another Seller's items

**Given** the filter `processing`
**When** the list loads
**Then** it shows only that Seller's Orders in `processing` (FR8)

**Given** a Customer or Admin token
**When** the route is called
**Then** the gateway returns 403 (FR16)

**Given** Seller Orders `/seller/orders`
**When** a Seller opens it
**Then** it shows a table of Order id, purchase date, own items subtotal (Money) and status badge, an Order Status filter, pagination and a Refresh button (UX-DR15)
**And** an empty result shows "No Orders in this range." style empty text for the selected status

### Story 3.2: Seller marks an Order shipped

As a Seller,
I want to mark one of my `processing` Orders as shipped,
So that it goes out for delivery (FR5, UX-DR15).

**Acceptance Criteria:**

**Given** order-service gRPC `MarkShipped(order_id)` and the gateway route `POST /api/seller/orders/:id/ship` for Sellers
**When** the caller owns the Order's items and the Order is `processing`
**Then** in one transaction the Order moves to `shipped`, the transition is added to the timeline, and `OrderShipped` (with the full snapshot) is written to the outbox on `order.events` (AD-5, AD-9, AD-10)

**Given** an Order of another Seller
**When** `MarkShipped` is called
**Then** it returns `NOT_FOUND` and nothing changes (FR5)

**Given** an Order that is not `processing`
**When** `MarkShipped` is called
**Then** it returns `FAILED_PRECONDITION` and nothing changes (FR5, AD-5)
**And** the transition is one conditional `UPDATE … SET status = 'shipped' WHERE id = $1 AND status = 'processing'`; 0 rows returns `FAILED_PRECONDITION`, so two concurrent calls produce one `OrderShipped` and one timeline entry

**Given** a Historical Order (`historical = true`), in any status
**When** `MarkShipped` is called
**Then** it returns `FAILED_PRECONDITION` and nothing changes; only Replay moves Historical Orders (AD-12)

**Given** Seller Orders in the web app
**When** a row is a live (non-Historical) `processing` Order
**Then** it shows a "Mark shipped" button; other rows, including Historical rows, do not
**And** clicking it opens a confirm Dialog; confirming updates the row to `shipped`
**And** a refusal shows a toast with the gateway message (UX-DR15)

### Story 3.3: Simulated delivery

As a Customer,
I want shipped Orders to be delivered automatically,
So that I can see my Order complete without a real carrier (FR6, FR7).

**Acceptance Criteria:**

**Given** shipping-simulator
**When** this story is done
**Then** it owns `shipments(order_id PK, deliver_at, delivered)` in its own database, plus inbox and outbox tables (AD-16)
**And** it consumes `order.events` through the inbox, drops `historical=true` envelopes before decoding (AD-13), and on `OrderShipped` inserts a shipment with `deliver_at = now() + SHIPPING_DELAY` (env, default 3s)

**Given** a shipment whose `deliver_at <= now()`
**When** the poller runs (every ≤ 500 ms, env)
**Then** in one transaction it marks the shipment delivered and writes `ShipmentDelivered` to its outbox on `shipping.events` (AD-6, AD-16)

**Given** order-service consumes `ShipmentDelivered`
**When** the Order is `shipped`
**Then** it moves to `delivered`, the transition is added to the timeline, and `OrderDelivered` is written to the outbox (AD-5)
**And** with default settings delivery is scheduled 3 s after `OrderShipped`, and the Order is `delivered` within `SHIPPING_DELAY` + 1 s of the `OrderShipped` commit (FR6; the extra second covers poller, relay and consumer latency)
**And** a `ShipmentDelivered` for an Order that is unknown or not `shipped` is `rejected` with a reason and counted in a metric

**Given** `OrderShipped` or `ShipmentDelivered` is delivered twice
**When** both are processed
**Then** there is one shipment and one `delivered` transition (NFR2)

**Given** the Customer's Order detail
**When** they Refresh after delivery
**Then** the timeline shows `… → processing → shipped → delivered` (FR7)

### Story 3.4: Delivery survives a crash, and one trace spans four services

As the builder,
I want to prove delivery is durable and a paid Order produces one trace across at least four services,
So that I can demo NFR1 and SM-3 in an interview (NFR1, NFR3, SM-3).

**Acceptance Criteria:**

**Given** an Order marked shipped
**When** shipping-simulator is stopped before `deliver_at` and started again later
**Then** the Order still reaches `delivered` after restart, with no manual action (AD-16, NFR1)
**And** a demo script (e.g. `make demo-durable-delivery`) runs this, polls with a deadline (env, default 60 s), and exits non-zero if the Order is not `delivered` by the deadline

**Given** a Customer pays an Order
**When** the builder opens Jaeger and searches by the Pay request's trace id
**Then** one trace contains spans from api-gateway, order-service, payment-service and shipping-simulator (its consumer span for `OrderApproved`/`OrderProcessing`, recorded even though it takes no action), linked through outbox rows and Kafka headers: ≥ 4 services (NFR3, SM-3)
**And** shipping-simulator starts that consumer span from the Kafka header before filtering by event type (Story 2.2), and a test asserts the span exists for an event it ignores
**And** the later Mark shipped request forms its own trace that includes shipping-simulator's delivery and order-service's `delivered` transition
**And** the README documents the steps to reproduce the trace demo

**Given** any service's logs for that flow
**When** the builder filters by the trace id
**Then** log lines from every participating service appear (NFR3)

## Epic 4: Replay real Olist history

The builder runs `make replay` over a date range and Historical Orders stream in as if live; a seeded Seller sees their real order history.

### Story 4.1: Replayer streams Historical Order status events

As the builder,
I want `make replay` to stream the dataset's order history as events in time order,
So that the system fills up as if the marketplace were live (FR18).

**Acceptance Criteria:**

**Given** `tools/olist-importer` with a replay command
**When** the builder runs `make replay FROM=2017-01-01 TO=2017-12-31` (speed optional)
**Then** only Historical Orders purchased in that range are replayed, and the default speed is 1 day of data per 1 second (FR18)
**And** `FROM` and `TO` are dates in America/Sao_Paulo time, both inclusive (`[FROM 00:00, TO+1 day 00:00)`); without them `make replay` uses the full dataset range
**And** every status event of an in-range Order is published, including events dated after `TO` (e.g. a delivery a month later), so in-range Orders get complete timelines
**And** the replayer runs separately from the services and talks only to Kafka (no SQL, no service gRPC) (AD-2, AD-12)

**Given** the Olist orders, items, customers, sellers, products and category translation CSVs
**When** the replayer derives status changes
**Then** it maps: purchase → `created`; `approved_at` → `approved`; `invoiced`/`processing` → `processing` at `approved_at`; `delivered_carrier_date` → `shipped`; `delivered_customer_date` → `delivered`; `canceled`/`unavailable` → `canceled` at the last known timestamp (AD-12)
**And** rows with no usable timestamp are skipped and counted in a replay report printed at the end
**And** a `canceled`/`unavailable` Order emits only `created` then `canceled` (its intermediate statuses are skipped), so every emitted sequence is allowed by AD-5
**And** an Order whose final status has no timestamp (e.g. `delivered` with a null `delivered_customer_date`) emits the statuses it can derive and is counted as `missing_final_ts` in the report
**And** within one Order, statuses are ordered by `(timestamp, status rank)`, and a later status with an earlier timestamp than the previous one is clamped to that timestamp, so `approved` and `processing` sharing `approved_at` always apply in order

**Given** each derived status change
**When** it is published
**Then** it is one `HistoricalOrderStatusRecorded` on `replay.orders`, keyed by `order_id`, with `historical=true`, `occurred_at` = the dataset timestamp converted from São Paulo time to UTC, and a payload already enriched with `customer_unique_id`, every item's `product_id, seller_id, seller_state, category_en` ("Uncategorized" if none), `price` and `freight` in centavos (AD-8, AD-10, AD-12, Conventions)
**And** `event_id = UUIDv5(NS_OLIST_REPLAY, order_id + ":" + status)`, with the namespace constant in `contracts` (AD-12)
**And** events are sorted by status timestamp and paced by the speed

**Given** the replayer is stopped with Ctrl-C
**When** it exits
**Then** it prints how far it got, and running it again republishes with the same `event_id`s

### Story 4.2: order-service ingests Historical Orders

As the builder,
I want order-service to record replayed history without running the saga,
So that Historical Orders appear in lists and flow on to read models (FR18, AD-12, AD-13).

**Acceptance Criteria:**

**Given** order-service consumes `replay.orders` through the inbox
**When** the first event for an Order arrives
**Then** it creates the Order with `historical=true`, `customer_id = customer_unique_id`, `purchased_at` from the dataset, and all its Order Items (which may belong to several Sellers), with that status as the first timeline entry; the first status may be any status (AD-5)
**And** no `AuthorizePayment` or other saga step is triggered (AD-4)
**And** `PayOrder` on a Historical Order returns `FAILED_PRECONDITION`, even if its status is `created`

**Given** later events for the same Order
**When** they arrive
**Then** each is applied only if forward-reachable from the current status; otherwise it is `rejected` with a reason and counted in a metric (AD-5, AD-7)
**And** each applied transition is stored in the timeline with its dataset timestamp

**Given** an applied Historical transition
**When** it commits
**Then** order-service writes the matching `Order*` fact with `historical=true` and the full snapshot to its outbox, reusing the incoming `event_id` (AD-10, AD-12)

**Given** payment-service and shipping-simulator
**When** they receive `historical=true` envelopes
**Then** they drop them before decoding: no Payment, no Shipment (AD-13)

**Given** a Historical Order
**When** a Customer or Seller tries to act on it
**Then** `PayOrder` and `MarkShipped` both return `FAILED_PRECONDITION` (Stories 3.2 and 4.2)

**Given** the same `replay.orders` event delivered twice
**When** it is processed
**Then** there is one transition and one outbox fact (NFR2)

### Story 4.3: Seeded users see their real history, and Replay restarts safely

As a seeded Seller,
I want to see my real Olist orders after Replay,
So that my account has history from day one (FR15, FR8, FR18).

**Acceptance Criteria:**

**Given** Replay has run over the full dataset range
**When** a seeded Seller logs in and opens Seller Orders
**Then** their Historical Orders are listed with dataset statuses and dates (FR15)
**And** for a Historical Order spanning several Sellers, they see only their own Order Items and subtotal (FR8)

**Given** a seeded Customer (chosen by most Orders, Story 1.5)
**When** they open My Orders after a full-range Replay
**Then** their Historical Orders appear with their timelines, and no Pay form is shown on them

**Given** Replay is stopped midway and started again over the same range
**When** it finishes
**Then** the number of Historical Orders and timeline entries equals a single uninterrupted run (FR18, NFR2)
**And** an automated test asserts this on a small fixture range

**Given** the replay report
**When** Replay finishes
**Then** it shows events published, rows skipped by reason, and the elapsed time

## Epic 5: Seller Revenue and the live Ops Dashboard

A Seller sees Seller Revenue for a date range; an Admin watches revenue by Seller state and by Category and Order counts by status update live during Replay. Ends demo-ready with Kubernetes manifests.

### Story 5.1: analytics-service builds the order projection from events

As the builder,
I want analytics-service to keep a read model of every Order from `order.events` alone,
So that revenue and dashboard queries never call another service (AD-10, AD-11, NFR5).

**Acceptance Criteria:**

**Given** analytics-service consumes `order.events` through the inbox, including `historical=true` events, with its consumer group starting from the earliest offset (Story 2.2)
**When** an `Order*` fact arrives
**Then** it upserts the projection from the event's full snapshot: one row per Order (`order_id, purchased_at, historical, status, voucher_amount`) and one row per Order Item (`order_id, seller_id, seller_state, category_en, price`) (AD-10)
**And** it never calls another service or reads another service's database (AD-2, AD-10)

**Given** events for an Order arrive out of order or twice
**When** they are applied
**Then** the Order's status only moves forward along the allowed transitions; an older or repeated status has no effect, so no count is ever doubled or goes below 0 (AD-5, AD-7, NFR2)

**Given** a stream of events
**When** analytics consumes them
**Then** it commits in batches at most every 500 ms (AD-17)
**And** it exposes a Prometheus lag histogram measured at apply time, `applied_at − origin_published_at`, so republished replay facts are measured from the replayer's original publish (AD-17, Story 2.1)
**And** when nothing is being applied the lag is reported as 0, not as a growing gap since the last event

**Given** an empty system before the first Replay
**When** the projection is queried
**Then** it holds only new Orders placed through Checkout, or nothing (FR17)

### Story 5.2: Seller sees Seller Revenue for a date range

As a Seller,
I want to see how much I earned between two dates,
So that I know my revenue across history and new sales (FR9, UX-DR16).

**Acceptance Criteria:**

**Given** analytics-service gRPC `GetSellerRevenue(from, to)` and the gateway route `GET /api/seller/revenue?from=&to=` for Sellers only
**When** a Seller calls it
**Then** it returns the Seller Revenue for the caller only, filtered on `purchased_at` within the range (FR9, AD-11)
**And** `from` and `to` are dates in America/Sao_Paulo time, both inclusive (`[from 00:00, to+1 day 00:00)`); a missing or malformed date, or `from` after `to`, returns `INVALID_ARGUMENT`

**Given** the revenue rule
**When** it is computed
**Then** only `delivered` Orders count; per new Order it is `max(0, Σ own item price − voucher_amount)`; per Historical Order it is `Σ own item price` with Vouchers ignored; freight is never included (FR9, AD-11)
**And** a test shows an Order with price 100 paid with a 30 Voucher plus card adds 70, and `canceled` or undelivered Orders add 0

**Given** Seller Revenue `/seller/revenue`
**When** a Seller opens it
**Then** it shows two native date inputs defaulting to 2016-09-01 → today and the total as Money (UX-DR16)
**And** a new Order delivered today counts in the default range
**And** "No data yet. Run `make replay` to stream Historical Orders." shows only while the projection is empty for every Seller; once any data exists, a Seller with no delivered revenue in the range sees R$ 0,00

### Story 5.3: Admin Ops Dashboard with live auto-refresh

As an Admin,
I want revenue by Seller state and by Category and Order counts by status for a date range, refreshing on their own,
So that I can watch the marketplace live during Replay (FR10, FR11, FR12, UX-DR4, UX-DR17).

**Acceptance Criteria:**

**Given** analytics-service gRPC `GetDashboard(from, to)` and the gateway route `GET /api/admin/dashboard?from=&to=` for Admin only
**When** it is called
**Then** it returns revenue by Seller state, revenue by Category (English name, "Uncategorized" for none), and Order counts per Order Status, all filtered on `purchased_at` (FR10, FR11, FR12)
**And** the sum across states equals the sum of Seller Revenue across all Sellers for the same range (FR10)
**And** for a new Order whose items span several Categories, `voucher_amount` is allocated to items pro-rata by item price (rounding remainder to the highest-priced item), so the sum across Categories also equals total Seller Revenue (FR11)
**And** counts across all statuses equal the total number of Orders in the range (FR12)
**And** date parsing and validation are the same as in Story 5.2

**Given** Ops Dashboard `/admin`
**When** an Admin opens it
**Then** it shows a date range filter defaulting to 2016-09-01 → today and three tables (by Seller state, by Category, by Order Status), each with a total row, money and counts right-aligned, and a bar cell behind each number sized to its share of the column maximum (UX-DR4, UX-DR17)
**And** the status table uses the status badge for each row

**Given** the dashboard is open
**When** time passes
**Then** it re-fetches every 2 s without a page reload, and "Updated Xs ago" next to the title stays current (FR12, AD-17)
**And** before any data it shows "No data yet. Run `make replay` to stream Historical Orders."

**Given** the re-fetch keeps failing (analytics or the gateway is down)
**When** each 2 s refresh fails
**Then** the last good data stays on screen, "Updated Xs ago" keeps counting from the last success, and the error toast is shown once per outage, not on every failed refresh

### Story 5.4: Prove the dashboard stays within 5 seconds during Replay

As the builder,
I want to show the Ops Dashboard keeping up with Replay,
So that I can demo the live read model in an interview (NFR4, SM-4).

**Acceptance Criteria:**

**Given** Replay running at default speed
**When** the builder watches Grafana
**Then** a provisioned dashboard panel shows analytics lag (p95 of `applied_at − origin_published_at`, Story 5.1) staying ≤ 5 s, plus relay unsent-row counts per service (NFR4, AD-17)

**Given** the Ops Dashboard during Replay
**When** the builder watches it
**Then** the Order counts and revenue totals change at least every few seconds without a reload (SM-4)

**Given** a demo script (e.g. `make demo-live-dashboard`)
**When** it runs a short Replay range
**Then** it samples the lag metric only while Replay is running, stops at a deadline (env, default 120 s), and exits non-zero if lag exceeded 5 s, the deadline passed, or any DLQ depth is > 0
**And** the README documents how to run the SM-4 demo

### Story 5.5: Kubernetes manifests

As the builder,
I want Kubernetes manifests for the whole system,
So that I can show the same services running on Kubernetes in week 8 (PRD §7.1).

**Acceptance Criteria:**

**Given** `deployments/k8s/`
**When** the builder applies the manifests to a local cluster (e.g. kind)
**Then** every service, web, Postgres, Kafka, Jaeger, Prometheus and Grafana run with the same env config as compose
**And** each service has liveness and readiness probes on `/healthz` and `/readyz`

**Given** the cluster is up
**When** the builder runs Import, Replay and a Checkout
**Then** the four demos (SM-1…SM-4) work the same as with compose
**And** every demo script takes `RUNTIME=compose|k8s` and stops/starts a service with `docker compose stop/start` or `kubectl scale --replicas=0/1` accordingly

**Given** service gRPC ports and databases
**When** the manifests are reviewed
**Then** only api-gateway, web and the observability UIs are exposed outside the cluster (AD-14)
**And** `make up` with docker compose remains the default local run (NFR6)
