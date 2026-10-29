---
title: "Product Brief: Olist Marketplace"
status: draft
created: 2026-10-29
updated: 2026-10-29
---

# Product Brief: Olist Marketplace

## Executive Summary

Olist Marketplace is a small e-commerce marketplace built as Go microservices on top of the public Olist Brazilian E-Commerce dataset (about 100k real orders from 2016–2018). Customers browse products, place orders, and pay. Sellers see their own orders and revenue. Ops/admin watch an analytics dashboard.

This is a solo learning project with about 8 weeks to build it. The goal is interview preparation, not a business. The product exists to give real reasons to practice distributed-systems patterns: service boundaries, database-per-service, asynchronous events, saga, transactional outbox, CQRS, and observability. A monolith would be simpler. Microservices are a deliberate requirement.

What makes the project more than a toy is the data. The real Olist history is imported once. A replayer then streams historical orders as live events, so analytics and observability run on realistic volume rather than a handful of hand-made test orders.

## The Problem

Most microservices practice projects fail in one of two ways. Either they are CRUD services wired together with nothing distributed to reason about, or the data is so thin that patterns like CQRS and outbox never come under real load. In an interview, the questions that matter are about failure. What happens to an order when payment-service is down? How do you avoid double-charging? How do you trace one request across services? A project that cannot show the answers running does not help.

## The Solution

A marketplace with a narrow set of user flows, each chosen because it exercises a specific pattern:

| Flow | Pattern it exercises |
|---|---|
| Customer places an order and pays | Saga (order → payment) with compensation; outbox for reliable event publishing |
| Payment through a simulated gateway (configurable failure rate, multiple payments per order, installments, boleto that can expire) | Failure handling, idempotency, compensation |
| Customer tracks delivery status | Event-driven state changes across services |
| Seller views own orders and revenue | Data owned by several services, queried through a read model |
| Ops dashboard: revenue by state and category, late-delivery rate | CQRS read model built from the event stream |
| Historical replay of the Olist CSVs as events | Realistic event volume for analytics and tracing |

There is a simple web UI for all three roles. Login uses JWT with three roles: customer, seller, admin. Demo accounts are seeded from a sample of the dataset's existing customers and sellers, and new customers can register themselves.

## Who This Serves

- **The builder (primary).** A developer preparing for backend/microservices interviews who needs a project they can explain end to end and demo live.
- **Interviewers (audience).** They judge the project in a few minutes, mostly by how failures are handled.
- **In-product roles.** Customer, seller, and ops/admin. These exist so the flows are realistic, not because they are real users.

## Success Criteria

The project succeeds if all four scenarios can be demoed live by week 8:

1. **No lost orders.** Stop payment-service, place orders, restart it. Every order completes, because events wait in the outbox.
2. **Automatic compensation.** A payment failure (forced through the gateway's failure rate) or an expired boleto moves the order to cancelled without manual action.
3. **One trace across services.** A single order shows as one distributed trace spanning at least 4 services.
4. **Live read model.** While the replayer runs, the ops dashboard updates within seconds. `[ASSUMPTION]` Target: under 5 seconds behind.

A secondary criterion: the builder can walk through every service boundary and explain why it sits where it does.

## Scope

**In (MVP):**
- Browse products
- Place order and pay
- Track delivery status
- Seller orders and revenue
- Ops analytics dashboard
- JWT auth
- Simple UI
- One-time CSV import
- Event replayer
- Docker Compose for local runs; Kubernetes manifests in week 8

**Out:**
- Product reviews (removed entirely)
- Stock/inventory (stretch goal only)
- Full-text search
- Real shipping integration
- Email/SMS notifications
- Multi-currency

**Simplicity rule.** When a feature competes with learning a pattern, the pattern wins and the feature gets cut. The replayer does not run the saga. It publishes historical events straight to the bus for analytics. Only orders placed through the UI or API go through the saga.

## Open Questions

- NATS or Kafka. Decide in the architecture phase.
- How delivery status moves forward without real shipping. `[ASSUMPTION]` A simulator or admin action advances orders through shipped → delivered.
- Whether auth lives in api-gateway alone or in a separate identity service.
