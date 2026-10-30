---
review: adversarial
target: ../ARCHITECTURE-SPINE.md
context: ../../../prds/prd-olist-marketplace-2026-10-29/prd.md
date: 2026-10-30
---

# Adversarial Review — Architecture Spine (olist-marketplace)

## Verdict

The spine sets the right paradigm (owner-per-entity, outbox, inbox, orchestrated saga), but it fixes the *mechanisms* and leaves the *contracts* open. Two teams can follow every AD to the letter and still disagree on who receives "Pay", what goes on which topic, which events exist, and what each event carries. Most findings below come from one root cause: **there is no single event and command catalog**. Closing C-1 to C-3 and H-1 to H-4 before epics would remove most of the divergence risk.

Method: for each finding, Unit A and Unit B are two plausible builds that both satisfy every AD as written, yet cannot work together. The fix is a new or tightened AD rule.

---

## Critical

### C-1 — Who receives the "Pay" request, and when the saga starts

- **Unit A (payment-service dev):** AD-3 allows `api-gateway → services` over gRPC and the seed diagram draws `GW → PAY`. They expose `PaymentService.Pay(order_id, voucher_code, card, idempotency_key)`, charge, and emit `PaymentSucceeded` on `payment.events`.
- **Unit B (order-service dev):** AD-4 says order-service "emits `AuthorizePayment`". They expose `OrderService.PayOrder`, write `AuthorizePayment` to the outbox, and wait for the reply.
- **Clash:** in A, order-service gets a `PaymentSucceeded` for a saga it never started. payment-service also cannot check that the caller owns the Order or that it is still `created`, because it has no Order data (an auth gap; see H-8). In B, nothing calls payment-service's `Pay`. AD-4 also does not say whether `AuthorizePayment` goes out at Checkout or at Pay time. Checkout and Pay are separate user actions under FR-1 and FR-2.
- **AD gap:** AD-3 and AD-4 do not name the entry point for FR-2, or what in the command starts it.
- **Fix (tighten AD-4):** "FR-2 enters only through `OrderService.PayOrder` (gRPC from the gateway). order-service checks ownership (`customer_id` = caller) and `status = created`. It then writes `AuthorizePayment{order_id, amount_total, voucher_code?, card_token?, idempotency_key}` to its outbox. payment-service exposes **no** gRPC for paying; its gRPC surface is Voucher admin (FR-19) only. Checkout does not start the saga."

### C-2 — Commands have no topic

- **Unit A:** AD-8 says "one topic per domain", so `AuthorizePayment` (an order-service output) goes on `order.events`. payment-service subscribes to `order.events` and filters by `event_type`.
- **Unit B:** AD-8 frames these as domain topics, so `AuthorizePayment` belongs to the payment domain and goes on `payment.events`. payment-service then consumes its own topic.
- **Clash:** the command is published to one topic and consumed from another, so it is never received. Option A also means analytics and shipping (which consume `order.events`) receive a payment command, and any careless switch on `event_type` treats it as an Order fact.
- **AD gap:** AD-8 names topics for events only. Commands are mentioned (`AuthorizePayment`) but have no topic.
- **Fix (tighten AD-8):** "Commands go on `<target>.commands` (`payment.commands`; `shipping.commands` if one is ever needed). Facts go on `<producer>.events`. A topic carries exactly one direction. The full list of event and command types, with their producer and consumers, lives in `contracts/events/v1` (see H-1). A type not in that list must not be published."

### C-3 — Historical Orders re-published on `order.events` trigger live reactions

- **Unit A (shipping-simulator):** following AD-4's choreography, it consumes `OrderShipped` from `order.events` and emits `ShipmentDelivered` after N seconds.
- **Unit B (order-service, Replay path):** AD-12 says Historical Orders are "re-published through its outbox", so the order-service dev emits `OrderShipped` for Historical Orders too, with `historical=true`.
- **Clash:** shipping-simulator sends `ShipmentDelivered` for about 100k Historical Orders. order-service then either (a) applies `shipped→delivered` *before* Replay delivers the real delivered event, so the timeline gets a wall-clock timestamp, or (b) rejects it as an invalid transition after Replay has already delivered it, which floods logs and retries. Any future reactor (for example payment on `OrderCreated`) has the same problem. AD-9 puts the `historical` flag in the payload, but no rule says a reactor must check it.
- **AD gap:** no rule says which consumers must ignore Historical events.
- **Fix (new AD):** "Process reactors (payment-service, shipping-simulator, and any saga or choreography step) ignore events with `historical=true`. Only read-model projectors (analytics, order-service's own lists) consume them. `historical` is an envelope field, not only a payload field, so a reactor can filter before decoding." Alternatively, give Historical re-publishes their own event types (`HistoricalOrderStatusRecorded`) that reactors never subscribe to.

---

## High

### H-1 — There is no event catalog, so each team invents its own status events

- **Unit A (order-service):** emits one generic `OrderStatusChanged{order_id, to}`. For approval, it emits only the final status, `processing`, because `approved→processing` happens in one transaction.
- **Unit B (analytics):** expects `OrderCreated`, `OrderApproved`, `OrderProcessing`, `OrderShipped`, `OrderDelivered`, `OrderCanceled`, and decrements the previous status bucket by type.
- **Clash:** FR-12 status counts are wrong. analytics never sees `OrderApproved`, and with A it cannot tell which bucket to decrement. shipping-simulator (C-3) has the same problem: does it match `OrderShipped` or `OrderStatusChanged where to=shipped`?
- **AD gap:** AD-8 fixes the envelope and naming *style*, not the set of types or their payloads.
- **Fix (new AD-13 "Event catalog"):** "`contracts/events/v1` (Go structs plus JSON Schema, imported by every producer and consumer through `pkg/eventbus`) is the only definition of event and command types. Order status facts are per-status (`OrderCreated` … `OrderCanceled`), and each one carries `from_status` and `to_status`. When `approved→processing` runs in one transaction, it emits **both** `OrderApproved` and `OrderProcessing`, in that order. Adding a field is a minor `event_version` bump. Renaming or removing a field is a new major version."

### H-2 — Which Order events carry the state that read models need

- **Unit A (order-service):** puts the items snapshot (`seller_id`, `seller_state`, `category`, `price`) only in `OrderCreated`, because that is when the data is captured. `voucher_amount` goes only in `OrderApproved`, because that is when it becomes known. `OrderDelivered` carries just `order_id`.
- **Unit B (analytics):** reads AD-9 as "Order events carry what read models need". It computes revenue statelessly on `OrderDelivered` from the payload and keeps no per-order state.
- **Clash:** B computes revenue 0, or panics, on every delivery. If B instead keeps per-order state but builds it late (after a projection rebuild from a compacted or partially consumed topic), it gets nothing.
- **AD gap:** AD-9 lists the fields but not *which events* carry them. `voucher_amount` exists only after payment, and `purchased_at` is not listed at all, even though FR-9 to FR-12 filter on purchase time.
- **Fix (tighten AD-9):** "Every `Order*` fact carries the full current Order snapshot: `order_id, customer_id, purchased_at, historical, status, voucher_amount (0 until approved), items[{product_id, seller_id, seller_state, category_en ('Uncategorized' if none), price, freight}]`. Consumers must not depend on having seen an earlier event for the same order." This is acceptable for one-Seller orders and small multi-Seller Historical Orders.

### H-3 — The outbox relay can reorder events for one Order

- **Unit A:** the relay runs several workers (or two replicas of order-service run during a compose restart). Each worker runs `SELECT … FOR UPDATE SKIP LOCKED LIMIT 100`. Worker 1 locks `OrderCreated`, worker 2 skips it and publishes `OrderApproved` first.
- **Unit B (analytics):** relies on AD-8 "partition key is `order_id`" to get per-order order.
- **Clash:** a Kafka partition keeps the order in which events are *published*, not the order in which they were *committed*. analytics sees `OrderApproved` before `OrderCreated`, so counts go negative or the projection is lost. SKIP LOCKED is meant to let workers run in parallel, which is exactly what breaks ordering.
- **AD gap:** AD-6 does not require publishing in order per aggregate.
- **Fix (tighten AD-6):** "Outbox rows carry a monotonic `seq` (bigserial). Either exactly one relay worker per service publishes in `seq` order, or the relay only publishes a row when no earlier unsent row exists for the same `aggregate_id`. The producer uses `acks=all`, idempotent producer, `max.in.flight=1` (or franz-go's ordered idempotent producer)." Also, as defense in depth: "Projectors apply a status event only if `to_status` is reachable from the stored status. Otherwise they keep it pending or ignore it, and never decrement below zero."

### H-4 — Replay message shape and Olist status mapping are undefined

- **Unit A (replayer):** emits one `replay.orders` message per Olist row (`order_status`). It passes through Olist statuses `invoiced`, `unavailable` and `created`, and the raw Portuguese category, with no seller state.
- **Unit B (order-service):** expects one message per status change with PRD statuses only. It plans to enrich with `seller_state` and `category` from catalog, which AD-3 forbids ("no other synchronous call").
- **Clash:** order-service rejects `invoiced` and `unavailable`, or stores statuses the PRD does not have. It cannot fill in the AD-9 fields without breaking AD-3. FR-7 timelines need a timestamp per status, but Olist provides only `purchase`, `approved_at`, `delivered_carrier_date` and `delivered_customer_date`. Canceled orders have no cancel timestamp.
- **AD gap:** AD-12 names the topic but not the payload, the status mapping, or who does the enrichment.
- **Fix (tighten AD-12):** "The replayer emits one `HistoricalOrderStatusRecorded` per derived status change. It carries a full snapshot already enriched from the CSVs: seller state, English category, `customer_unique_id`, and price/freight in centavos. Mapping: purchase→`created`, `approved_at`→`approved`, `delivered_carrier_date`→`shipped`, `delivered_customer_date`→`delivered`. `invoiced`/`processing` map to `processing` at `approved_at`. `canceled`/`unavailable` map to `canceled` at the last known timestamp. Rows with no usable timestamp are skipped and counted in a replay report. Events are emitted in **status-timestamp order** (not purchase order), paced at the configured speed. order-service makes no calls to resolve them."

### H-5 — Voucher validation and concurrent use

- **Unit A (payment-service):** an unknown or already-used Voucher is a failure. It emits `PaymentFailed`, and order-service moves the Order to `canceled` (AD-4 offers only those two outcomes).
- **Unit B (UI/gateway dev):** expects a typo in a Voucher code to be a synchronous 4xx, with the Order staying `created` so the Customer can retry.
- **Concurrency pair:** two Orders apply the same Voucher at the same moment. Each worker reads `used=false` and marks it used. The result is one Voucher spent twice, which breaks FR-2 ("already used cannot be applied again").
- **AD gap:** AD-4 does not distinguish *validation* from *decline*, and no rule covers Voucher concurrency.
- **Fix (new rule under AD-4):** "Voucher redemption is a single conditional update: `UPDATE voucher SET used_by_order=$1 WHERE code=$2 AND used_by_order IS NULL`. Zero rows means `PaymentFailed{reason: VOUCHER_UNAVAILABLE}`. Any `PaymentFailed` cancels the Order, and the UI must say so. A restore resets `used_by_order` only when it equals this order_id." If cancel-on-typo is not wanted, add a read-only `ValidateVoucher` gRPC that order-service calls in `PayOrder` before emitting. That needs an explicit AD-3 exception.

### H-6 — Idempotency key scope, and "no double charge" with a different key

- **Unit A:** payment-service dedupes on `(idempotency_key)` only. The Customer double-clicks with a new key, or retries after a timeout, and gets two Payments for one Order.
- **Unit B:** order-service dedupes on `(order_id)`. It also emits a second `AuthorizePayment` (with a new `event_id`) when it sees a duplicate key with a different payload.
- **Clash:** FR-4 says "a repeated payment request for the same Order charges at most once", but AD-7 only says a key is "carried". Neither rule binds `order_id`, and nobody owns key storage.
- **Fix (tighten AD-7):** "The idempotency key is checked in order-service `PayOrder` (table `payment_requests(idempotency_key PK, order_id, request_hash)`). The same key with a different hash returns `ALREADY_EXISTS`. A second `PayOrder` for an Order that is no longer `created`, or that already has an `AuthorizePayment` in flight, is rejected (`FAILED_PRECONDITION`). payment-service enforces `UNIQUE(order_id)` on Card Payment and Voucher redemption as the last line of defense."

### H-7 — shipping-simulator in-memory state against outbox and NFR-1

- **Unit A:** following AD-1's "in-memory" option, it consumes `OrderShipped`, commits the offset, and sets a 3 s timer. It crashes and the Order stays `shipped` forever.
- **Unit B:** reads AD-6 ("every service that publishes events") and AD-7 (inbox) as requiring Postgres.
- **AD gap:** AD-1 explicitly allows a design that AD-6 and AD-7 forbid, and NFR-1's "no lost Orders" fails for the shipped stage.
- **Fix (tighten AD-1):** "shipping-simulator owns a DB (`shipments(order_id PK, deliver_at, delivered)`) with inbox and outbox like every other service. A poller emits `ShipmentDelivered` when `deliver_at <= now()`. The delay comes from env `SHIPPING_DELAY` (default 3s)." Remove "in-memory".

### H-8 — Trust boundary for gRPC metadata, and role propagation

- **Unit A (gateway):** passes only `x-account-id`, as AD-11 says.
- **Unit B (order-service):** "enforces ownership". For `ListOrders` it queries `WHERE customer_id=$id OR item.seller_id=$id`, because it has no role. For `MarkShipped` it checks only that the account owns a seller item. A Seller's account id equals `seller_id`, and a Customer's equals `customer_unique_id`. These are separate id spaces with no guarantee they never overlap.
- **Also:** every service's gRPC port is reachable on the compose network (and from the host if ports are published). Anyone can set `x-account-id` and bypass the gateway, which breaks NFR-7 ("all functions require a valid token").
- **Fix (tighten AD-11):** "The gateway forwards `x-account-id` **and** `x-role`. Services use the role to choose the ownership predicate (customer → `customer_id`; seller → `item.seller_id`). Service gRPC ports are not published to the host. Services accept metadata only from the gateway: a shared internal network, plus either forwarding the original JWT for re-verification against the cached public key, or a gateway-signed header. A service that receives no identity metadata on a non-public RPC returns `UNAUTHENTICATED`." Also specify how the public key is published (`GET /.well-known/jwks.json` or a gRPC `GetPublicKeys`, fetched at startup and refreshed when an unknown `kid` arrives).

### H-9 — Order total, freight, and "Voucher value applied"

- **Unit A (order-service):** `amount_total = Σ price` (FR-9 says revenue excludes freight).
- **Unit B (payment-service):** charges `Σ(price + freight)`, which matches "Order Item, with price and freight value".
- **Also:** new Orders have no freight source. catalog has no freight, and AD-3's Checkout snapshot lists price, Seller and Category only. And when a 50 Voucher pays a 30 Order, is `voucher_amount` 50 (face value) or 30 (applied)? FR-9's floor-0 rule gives the same revenue either way, but FR-10 totals and payment records diverge.
- **Fix (new convention):** "`order_total = Σ(price + freight)`. Freight for new Orders is snapshotted from catalog (the Product's most recent dataset `freight_value`, or 0; decide one). `voucher_amount` in events is the **applied** amount, `min(face, order_total)`. Card charges `order_total − voucher_amount`. Revenue per Order is `max(0, Σprice − voucher_amount)` for new Orders."

---

## Medium

### M-1 — Trace propagation: `trace_id` alone cannot link spans, and checkout is not 4 services

- **Unit A:** follows AD-8 literally. It puts `trace_id` in the JSON envelope, and the consumer starts a *new* root span tagged with that id.
- **Unit B:** uses OTel's Kafka propagator (W3C `traceparent` in Kafka headers), but the relay runs later in its own goroutine with no context, so it injects the relay's own span.
- **Clash:** Jaeger shows disconnected traces. Also, under C-1 the Checkout trace covers gateway, order and catalog (3 services). Payment is a separate request and so a separate trace, so SM-3/NFR-3 ("one Checkout, ≥4 services") fails unless `OrderCreated` consumers (analytics) join it.
- **Fix:** "The envelope carries `traceparent` (W3C), not bare `trace_id`. The outbox row stores the `traceparent` captured in the use case, and the relay injects it into Kafka headers. Consumers start a child span from it. NFR-3 is demonstrated on the Checkout+Pay path, or the PRD's 'one Checkout' is read as the Checkout request plus all its asynchronous consequences."

### M-2 — `occurred_at` against `purchased_at` against wall clock for Historical events

- **Unit A (order-service):** sets `occurred_at = now()` when re-publishing Historical events (the outbox write time).
- **Unit B (analytics/UI):** uses `occurred_at` for the FR-7 timeline and FR-10 to FR-12 date filters.
- **Clash:** every Historical Order appears to have happened today, so date-range filters return nothing.
- **Fix:** "`occurred_at` is the business time of the fact (for Historical Orders, the dataset timestamp). `published_at` is a separate envelope field for lag measurement (NFR-4). Date-range queries filter on `payload.purchased_at`."

### M-3 — Poison and invalid-transition events

- **Unit A:** on an invalid transition (for example a duplicate `ShipmentDelivered` with a new `event_id`, or an out-of-order event), returns an error. The franz-go loop retries forever and the partition stalls, which also stalls NFR-4.
- **Unit B:** logs and commits, silently dropping a legitimate early event.
- **Fix (tighten AD-7):** "Handler outcomes: `applied`, `duplicate` (inbox hit), `rejected` (domain-invalid: record in inbox with reason, ack, increment a metric), `retryable` (infrastructure error: back off, do not commit). After N retryable failures, move the message to `<topic>.dlq`."

### M-4 — Importer writes into two services' private schemas

- **Unit A (catalog dev):** renames `products.category` to `category_key` in a catalog migration.
- **Unit B (importer dev):** writes SQL against the old column, using which DB user? AD-2 says each user is scoped to one DB.
- **Clash:** the importer breaks silently. AD-2's "no service ships migrations for another" is followed in letter, but the coupling is the same.
- **Fix (tighten AD-12):** "The importer calls each owner's admin gRPC (`catalog.ImportProducts`, `identity.ImportAccounts`, as idempotent upserts keyed by dataset id), or it is a subcommand compiled inside each service's module (`catalog-service import`). It never uses SQL against another service's schema. Import is idempotent by natural-key upsert (FR-17)."

### M-5 — Checkout idempotency and partial failure

- **Unit A:** creates one Order per Seller in separate transactions. A crash after the 1st of 3 leaves a partial Checkout.
- **Unit B (UI):** retries on timeout and gets a duplicate set of Orders.
- **Fix:** "Checkout writes all its Orders, plus their `OrderCreated` outbox rows, in one order-service transaction and accepts a `checkout_idempotency_key`. Each Order carries `checkout_id`."

### M-6 — Deterministic `event_id` is not specified precisely

- AD-7 says "derived from `order_id` + status", but IDs must be uuid. Unit A uses UUIDv5 with namespace X, and Unit B (a later replayer refactor) uses a different namespace or input format. A restart then double counts (FR-18).
- On the order-service side, does its re-publish reuse the replay `event_id` or mint UUIDv7? If it mints a new one and its inbox is ever reset, analytics double counts.
- **Fix:** "Replay `event_id = UUIDv5(NS_OLIST_REPLAY (fixed constant in contracts), order_id || ':' || status)`. order-service's re-publish of a Historical event reuses the incoming `event_id`. Projectors are also state-idempotent: upsert per `(order_id, status)`."

### M-7 — Seller state and Category: the snapshot key against the display name

- **Unit A (order-service):** snapshots the category *key* (`cama_mesa_banho`), or null.
- **Unit B (analytics):** groups by English name and puts null under "Uncategorized".
- **Clash:** FR-11 shows Portuguese keys, or two buckets for the same Category.
- **Fix:** covered by H-2: the snapshot carries `category_en`, with "Uncategorized" resolved by catalog at snapshot time.

---

## Low

- **L-1 — The card decline setting is env-only.** SM-2 needs the decline rate toggled. "Config: env only" forces a restart of payment-service, which also exercises NFR-1 and may be acceptable. Fix: allow an admin gRPC `SetGatewayDeclineRate` on payment-service, or document the restart as part of the demo.
- **L-2 — Relay poll interval and NFR-4.** No bound is set. Fix: relay poll ≤ 200 ms (env), analytics commits per batch, dashboard refresh ≤ 2 s, so total lag stays ≤ 5 s.
- **L-3 — Health endpoint is unnamed.** NFR-3 needs "basic health". Fix: `/healthz` (liveness) and `/readyz` (DB and Kafka reachable) on the same port as `/metrics`, used by compose healthchecks.
- **L-4 — No card data in events.** Fix: `AuthorizePayment` carries only a simulated `card_token` or test-card enum, never a PAN-like value.
- **L-5 — Orders stuck in `created` before payment.** Unpaid new Orders stay `created` indefinitely, which makes FR-12 counts drift. Fix: decide whether an abandoned Order expires (add a `created→canceled` timeout rule) or is accepted as is. Add the rule to AD-5 either way.
- **L-6 — "Historical Orders may enter at any status".** It is unclear whether a Historical Order, once ingested, must still follow the transition machine for later replay events. Fix: "Historical Orders follow the same machine, except that the first event may be any status. A later event must be forward-reachable."
- **L-7 — Proto contract ownership.** `proto/<service>/v1` is fine, but nobody owns generated code or breaking-change rules. Fix: use `buf` with a breaking-change check in CI, and generate code into `proto/gen/go`.

---

## PRD Coverage Check

| Req | Covered by | Gap |
| --- | --- | --- |
| FR-1 | AD-3 checkout gRPC | Atomicity and idempotency of the multi-Order Checkout (M-5) |
| FR-2 | AD-4 | Entry point (C-1); Voucher validation and concurrency (H-5); order total (H-9) |
| FR-3 | AD-4 | OK once H-5 is fixed |
| FR-4 | AD-7 | Key scope and order_id uniqueness (H-6) |
| FR-5 | AD-5 | Seller ownership needs role (H-8); event for `processing` (H-1) |
| FR-6 | AD-3 | Durability of the shipping simulator (H-7); configurable delay not stated |
| FR-7 | AD-10 | Status timeline storage and timestamps not stated (M-2) |
| FR-8 | AD-10, AD-11 | Role-based predicate (H-8); per-item filtering for multi-Seller Historical Orders implied, not stated |
| FR-9 | AD-9, AD-10 | Event field placement (H-2); applied Voucher amount (H-9) |
| FR-10/11 | AD-9 | `category_en` and "Uncategorized" (M-7) |
| FR-12 | AD-9 | Status event catalog (H-1); ordering (H-3) |
| FR-13 | AD-1 only | Derived name and price rule (most recent dataset price) not assigned to importer or catalog. Add one line to AD-12. |
| FR-14 | AD-11 | OK (registration creates Customer role only; state it) |
| FR-15 | AD-11, AD-12 | Seeded password distribution deferred to epics; acceptable |
| FR-16 | AD-11 | Gateway trust boundary (H-8) |
| FR-17 | AD-12 | Idempotent upsert not stated; schema coupling (M-4) |
| FR-18 | AD-7, AD-12 | Replay payload, mapping and pacing (H-4); `event_id` derivation (M-6) |
| FR-19 | AD-1 | "Value immutable after use" is fine in payment-service; gRPC surface not listed |
| NFR-1 | AD-4, AD-6 | shipping-simulator in-memory (H-7) |
| NFR-2 | AD-7 | Ordering (H-3); state-idempotent projectors (M-6) |
| NFR-3 | AD-8 | `traceparent` across the outbox; 4-service trace path (M-1); health (L-3) |
| NFR-4 | none | No relay and refresh budget (L-2) |
| NFR-5 | AD-2 | Importer bypass (M-4) |
| NFR-6 | Structural Seed | OK |
| NFR-7 | AD-11 | Direct gRPC bypass of the gateway (H-8) |

## Suggested New and Tightened ADs (summary)

1. **AD-13 Event & command catalog** (`contracts/events/v1`): every type, producer, consumers, full payload. Closes H-1, H-2, M-7, and part of C-2.
2. **AD-8+**: `<target>.commands` against `<producer>.events`; `traceparent`, `historical` and `published_at` in the envelope. Closes C-2, C-3, M-1, M-2.
3. **AD-4+**: `PayOrder` is the only entry point; Voucher conditional redemption; validation failure cancels the Order. Closes C-1, H-5.
4. **AD-6+**: ordered publish per aggregate. Closes H-3.
5. **AD-7+**: key scope, `UNIQUE(order_id)`, handler outcome classes, DLQ, UUIDv5 namespace. Closes H-6, M-3, M-6.
6. **AD-11+**: forward the role; services trust only gateway-originated identity; JWKS endpoint. Closes H-8.
7. **AD-12+**: Replay payload, status mapping and enrichment; importer through the owners' APIs. Closes H-4, M-4.
8. **AD-1 fix**: shipping-simulator has a DB. Closes H-7.
9. **Money convention+**: order total, freight source, applied Voucher amount. Closes H-9.
