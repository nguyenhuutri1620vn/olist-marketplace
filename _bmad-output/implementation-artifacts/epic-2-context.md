# Epic 2 Context: Checkout and pay, with a saga that survives failure

<!-- Compiled from planning artifacts. Edit freely. Regenerate with compile-epic-context if planning docs change. -->

## Goal

A Customer checks out Products from several Sellers in one action (one Order per Seller), pays each Order by card, Voucher or both, and follows its status timeline. A declined card cancels the Order and gives the Voucher back, and a payment-service outage loses nothing: every paid Order still leaves `created` once the service returns. The Admin creates Vouchers and sets the simulated card decline rate. The epic first lays the reliability plumbing (event envelope, transactional outbox relay, idempotent inbox consumers) that every later event-driven feature reuses, and ends with scripted demos proving no lost Orders, compensation, and one trace across services.

## Stories

- Story 2.1: Event envelope and transactional outbox relay
- Story 2.2: Idempotent inbox consumers
- Story 2.3: Checkout creates one Order per Seller
- Story 2.4: Customer sees own Orders and status timeline
- Story 2.5: Select Products and check out in the web app
- Story 2.6: Admin manages Vouchers and the card decline rate
- Story 2.7: Request payment for an Order
- Story 2.8: Authorize payment and approve the Order
- Story 2.9: Compensation when payment fails
- Story 2.10: Pay form and payment states in the web app
- Story 2.11: Prove no lost Orders when payment-service is down

## Requirements & Constraints

- Checkout creates exactly one `created` Order per Seller, each with only that Seller's items; there is no cart and no quantity field (duplicate product ids are invalid).
- Pay one `created` Order by card, Voucher, or Voucher + card for the remainder. Covered Order moves to `approved` then automatically to `processing` (never stays `approved`). Paying one Order never changes sibling Orders from the same Checkout.
- Card decline cancels the Order and restores any Voucher; no Order stays `created` after a decline. A used Voucher cannot be applied again; Vouchers are fixed-value, single-use, never partially used, value immutable.
- Repeated payment request with the same idempotency key charges at most once and yields one Payment record.
- Customers see only their own Orders (another Customer's id → `NOT_FOUND`); status changes visible on refresh, no live push.
- No lost Orders: with any service down, every Order leaves `created` once it is back; no event lost between DB write and publish. Every consumer tolerates duplicate delivery with no extra effect.
- One Pay produces one distributed trace spanning gateway → order → payment → order (≥ 4 services completes in Epic 3); logs carry the trace id.
- Role access: Customer → Checkout, pay, own Orders, Voucher lookup; Admin → Vouchers and decline rate; others get 403.
- Demo scripts poll with a deadline (env, default 60 s), print final statuses, and exit non-zero if any Order is still `created` or any DLQ depth > 0.

## Technical Decisions

- **Ownership:** order-service owns Order, Order Item, status timeline, saga state, `payment_requests`, `checkouts`. payment-service owns Payment, Voucher, gateway settings. Only order-service writes Order Status; allowed transitions `created→approved→processing→shipped→delivered` and `created→canceled`, each stored in the timeline with timestamp. Unpaid Orders never expire.
- **Communication:** gateway→services gRPC; order→catalog gRPC only at Checkout (deadline from env; unavailable/slow → `UNAVAILABLE`, writes nothing). All other service-to-service traffic via Kafka: `payment.commands` (order→payment), `payment.events` (payment→order), `order.events` (order facts). One direction per topic; partition key `order_id`.
- **Envelope (JSON):** `event_id, event_type, event_version, aggregate_id, historical, occurred_at, published_at, origin_published_at, traceparent, payload`. `traceparent` also a Kafka header. `published_at` set at relay time; `origin_published_at = published_at` on first publish, copied unchanged on republish of a derived fact.
- **Event catalog:** `contracts/events/v1` (Go structs + JSON Schema via `pkg/eventbus`) is the only definition; unlisted types cannot be published. Each story adds the types it needs. Epic 2 types: `OrderCreated/Approved/Processing/Canceled` (with `from_status`, `to_status`, full snapshot), `AuthorizePayment{order_id, amount_total, voucher_code?, card_token, idempotency_key}`, `PaymentSucceeded{voucher_amount}`, `PaymentFailed{reason: CARD_DECLINED|VOUCHER_UNAVAILABLE, voucher_restored}`. Minor version bump to add a field, major to rename/remove.
- **Order snapshot on every `Order*` fact:** `order_id, customer_id, checkout_id, purchased_at, historical, status, voucher_amount` (0 until approved) plus `items[{product_id, seller_id, seller_state, category_en, price, freight}]` ("Uncategorized" when no Category).
- **Outbox (`pkg/outbox`):** state + outbox row (`bigserial seq`, current `traceparent`) in one transaction via a transaction-scoped API; no direct Kafka publish from use cases. One relay goroutine per service, poll ≤ 200 ms (env), `WHERE sent_at IS NULL ORDER BY seq` with no high-water mark, `FOR UPDATE SKIP LOCKED` only as a second-instance guard. Writes touching an Order lock its row first. franz-go idempotent producer, `acks=all`, ordered mode. Metric for unsent row count.
- **Inbox (`pkg/inbox`):** `processed_events(event_id)` recorded with the change in one transaction. Outcomes `applied / duplicate / rejected / retryable`; rejected = domain-invalid, recorded with reason, acked, counted; retryable = backoff, offset not committed, to `<topic>.dlq` after N tries (env). Undecodable or unsupported major version → rejected; unknown `event_type` on shared topic → applied no-op. New groups start at earliest. DLQ depth metric per topic plus `make dlq-redrive TOPIC=<topic>`. Consumer span started from the header for every message before any filtering. Reactors (payment-service) drop `historical=true` envelopes before decoding.
- **Saga (orchestrated by order-service):** Checkout does not start it. `OrderService.PayOrder(order_id, voucher_code?, card_token, idempotency_key)` is the only pay entry: ownership + `created` check under `SELECT … FOR UPDATE`, store `payment_requests(idempotency_key PK, order_id, request_hash)` and outbox `AuthorizePayment` in one transaction; returns "accepted". Same key/same body → same response; same key/different body → `ALREADY_EXISTS`; not `created` or another key in flight → `FAILED_PRECONDITION`; unknown card token → `INVALID_ARGUMENT`.
- **Payment processing:** payment-service has no pay RPC. Voucher redeemed by one conditional `UPDATE … SET used_by_order = $order WHERE code = $code AND used_by_order IS NULL` (0 rows → `PaymentFailed{VOUCHER_UNAVAILABLE, false}`, card not charged). On decline, restore with `SET used_by_order = NULL WHERE code = $code AND used_by_order = $order` and emit `PaymentFailed{CARD_DECLINED, voucher_restored}`. Exactly one Payment row per Order (`UNIQUE(order_id)`) for every outcome, with `voucher_code?, voucher_amount, card_amount, status, reason?`. Redemption state lives only in `vouchers.used_by_order` (unique where not null); no redemption table. Simulated card declines with probability = decline rate, or always for the "declines" test token.
- **Saga replies in order-service:** `PaymentSucceeded` → in one transaction `created→approved→processing`, both timeline entries, store `voucher_amount`, outbox `OrderApproved` then `OrderProcessing`. Any `PaymentFailed` → `canceled`, store `cancel_reason` and `voucher_restored`, outbox `OrderCanceled`. Reply for unknown or non-`created` Order → rejected + metric.
- **Checkout idempotency:** `checkouts(customer_id, idempotency_key, request_hash, checkout_id)` with PK `(customer_id, idempotency_key)`; keys scoped per Customer; same key/different hash → `ALREADY_EXISTS`. All Orders, first timeline entries and one `OrderCreated` per Order in one transaction sharing `checkout_id`.
- **Money:** int64 centavos. Snapshot `price, freight, seller_id, seller_state, category_en` from catalog into each Order Item. `order_total = Σ(price + freight)`; `voucher_amount = min(face, order_total)`; card charged `order_total − voucher_amount`.
- **Admin/lookup gRPC on payment-service:** `CreateVoucher(value)` (value ≤ 0 → `INVALID_ARGUMENT`; random unique code, regenerate on collision with bounded retries), `ListVouchers(page)`, `GetDeclineRate`, `SetDeclineRate(percent)` (integer 0–100 else `INVALID_ARGUMENT`; default from env). Customer `LookupVoucher(code)` via `GET /api/vouchers/:code`: face value + usable, `NOT_FOUND` if unknown, read-only and advisory only.
- **Order queries:** `ListMyOrders(page)` ordered `purchased_at DESC, id DESC`, `page_size` ≤ 100; `GetOrder(id)` returns `cancel_reason` and `voucher_restored`.

## UX & Interaction Patterns

- **Products → Checkout:** checkbox selection across pages and filters; sticky bottom bar "N items from M Sellers · R$ total · [Check out]". Selection in page state mirrored to `sessionStorage` (no server cart), survives login redirect, cleared after success.
- **Checkout review `/checkout`:** one Card per Seller with items and subtotal, header "This will create M Orders.", "Place Checkout". Empty selection → back to Products with toast "Select at least one Product to check out." Success toast "M Orders created — one per Seller." then My Orders. Double-click sends one idempotency key.
- **My Orders `/orders` / Order detail `/orders/:id`:** date, Money total, status badge, pagination, Refresh; detail shows items, total, timeline oldest first with timestamps.
- **Status badge:** pill filled with the status color token (`created` slate-500, `approved` blue-600, `processing` amber-600, `shipped` violet-600, `delivered` green-600, `canceled` red-600), white text with the exact status word.
- **Pay form (only on `created`):** optional Voucher input, card token Select ("Test card — approves" / "Test card — declines", no card numbers), "Pay R$ X". Voucher lookup shows "Voucher R$ V · Card R$ remainder"; unknown/used code shows the gateway message inline and keeps Pay disabled. One idempotency key per Order per page visit; button disabled after click with "Processing payment…", poll every 1 s up to 10 s. 409 → "Payment already in progress." and same polling. Outcomes: "Payment approved."; destructive toast by `cancel_reason` ("Card declined. Order canceled." / "Voucher unavailable. Order canceled.") plus "Voucher restored." only when true, pay form hidden; timeout "Still processing. Refresh in a moment." No confirm dialog for Pay.
- **Admin:** `/admin/vouchers` create form (value) + table of code, value, used/unused; `/admin/gateway` sets decline rate 0–100%.
- Literal microcopy naming backend facts; gateway messages verbatim; service-down → "Request failed: {code} — {message}" with page still usable.

## Cross-Story Dependencies

- Builds on Epic 1: gateway auth/role enforcement and JWT forwarding, catalog Product data for snapshots, web shell, Money component, and seeded `pkg/outbox`, `pkg/inbox`, `contracts/events/v1`.
- 2.1 and 2.2 underpin every publisher/consumer story (2.3, 2.7–2.9) and all later epics.
- 2.3 (Order schema, `OrderCreated`) precedes 2.4, 2.5 and 2.7; 2.6 (Voucher table, decline rate, `LookupVoucher`) precedes 2.8–2.10.
- 2.7 → 2.8 → 2.9 form the saga; 2.10 needs 2.4, 2.6–2.9; 2.11 validates the whole epic.
- `order.events` facts emitted here are consumed later by shipping-simulator (Epic 3) and analytics (Epic 5); the ≥ 4-service trace completes in Epic 3.
