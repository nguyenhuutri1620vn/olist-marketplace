---
id: SPEC-olist-marketplace
companions:
  - glossary.md
  - ../../planning-artifacts/prds/prd-olist-marketplace-2026-10-29/prd.md
  - ../../planning-artifacts/prds/prd-olist-marketplace-2026-10-29/addendum.md
  - ../../planning-artifacts/architecture/architecture-olist-marketplace-2026-10-30/ARCHITECTURE-SPINE.md
sources:
  - ../../planning-artifacts/briefs/brief-olist-marketplace-2026-10-29/brief.md
  - ../../planning-artifacts/briefs/brief-olist-marketplace-2026-10-29/addendum.md
---

> **Canonical contract.** This SPEC and the files in `companions:` are the complete, preservation-validated contract for what to build, test, and validate. Source documents listed in frontmatter are for traceability — consult them only if you need narrative rationale or prose color this contract intentionally omits.

# Olist Marketplace MVP

## Why

A vision to realize. One developer is preparing for backend interviews and needs a "mini Shopee", built as Go microservices on the real Olist Brazilian E-Commerce dataset. It lets them practice service boundaries, database-per-service, Kafka events, saga, outbox, CQRS, observability, concurrency, and the BMAD method. The roles (Customer, Seller, Admin) exist to make the flows realistic; nobody depends on the system. Every trade-off resolves toward what the builder can explain and demo under failure in an interview.

## Capabilities

The testable consequences for each capability are the PRD FRs it cites. Terms follow `glossary.md`.

- **CAP-1** — Checkout and payment (FR-1–FR-4)
  - **intent:** A Customer checks out Products from several Sellers, then pays each resulting Order separately by Card Payment, Voucher, or both.
  - **success:** A 2-Seller Checkout creates exactly 2 `created` Orders. A paid Order reaches `processing`. A declined card cancels the Order and makes its Voucher usable again. A repeated payment request charges once.
- **CAP-2** — Delivery tracking (FR-5–FR-7)
  - **intent:** A Seller ships their own processing Orders. A simulator delivers them. A Customer follows each Order's status timeline.
  - **success:** `approved` becomes `processing` without user action. Only the owning Seller can ship. `shipped` becomes `delivered` within the configured delay (default 3 s). A Customer sees only their own Orders.
- **CAP-3** — Seller orders and revenue (FR-8–FR-9)
  - **intent:** A Seller lists their Orders by Order Status and sees Seller Revenue for a purchase-date range, across Historical Orders and new Orders.
  - **success:** Only `delivered` Orders count. Revenue is price only. New Orders deduct the applied Voucher, with a floor of 0. Historical Orders ignore Vouchers. Example: 100 with a 30 Voucher yields 70.
- **CAP-4** — Ops dashboard (FR-10–FR-12)
  - **intent:** An Admin sees Seller Revenue by Seller state and by Category, plus Order counts per Order Status, for a purchase-date range.
  - **success:** State totals equal the all-Seller total. Products with no Category appear as "Uncategorized". Counts sum to all Orders in range and update during Replay without a page reload.
- **CAP-5** — Product browsing (FR-13)
  - **intent:** Anyone browses Products filtered by Category, with a derived display name and price.
  - **success:** Name is `<Category English> #<4 chars of id>`. Price is the most recent dataset Order Item price. The Category filter returns only that Category.
- **CAP-6** — Accounts and login (FR-14–FR-16)
  - **intent:** Visitors register as Customers. Seeded Customer, Seller and single Admin Accounts log in. Each role reaches only its own functions.
  - **success:** Registration creates Customers only. A seeded Seller sees their Historical Orders. A Customer calling a Seller or Admin function is refused.
- **CAP-7** — Data import and replay (FR-17–FR-18)
  - **intent:** The builder imports reference data once, then replays Historical Orders as events over a chosen date range.
  - **success:** Import twice creates no duplicates and loads no Orders. `make replay` takes from/to dates at a default of 1 day of data per second. Revenue and the dashboard fill while it runs. A restart double-counts nothing.
- **CAP-8** — Voucher management (FR-19)
  - **intent:** An Admin creates fixed-value, single-use Vouchers.
  - **success:** Each Voucher has a unique code and starts unused. Its value cannot change after use. Partial use is impossible.

## Constraints

- Microservices are a requirement, not up for debate. The stack is Go, Postgres database-per-service, Kafka, Docker Compose, and a React + Vite UI. Kubernetes manifests come in week 8. The whole project is about 8 weeks, solo.
- NFR-1 to NFR-7 bind every capability:
  - no lost Orders while a service is down
  - idempotent event consumers
  - one trace across at least 4 services
  - Ops Dashboard at most 5 s behind events during Replay
  - no cross-service database reads
  - one-command local run
  - hashed passwords and expiring tokens
- Architecture decisions AD-1 to AD-17 in the spine are binding.
- Simplicity wins. When a feature competes with learning a pattern, the feature is cut.
- New Orders are always single-Seller. Historical Orders may span Sellers and never run the Order Saga.
- Payment methods are a simulated card plus a Voucher only. Any payment failure cancels the Order.

## Non-goals

- A production system or a Shopee feature clone. "Mini Shopee" means real data and real flows.
- Real payments, a real carrier, or a shipping domain. Freight is not revenue.
- Reviews, stock/inventory, cart, flash sales, installments, boleto, estimated delivery dates, and the late-delivery rate.
- Full-text search, email/SMS, multi-currency, a mobile app, Seller self-registration, and Seller product management.

## Success signal

By week 8 the builder can run four live demos and explain every service boundary and saga step without notes:

- stop payment-service and every Order still leaves `created` after restart
- a declined card cancels the Order and returns the Voucher
- one Pay request shows as a single trace across at least 4 services
- the Ops Dashboard moves within 5 s while Replay runs
