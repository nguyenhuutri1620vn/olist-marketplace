---
title: "PRD: Olist Marketplace"
status: final
created: 2026-10-29
updated: 2026-10-30
---

# PRD: Olist Marketplace

## 0. Document Purpose

This PRD defines what Olist Marketplace must do for its MVP. It builds on the product brief at `../../briefs/brief-olist-marketplace-2026-10-29/` and does not repeat it. Domain terms are defined once in the Glossary and used exactly. Features are grouped, with globally numbered FRs (FR-1…FR-N). Assumptions made during drafting were confirmed by the builder and are listed in the Assumptions Index. Technology choices live in `addendum.md`, not here.

## 1. Vision

Olist Marketplace is a personal learning vehicle: a "mini Shopee" built on the real Olist Brazilian E-Commerce dataset. It exists so the builder can practice microservices, domain modeling, event streaming, and concurrency under realistic conditions, with BMAD as the delivery method.

Success is not users or revenue. Success is the builder being able to explain every service boundary and failure path in an interview, and to demo them running.

## 2. Target User

### 2.1 Jobs To Be Done

- **Functional:** Learn domain modeling, microservices patterns, Kafka, and concurrency by building them against real data, not toy examples.
- **Functional:** Learn the BMAD method end to end, from brief to shipped stories, plus the superpowers skills as a development aid.
- **Interview:** Have working code and a live demo to point at when asked about saga, outbox, CQRS, tracing, or idempotency.

### 2.2 Non-Users (v1)

- Real shoppers, sellers, or operators. The customer, seller, and admin roles exist so the flows are realistic; nobody depends on the system.

## 3. Glossary

- **Customer** — A person who buys. Dataset Customers are identified by `customer_unique_id`; others register (FR-14). Has one Account.
- **Seller** — A shop that lists Products and fulfils Orders. Has one Account.
- **Admin** — An operator who views the Ops Dashboard and manages Vouchers.
- **Account** — Login credentials plus one role: Customer, Seller, or Admin.
- **Product** — An item sold by exactly one Seller, in one Category.
- **Category** — A product grouping, shown in English using the dataset's translation table.
- **Checkout** — The Customer's action of buying one or more Products. Produces one Order per Seller.
- **Order** — A purchase containing one or more Order Items, with one Order Status. A new Order is always from exactly one Seller; a Historical Order may span several.
- **Order Item** — One Product line within an Order, with price and freight value.
- **Order Status** — One of: `created` (awaiting payment), `approved` (fully paid), `processing` (Seller is preparing it; entered automatically after `approved`), `shipped`, `delivered`, `canceled`. Names follow the Olist dataset.
- **Payment** — Money applied to one Order. An Order has at most one Card Payment and at most one Voucher.
- **Card Payment** — A Payment through the simulated card gateway, which can be configured to decline.
- **Voucher** — A single-use code with a fixed value. Created by Admin; cannot be partially used.
- **Historical Order** — An Order imported from the Olist CSVs. May span several Sellers and does not go through the Order Saga.
- **Order Saga** — The sequence that takes a new Order from `created` to `approved` or `canceled`, including compensation.
- **Seller Revenue** — See FR-9 for the exact rule.
- **Replay** — Streaming Historical Orders over time as events, as if they were happening live.

## 4. Features

### 4.1 Checkout and Payment

**Description:** A Customer selects Products and checks out. Checkout creates one Order per Seller, each in `created`. The Customer pays each Order separately, by Card Payment, Voucher, or both. Each Order runs its own Order Saga. If any part of the payment fails, the Order is canceled and anything already applied is given back. Historical Orders never enter this flow.

#### FR-1: Checkout splits by Seller

A Customer can check out Products from several Sellers in one action.

**Consequences (testable):**
- Checking out Products from 2 Sellers creates exactly 2 Orders, each containing only that Seller's Order Items.
- Each new Order starts in `created`.

#### FR-2: Pay an Order

A Customer can pay one `created` Order with a Card Payment, a Voucher, or a Voucher plus a Card Payment for the remainder.

**Consequences (testable):**
- An Order whose Payments cover its total moves to `approved`.
- A Voucher already used cannot be applied again.
- Paying one Order does not change the status of the Customer's other Orders from the same Checkout.

**Out of Scope:** installments, boleto, one payment covering several Orders.

#### FR-3: Compensation on failed payment

When a Card Payment is declined, the system cancels the Order and restores any Voucher applied to it.

**Consequences (testable):**
- With the gateway set to decline 100%, an Order paid with Voucher + card ends `canceled` and the Voucher is usable again.
- No Order stays in `created` after a decline.

#### FR-4: No double charge

A repeated payment request for the same Order charges at most once.

**Consequences (testable):**
- Sending the same payment request twice (same idempotency key) yields one Payment record.

### 4.2 Delivery Tracking

**Description:** When an Order becomes `approved`, the system moves it to `processing` straight away. The Seller then marks it shipped from the Seller UI. A delivery simulator then marks it `delivered` a few seconds later, standing in for a real carrier. The Customer sees the current Order Status of each of their Orders.

#### FR-5: Seller ships an Order

The system moves every `approved` Order to `processing` automatically. A Seller can then mark one of their own `processing` Orders as shipped.

**Consequences (testable):**
- An Order never stays in `approved`; it reaches `processing` without any user action.
- Marking shipped moves the Order to `shipped`.
- A Seller cannot ship another Seller's Order, or an Order that is not `processing`.

#### FR-6: Simulated delivery

The system moves a `shipped` Order to `delivered` automatically.

**Consequences (testable):**
- A simulated courier delivers a `shipped` Order after `SHIPPING_DELAY` (default 3 seconds, configurable), so demos reach `delivered` quickly; the Order then shows `delivered` within about 1 second more.

#### FR-7: Customer tracks Orders

A Customer can see each of their Orders with its Order Status and its timeline of status changes.

**Consequences (testable):**
- A status change is visible to the Customer after a page refresh; live push is not required.
- A Customer sees only their own Orders.

**Out of Scope:** estimated delivery dates.

### 4.3 Seller Orders and Revenue

**Description:** A Seller sees the Orders that contain their Products and how much they earned. Revenue covers both Historical Orders and new Orders, so a Seller from the dataset sees real history from day one.

#### FR-8: Seller order list

A Seller can list their own Orders and filter them by Order Status.

**Consequences (testable):**
- Filtering by `processing` shows only that Seller's Orders waiting to ship.
- A Seller never sees another Seller's Orders. For a Historical Order spanning several Sellers, a Seller sees only their own Order Items.

#### FR-9: Seller Revenue

A Seller can see their Seller Revenue for a date range (from/to).

- **Seller Revenue** — For `delivered` Orders only: the sum of the Seller's Order Item `price` (freight excluded), minus the Voucher value applied to those Orders. Never below 0 per Order.

**Consequences (testable):**
- `canceled` Orders and Orders not yet `delivered` contribute 0.
- An Order of price 100 paid with a 30 Voucher plus card adds 70.
- The date range filters on the Order's purchase time.
- For Historical Orders, Vouchers are ignored: revenue is the full Order Item `price`. Only new Orders deduct Vouchers.

### 4.4 Ops Dashboard

**Description:** An Admin watches the marketplace as a whole. The dashboard is a read model fed by events, so it keeps updating while Replay runs. It covers Historical Orders and new Orders together.

#### FR-10: Revenue by Seller state

An Admin can see Seller Revenue grouped by the Seller's state, for a date range (from/to on purchase time).

**Consequences (testable):**
- The total across all states equals the sum of Seller Revenue across all Sellers for the same range.

#### FR-11: Revenue by Category

An Admin can see Seller Revenue grouped by Category (English name), for the same date range.

**Consequences (testable):**
- Products without a Category are grouped under "Uncategorized".

#### FR-12: Order counts by status

An Admin can see the number of Orders in each Order Status, for the same date range.

**Consequences (testable):**
- While Replay runs, the counts change without a page reload; auto-refresh every few seconds is enough.
- Counts across all statuses equal the total number of Orders in the range.

### 4.5 Product Browsing

**Description:** Anyone can browse Products by Category. The Olist dataset has no product names or prices, so the system derives both.

#### FR-13: Browse and filter Products

A visitor can list Products and filter them by Category.

**Consequences (testable):**
- Each Product shows a display name of the form `<Category English name> #<first 4 chars of product id>`, e.g. "Bed Bath Table #a1b2".
- Each Product shows a price equal to its most recent Order Item `price` in the dataset.
- Filtering by a Category shows only Products in that Category.

### 4.6 Accounts and Login

**Description:** Every action except browsing needs login. Each Account has exactly one role.

#### FR-14: Register

A visitor can register a Customer Account.

**Consequences (testable):**
- The new Account can log in straight away as a Customer.
- Seller and Admin Accounts cannot be created by registration.

#### FR-15: Seeded accounts

The system provides ready-made demo Accounts for a sample of dataset Customers and Sellers, plus exactly one Admin Account.

**Consequences (testable):**
- Logging in as a seeded Seller shows that Seller's Historical Orders in FR-8.

#### FR-16: Role-based access

Each role reaches only its own functions: Customer → Checkout, payment, own Orders; Seller → own Orders and Seller Revenue; Admin → Ops Dashboard and Vouchers (FR-19).

**Consequences (testable):**
- A Customer calling a Seller or Admin function is refused.

### 4.7 Data Import and Replay

**Description:** The system starts from real Olist data. Import loads reference data once. Replay then streams Historical Orders as events in time order, so every downstream view fills up as if the marketplace were live.

#### FR-17: Import reference data

The builder can load Products, Categories, Sellers, and Customers from the Olist CSVs with one command.

**Consequences (testable):**
- Running Import twice does not create duplicates.
- Historical Orders are not loaded by Import; they arrive only through Replay. Before the first Replay, Seller Revenue and the Ops Dashboard are empty.

#### FR-18: Replay Historical Orders

The builder can run Replay with `make replay`, separately from the running services. Replay publishes each Historical Order's status changes as events, in purchase-time order.

**Consequences (testable):**
- The builder can choose a start date and an end date; only Historical Orders purchased in that range are replayed.
- Default speed is 1 day of data per 1 second.
- Seller Revenue (FR-9) and the Ops Dashboard (FR-10 to FR-12) update while Replay runs.
- Stopping and restarting Replay does not double-count any Historical Order.

### 4.8 Voucher Management

**Description:** The Admin creates Vouchers so Customers can test Voucher payment and compensation.

#### FR-19: Create Vouchers

An Admin can create Vouchers with a fixed value.

**Consequences (testable):**
- Each new Voucher has a unique code and starts unused.
- A Voucher's value cannot be changed after it is used.

## 5. Cross-Cutting Non-Functional Requirements

These are the real learning targets. Each one maps to an interview demo in §8.

- **NFR-1 Reliability — no lost Orders.** If any service is down while Orders are placed, every Order still leaves `created` (to `processing` or `canceled`) once that service is back. No event is lost between a database write and its publication.
- **NFR-2 Idempotency.** Every consumer of an event tolerates receiving it more than once with no extra effect (no double charge, no double revenue, no double count).
- **NFR-3 Observability.** One Checkout produces one distributed trace spanning at least 4 services. Logs carry the trace id. Each service exposes basic health and request metrics.
- **NFR-4 Read-model freshness.** During Replay at default speed, the Ops Dashboard is no more than 5 seconds behind the events published.
- **NFR-5 Data ownership.** Each service owns its data and reads no other service's database. Cross-service data arrives through APIs or events only.
- **NFR-6 One-command local run.** The whole system, including Kafka and databases, starts locally with one command.
- **NFR-7 Basic security.** Passwords are stored hashed. Login tokens expire. All functions except browsing require a valid token.

## 6. Non-Goals

- Not a production system and not a Shopee feature clone. "Mini Shopee" means real data and real flows, not breadth of features.
- No real payments, no real carrier, no shipping domain (freight is not revenue here).
- No product reviews, no stock/inventory, no cart, no flash sales, and no Vouchers beyond fixed single-use codes.
- No full-text search, no email/SMS notifications, no multi-currency, no mobile app.
- No Seller self-registration and no Seller product management.

## 7. MVP Scope

### 7.1 In Scope

Features 4.1–4.8 (FR-1 to FR-19), NFR-1 to NFR-7, a simple web UI for Customer, Seller, and Admin, local run with one command, and Kubernetes manifests in week 8.

### 7.2 Out of Scope for MVP (deferred)

- Stock reservation and an oversell scenario (would make concurrency practice richer).
- Shopee-like features: cart, flash sale, richer vouchers.
- Estimated delivery date and late-delivery rate.

## 8. Success Metrics

**Primary — live demos by week 8**
- **SM-1 No lost Orders:** stop the payment service, place Orders, restart it; every Order leaves `created`. Validates FR-2, FR-3, NFR-1.
- **SM-2 Compensation:** with the card gateway set to decline, an Order paid with Voucher + card ends `canceled` and the Voucher is usable again. Validates FR-3.
- **SM-3 One trace:** a single Checkout shows as one trace across at least 4 services. Validates NFR-3.
- **SM-4 Live read model:** during Replay, Ops Dashboard numbers change within 5 seconds. Validates FR-12, FR-18, NFR-4.

**Secondary**
- **SM-5 Explainability:** the builder can explain every service boundary and every saga step without notes.

**Counter-metrics (do not optimize)**
- **SM-C1 Feature count and UI polish.** More screens do not mean more learning. Counterbalances SM-5.
- **SM-C2 Replay throughput.** Faster than 1 day per second is not a goal if it costs correctness (NFR-2). Counterbalances SM-4.

## 9. Open Questions

1. Does login live only in the gateway, or in a separate identity service? → Architecture.
2. How many seeded Customer and Seller Accounts, and how are their passwords shared? → Architecture/epics.
3. UI technology. → Architecture or UX.

## 10. Assumptions Index

All assumptions made during drafting were confirmed by the builder on 2026-10-30:

- §4.2 FR-7 — Customer sees status changes after a page refresh; live push not required.
- §4.4 FR-11 — Products without a Category show as "Uncategorized".
- §4.4 FR-12 — Dashboard auto-refreshes every few seconds rather than live push.
- §5 NFR-4 — Dashboard lag target of 5 seconds.
