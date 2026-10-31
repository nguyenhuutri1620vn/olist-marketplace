# Epic 1 Context: Browse the real Olist catalog and sign in

<!-- Compiled from planning artifacts. Edit freely. Regenerate with compile-epic-context if planning docs change. -->

## Goal

A visitor browses real Olist Products by Category, registers as a Customer or logs in with a seeded Customer, Seller or Admin Account, and each role sees only its own navigation. Before any feature lands, the existing Go scaffold is reshaped to the architecture's seven-service set and the whole platform (services, Postgres, Kafka, observability) starts locally with one command, so every later story lands in the right service with logging, tracing, health and metrics already in place.

## Stories

- Story 1.1: Align the repo scaffold and start the platform with one command
- Story 1.2: Observability baseline in every service
- Story 1.3: Import catalog reference data
- Story 1.4: Register and log in
- Story 1.5: Seed demo Accounts
- Story 1.6: Gateway authentication and role-based access
- Story 1.7: Web app shell, login and register
- Story 1.8: Browse and filter Products

## Requirements & Constraints

- The whole system, including Kafka and databases, starts locally with one command (`make up`).
- Each service owns its data and never reads another service's database; cross-service data arrives only via gRPC APIs or Kafka events.
- Every service logs JSON with the trace id, traces with OpenTelemetry, and exposes `/healthz`, `/readyz` and `/metrics` on one ops port.
- Passwords are bcrypt-hashed; login tokens expire (1 h, no refresh); everything except browsing requires a valid token.
- Registration creates Customer Accounts only; seeded Accounts cover a sample of dataset Customers and Sellers plus exactly one Admin.
- Product display name is `<Category English name> #<first 4 chars of product id>`; price is the most recent dataset Order Item price. Import is idempotent and loads no Historical Orders.

## Technical Decisions

- **Service set (exactly seven):** api-gateway (no data), identity-service (Accounts, credentials, signing keys), catalog-service (Product, Category, Seller profile), order-service, payment-service, shipping-simulator (own DB), analytics-service (read models only). `customer-service` becomes `identity-service`; `review-service` is removed.
- **Hexagonal layout per service:** `cmd/server` (wiring only), `internal/domain` (imports nothing internal), `internal/service` (use cases, defines ports), `internal/repository` (Postgres adapters), `internal/transport` (gRPC/HTTP/Kafka handlers), plus `migrations/`.
- **Database per service:** one Postgres 18 instance, one database and one DB user per data-owning service, no grants on other databases. Each service embeds its `migrations/` (golang-migrate v4, iofs, pgx/v5 driver) and applies them at startup.
- **Transport:** browser → api-gateway REST/JSON; gateway → services gRPC with contracts in `proto/<service>/v1`, generated Go in `proto/gen/go`, checked by `buf lint` and `buf breaking`. Import tool calls owners' admin gRPC RPCs (idempotent upserts keyed by dataset id).
- **Network:** one compose network; only api-gateway, web (from Story 1.7) and observability UIs publish host ports; service gRPC ports and Postgres are not published.
- **Identity:** RS256 JWT with claims `sub`, `role`, `kid`, served JWKS at `/.well-known/jwks.json`; gateway verifies and enforces role, forwards the JWT in gRPC metadata; services re-verify against cached JWKS. Seeded Accounts use dataset ids; new Accounts use UUIDv7.
- **Conventions:** uuid ids; int64 centavos for money; `timestamptz` UTC; gRPC status codes between services; gateway error body `{ "error": { "code", "message" } }`; config via environment variables.
- **Stack:** Go 1.27.x, PostgreSQL 18.x (volume at `/var/lib/postgresql`), Kafka 4.3.x KRaft, franz-go v1.22.x, pgx v5, golang-migrate v4.20.x, OTel Go SDK v1.47.x, Jaeger v2.22.x, Prometheus 3.x, Grafana 13.x, kafbat kafka-ui v1.5.x, React 19 + Vite 8.
- **Repo seeds needed early:** `contracts/events/v1/` (only source of event/command types), `pkg/{logger,tracing,eventbus,outbox,inbox}/`.

## UX & Interaction Patterns

- Role-specific navigation: Customer, Seller and Admin each see only their own menu after login; a visitor can browse Products without logging in.
- The `web` React SPA container and its host port arrive in Story 1.7, not earlier.

## Cross-Story Dependencies

- Story 1.1 is the foundation for all stories: service names, DB-per-service, migrations runner, proto layout and `make up`.
- Story 1.2 builds on 1.1's services and compose (Prometheus scrape targets, Grafana data sources, gRPC health checks).
- Stories 1.3 and 1.5 depend on catalog and identity admin import RPCs; 1.6 depends on identity's JWKS from 1.4; 1.7–1.8 depend on gateway routes from 1.6.
- Epic 2 reuses `pkg/outbox`, `pkg/inbox` and `contracts/events/v1` seeded here.
