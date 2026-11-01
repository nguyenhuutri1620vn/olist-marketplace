---
title: 'Story 2.1: Event envelope and transactional outbox relay'
type: 'feature'
created: '2026-11-01'
status: 'done'
baseline_commit: 'aa0ef046ee3f9364d23aa7f0029646aef345df2f'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-2-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Nothing can publish events yet: `contracts/events/v1`, `pkg/eventbus` and `pkg/outbox` are empty, so later stories have no safe way to get a state change onto Kafka without losing or reordering events (NFR1, AD-6, AD-8, AD-9).

**Approach:** Define the JSON envelope and a closed event catalog in `contracts/events/v1` (Go structs, JSON Schema generated from them); `pkg/eventbus` builds envelopes and Kafka records; `pkg/outbox` writes envelopes in the caller's DB transaction and runs one ordered relay per service on an idempotent franz-go producer. `pkg/app` starts the relay for services that opt in; order-service opts in now so the relay runs in `make up`.

## Boundaries & Constraints

**Always:** envelope fields exactly `event_id, event_type, event_version, aggregate_id, historical, occurred_at, published_at, origin_published_at, traceparent, payload`; `event_version` is `"MAJOR.MINOR"`; new `event_id` is UUIDv7 unless the caller passes one (republish/replay); partition key = `aggregate_id` (the `order_id`); `traceparent` from the caller's span stored on the row and sent as envelope field and Kafka header; `published_at` set by the relay; `origin_published_at` = `published_at` unless the writer passed one, then copied unchanged; only types listed in the catalog are accepted (others → error, nothing written); JSON Schema per type and for the envelope generated from the Go structs, with a test failing on drift; outbox writes only through a `pgx.Tx` the caller owns; relay: one goroutine, poll interval from `OUTBOX_POLL_INTERVAL` (default 200ms, rejected if > 200ms or ≤ 0), each tick takes a one-row relay lock with `FOR UPDATE SKIP LOCKED` (busy → skip tick), selects `WHERE sent_at IS NULL ORDER BY seq` (batch ≤ 100, no high-water mark), produces synchronously, marks sent in the same transaction; any produce error marks nothing; producer idempotent, `acks=all`, ordered; gauge `outbox_unsent_rows`. Test first for every behavior.

**Never:** any real event type (later stories add them); a publish API usable outside the outbox; parallel relays; topic management; inbox/consumers (Story 2.2); per-Order row locking (a writer's rule, documented, used from Story 2.3).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Write | catalog event in a committed tx | one row, `seq` increasing, traceparent of the active span | N/A |
| Unknown type | event not in the catalog | nothing written | error |
| Rollback | write then rollback | no row, nothing published | N/A |
| Relay | unsent rows 1..n | published in `seq` order, key = aggregate_id, header traceparent, `published_at` set, `origin_published_at` = `published_at` | N/A |
| Republish | origin passed by writer | `origin_published_at` = passed value | N/A |
| Late commit | seq 2 committed and sent, then seq 1 commits | seq 1 still published | N/A |
| Crash | rows committed while relay down, or produce fails | after restart / next tick all published in order | rows stay unsent |
| Second instance | relay lock held | tick skipped, nothing published twice in parallel | N/A |
| Metric | k unsent rows | `outbox_unsent_rows` = k | N/A |
| Bad interval | `OUTBOX_POLL_INTERVAL=1s` or `0` | service fails to start | config error |

</frozen-after-approval>

## Code Map

- `contracts/events/v1/doc.go` -- empty package `eventsv1`; add `envelope.go` (Envelope struct, topic constants from AD-8), `catalog.go` (`Event` interface: `EventType()`, `EventVersion()`, `Topic()`, `AggregateID()`; catalog list, empty in this story; `Lookup`), `schema/` + generator (`go generate`, invopop/jsonschema) and drift test. Tests use an internal test-only type registered through an unexported hook.
- `pkg/eventbus/doc.go` -- add `New(ctx, ev, Meta{OccurredAt, Historical, EventID, OriginPublishedAt})` → `Message` (envelope without `published_at`, traceparent via `otel.GetTextMapPropagator()` set in `pkg/tracing`); `Record(msg, publishedAt) *kgo.Record`; `NewProducer(brokers)` (franz-go, idempotent, `RequiredAcks(AllISRAcks)`).
- `pkg/outbox/doc.go` -- add `schema.sql` (embedded, exported `SchemaSQL`: `outbox(seq bigserial PK, event_id uuid UNIQUE, topic, key, envelope jsonb, traceparent, origin_published_at, created_at, sent_at)`, partial index on unsent, `outbox_relay_lock` one row), `Write(ctx, tx pgx.Tx, msg)`, `Relay` with `Run(ctx)`, gauge.
- `pkg/bootstrap/checks.go` -- reuse broker parsing from `KafkaCheck` for the producer.
- `pkg/app/app.go` -- `Service.UsesKafka` exists; add `Service.Outbox bool` (requires DB and Kafka): starts the relay goroutine, registers the gauge on `ops.NewRegistry()`, stops on shutdown.
- `services/order-service/migrations/` -- empty FS; add `000001_outbox.{up,down}.sql` equal to `outbox.SchemaSQL` (test compares); `cmd/server/main.go` sets `Outbox: true`.
- `pkg/dbmigrate` tests' `DBMIGRATE_TEST_DSN` pattern -- reuse for Postgres; add `OUTBOX_TEST_KAFKA` (brokers) for Kafka integration tests, skipped when unset.
- `.github/workflows/ci.yml` -- `integration` job: add a Kafka service and run `pkg/outbox`, `pkg/eventbus`, contracts tests; smoke: order-service `/metrics` has `outbox_unsent_rows 0`.

## Tasks & Acceptance

**Execution:**
- [x] `contracts/events/v1` -- envelope, topics, catalog, schema generator, generated `schema/envelope.json`, drift test.
- [x] `pkg/eventbus` -- envelope building, record mapping, producer constructor.
- [x] `pkg/outbox` -- schema SQL, `Write`, `Relay`, gauge, poll interval config.
- [x] `pkg/app`, `services/order-service` -- `Outbox` option, migration, wiring.
- [x] `.github/workflows/ci.yml`, `README.md` -- Kafka in integration job, smoke metric, env docs.
- [x] Tests -- unit tests for the matrix; Postgres + Kafka integration tests for crash/restart, late commit, rollback, second instance.

**Acceptance Criteria:**
- Given Postgres and Kafka, when rows are committed while the relay is stopped and the relay then starts, then a consumer reads them in `seq` order with the envelope fields and the `traceparent` header.
- Given `make up`, when order-service is healthy, then its `/metrics` shows `outbox_unsent_rows 0`.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD, as in Epic 1.
- `contracts/events/v1`: `Envelope`, topic constants and `Topics`, `Event` interface, closed `catalog` (empty), `Check`/`Lookup`/`Validate`, `RegisterForTest(t, ev)` (panics outside test binaries via `testing.Testing()`, removed at test end) so shared packages can be tested before real types exist. `internal/schemagen` (invopop/jsonschema v0.14.0) writes `schema/envelope.json` and `<Type>.v<major>.json`; `go generate` in the package; `TestSchemasAreUpToDate` fails on drift.
- `pkg/eventbus`: `New(ctx, ev, Meta)` → `Message` (catalog check, required `occurred_at` and aggregate id, UUIDv7 or a given uuid, traceparent from the global propagator), `Record(msg, publishedAt)` (key = aggregate id, traceparent header, origin defaults to publish time), `NewProducer` (acks=all, idempotent by default, auto topic creation).
- `pkg/outbox`: deviation from the Code Map — `Write(ctx, tx, ev, meta)` takes the event, not a pre-built `Message`, because `Message` fields are exported and a hand-built one would skip the catalog check. Table columns are `seq, event_id, topic, envelope jsonb, traceparent, created_at, sent_at` (key and origin live inside the envelope). Relay: one-row `outbox_relay_lock` taken with `FOR UPDATE SKIP LOCKED` (busy → skip tick), batch ≤ 100 by `seq`, `ProduceSync`, mark sent in the same transaction; gauge `outbox_unsent_rows` refreshed each tick; `PollInterval()` reads `OUTBOX_POLL_INTERVAL`.
- `pkg/app`: `Service.Outbox` (needs Migrations and UsesKafka, checked before startup); the relay outlives the servers' drain so rows committed by in-flight requests still go out. `bootstrap.KafkaBrokers` extracted from `KafkaCheck`.
- order-service: `000001_outbox` migration (test keeps it equal to `outbox.SchemaSQL`), `Outbox: true`. All modules depending on `pkg` got a `contracts` replace (`make tidy` clean).
- Mutation checks: dropping the relay lock fails the second-instance test; `ORDER BY seq DESC` fails 4 tests; not starting the relay in `pkg/app` fails the service test (the gauge starts at 0, so that test waits on `sent_at` in the DB first); acks=1 / idempotence off fails the producer test.
- Live check: `make up`; Prometheus shows `outbox_unsent_rows{instance="order-service:8090"} 0`; a raw row inserted into order's outbox was marked sent within 1 s and read back from `order.events` with key = aggregate id and `published_at` = `origin_published_at` (row deleted afterwards; the probe message stays on the local topic).

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge | A tick blocks forever while Kafka is down (franz-go retries without end), holding the transaction and lock; gauge frozen | medium | No deadline on `ProduceSync`; now a 10 s publish timeout and the gauge is counted before publishing (test with a blocking producer failed first) | patch |
| 2 | blind, edge, vgap | Comment promises rows committed during the drain are published before exit, but `Run` returned at once with no flush; untested | medium | Final bounded flush on stop; app test commits a row mid-drain and checks it is sent; deriving the relay ctx from the signal fails it | patch |
| 3 | edge, vgap, blind | Gauge starts at 0, so "0" proves nothing (CI smoke passes vacuously) | low | Gauge is NaN until the first count (test failed first); smoke now proves the relay counted | patch |
| 4 | edge | Missing `outbox_relay_lock` row looks like a held lock: silent stall | low | Now an error naming the schema (test failed first) | patch |
| 5 | edge | `NewRelay` with interval ≤ 0 panics; tiny `OUTBOX_POLL_INTERVAL` hot-loops | low | Default for ≤ 0; env floor 10ms (tests failed first) | patch |
| 6 | edge | Non-canonical given `event_id` (`{…}`, upper case) differs from the stored uuid | low | Canonicalised (test failed first) | patch |
| 7 | blind, edge | `RegisterForTest` overwrites/clears an outer registration; may shadow a real catalog type | low | Restores the previous entry; refuses real types (test failed first) | patch |
| 8 | blind, edge | Duplicate catalog types, non-struct types accepted | low | Validate requires a struct; schemagen rejects duplicates; catalog uniqueness test (tests failed first) | patch |
| 9 | blind | Envelope schema looser than code (`event_id` uuid, empty type/aggregate) | low | `format=uuid`, `minLength=1` (test failed first) | patch |
| 10 | blind | At-least-once resend not documented; producer comment overpromises | low | Package doc and `NewProducer` doc state it | patch |
| 11 | vgap | CI path filters of three services miss `contracts/**` | low | They now depend on contracts through `pkg` | patch |
| 12 | blind | `Relay.Run` returns a meaningless error | low | Now returns nothing | patch |
| 13 | blind, edge | Sent rows never deleted | medium | Outside this story's intent | defer |
| 14 | vgap, blind | Smoke cannot prove publishing in the stack | low | No writer until Story 2.3; integration tests cover the path | defer |
| 15 | blind | Catalog keyed by type only; two majors cannot coexist | low | No type exists yet; needed with the first breaking change | defer |
| 16 | blind | Per-Order lock rule not enforced | low | Frozen Never: a writer's rule from Story 2.3 | reject |
| 17 | blind | Topics auto-created with broker defaults | low | Frozen Never: topic management | reject |
| 18 | blind | `eventsv1` imports `testing` | low | Guarded by `testing.Testing()`; test flags register only via `testing.Init`; a separate package cannot reach the registry | reject |
| 19 | blind | `traceparent` column duplicates the envelope | low | Story requires the row to carry it; harmless | reject |
| 20 | blind | Schema copied by hand per service | low | `SchemaSQL` doc says so and each copy has an equality test | reject |
| 21 | edge | Nil event panics in `New` | low | Programmer error that fails loudly | reject |
| 22 | edge | Reused `event_id` written twice gives an opaque unique error | low | Loud failure; idempotent handling is Story 2.2's inbox | reject |
| 23 | edge | Late-commit and second-instance tests use a fake producer; no real-Kafka crash mid-produce | low | Relay logic is producer-independent; failing-producer test covers resend; real-Kafka restart test exists | reject |

## Design Notes

- The relay lock row (instead of `SKIP LOCKED` on outbox rows) is what keeps a second instance from publishing the rows after the first instance's batch out of order; `SKIP LOCKED` on that row only makes the second instance skip its tick.
- A failed produce rolls back the mark; already-delivered records of that batch are sent again next tick (at-least-once, consumers are idempotent from Story 2.2), still in `seq` order.

## Verification

**Commands:**
- `make build && make test` with `DBMIGRATE_TEST_DSN` and `OUTBOX_TEST_KAFKA` set -- expected: exit 0, integration tests run (not skipped).
- `cd contracts && go generate ./... && git diff --exit-code` -- expected: no drift.
- `make up`, then `curl 127.0.0.1:…` order-service metrics inside the network -- expected: `outbox_unsent_rows 0`.
