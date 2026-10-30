---
title: "Addendum: Olist Marketplace PRD"
created: 2026-10-29
updated: 2026-10-30
---

# Addendum: Olist Marketplace PRD

## Technology decisions

- **Message broker: Kafka.** Chosen over NATS during PRD discovery because Kafka is the learning target.

## Deferred ideas

- Shopee-like features (cart, vouchers, flash sale): deferred to after MVP.
- Stock/inventory and an oversell (concurrency) scenario: deferred; would add a reserve-stock step to the saga.
- BMAD TEA module: not used.
