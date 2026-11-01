---
title: 'Story 2.2: Idempotent inbox consumers'
type: 'feature'
created: '2026-11-01'
status: 'done'
baseline_commit: 'abf2af276ef5932855def8f36e9185d682054fab'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-2-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Story 2.1 publishes at least once, but nothing can consume safely: redelivery would double-apply, and a bad or failing message would stall its partition (NFR2, AD-7).

**Approach:** `pkg/inbox` runs a Kafka consumer group whose typed handlers apply each event inside one DB transaction together with a `processed_events` row, classifies every message as `applied`, `duplicate`, `rejected` or `retryable`, retries with backoff and then parks the message on `<topic>.dlq`. A `tools/dlq-redrive` command (`make dlq-redrive TOPIC=…`) republishes parked messages, a per-topic DLQ depth metric shows what is parked, and `make dlq-check` (the same tool, reading Kafka directly) fails when any `*.dlq` topic has depth above 0. `pkg/app` runs consumers for services that declare them.

## Boundaries & Constraints

**Always:** `processed_events(consumer, event_id)` primary key, written in the same transaction as the handler's change; a known key → `duplicate`, nothing applied; outcomes exactly `applied | duplicate | rejected | retryable`; handler error marked `inbox.Reject(reason)` → `rejected` (handler changes rolled back, the row recorded with outcome and reason, acked); any other handler or DB error → `retryable`: rolled back, retried in place with backoff (100ms doubling, max 5s), offset not committed; after `INBOX_MAX_ATTEMPTS` (env, default 5) attempts the record goes to `<topic>.dlq` (same key, value, headers, plus `dlq-reason` and `dlq-attempts`) and only then is the offset committed; undecodable envelope, unknown major `event_version` or undecodable payload → `rejected` with a reason, never retried; `event_type` with no handler → `applied` no-op; consumers marked as reactors drop `historical=true` before decoding (`applied` no-op); one record at a time per consumer, offsets committed manually after each outcome; new groups start at the earliest offset; a span is started from the `traceparent` header for every record before any filtering, and the handler's ctx carries it; counter `inbox_messages_total{consumer,topic,outcome}`; gauge `inbox_dlq_depth{topic}` = DLQ end offset − offset committed by the `dlq-redrive` group, refreshed every ≤ 15 s; redrive copies each DLQ record (minus `dlq-*` headers) back to its original topic and commits the `dlq-redrive` group; the check lists every `*.dlq` topic and uses the same depth definition. Test first for every behavior.

**Never:** real consumers or handlers for business events (Stories 2.7–2.9); parallel processing within a consumer; deleting DLQ messages; demo scripts themselves (later stories call `make dlq-check`).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| First delivery | known type, handler ok | change + `processed_events` row committed together; `applied` | N/A |
| Redelivery | same `event_id` again | handler not called; `duplicate` | N/A |
| Domain-invalid | handler returns `Reject("x")` | change rolled back, row with outcome `rejected`, reason `x`; acked | N/A |
| Transient | handler fails twice then succeeds | applied once on attempt 3; offset not committed before | N/A |
| Persistent | always retryable | after N attempts on `<topic>.dlq` with reason; partition moves on | DLQ produce fails → keep retrying |
| Bad message | not JSON / major 2 for a v1 handler / bad payload | `rejected` with reason, no retry | N/A |
| Unknown type | no handler for `event_type` | `applied` no-op | N/A |
| Historical | reactor, `historical=true` | dropped before decoding, span still recorded | N/A |
| Trace | header `traceparent` | handler span is its child; logs carry trace id | N/A |
| Redrive | 2 messages parked | both back on the topic, depth 0 | N/A |
| DLQ check | any depth > 0 | `make dlq-check` exits 1 | N/A |

</frozen-after-approval>

## Code Map

- `pkg/inbox/doc.go` -- empty; add `schema.sql` (`SchemaSQL`, `processed_events(consumer, event_id, event_type, outcome, reason, processed_at)`), `inbox.go` (`Consumer` with `Handle[T eventsv1.Event](c, fn func(ctx, pgx.Tx, eventsv1.Envelope, T) error)`, `Reject`, outcome, `Run(ctx)`), `dlq.go` (DLQ produce, depth gauge via `kadm`), `MaxAttempts()` env.
- `pkg/outbox/outbox.go` -- pattern to mirror: embedded `schema.sql`, gauge NaN until first value, `RegisterForTest` types in tests, `newPool`/isolated schema test helpers, `OUTBOX_TEST_KAFKA`.
- `pkg/eventbus/eventbus.go` -- `TraceparentHeader`, `NewProducer` (reuse for DLQ and redrive).
- `contracts/events/v1/catalog.go` -- `Check`, `Event.EventVersion()` major for the version check.
- `pkg/app/app.go` -- add `Service.Consumers []*inbox.Consumer` (needs Migrations and UsesKafka): started after Init, metrics registered, stopped after the servers drain like the relay.
- `pkg/logger/logger.go` -- trace handler already adds the trace id from ctx.
- `tools/` -- new module `tools/dlq-redrive` (`main.go`, Dockerfile, go.work entry); compose service under profile `tools`; `Makefile` `MODULES`, `dlq-redrive` (requires `TOPIC`), `dlq-check` (`dlq-redrive -check`); `tests/scaffold` `tools` list.
- `.github/workflows/ci.yml` -- run `pkg/inbox` tests in the integration job; build the redrive image.

## Tasks & Acceptance

**Execution:**
- [x] `pkg/inbox` -- schema, consumer, outcomes, retry/DLQ, tracing, metrics.
- [x] `pkg/app` -- `Consumers` option and lifecycle.
- [x] `tools/dlq-redrive`, compose, `Makefile`, `tests/scaffold`, `README.md` -- redrive and check commands.
- [x] `.github/workflows/ci.yml` -- inbox tests with Postgres and Kafka; redrive image build.
- [x] Tests -- matrix rows with real Postgres and Kafka (`DBMIGRATE_TEST_DSN`, `OUTBOX_TEST_KAFKA`), redrive end to end, app lifecycle.

**Acceptance Criteria:**
- Given Postgres and Kafka, when one event is delivered twice and a handler fails transiently then succeeds, then the effect is applied once and the failing message is retried, not skipped.
- Given `make up` and a record parked on `order.events.dlq`, when `make dlq-redrive TOPIC=order.events` runs, then the record is back on `order.events` and `make dlq-check` passes.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD, as before.
- `pkg/inbox`: `schema.sql`/`SchemaSQL` (`processed_events`, PK `(consumer, event_id)`, outcome `applied|rejected` with reason); `NewConsumer(group, topics...)`, `DropHistorical()`, generic `Handle[T]` (panics for a type outside the catalog), `Reject(reason)`, `Outcome`; `process` = span from the header first, then up to `maxAttempts` tries with `backoff` (100ms doubling, cap 5s), then `park` on `<topic>.dlq` (copies key/value/headers, adds `dlq-reason`, `dlq-attempts`; retries the DLQ produce until it lands or ctx ends). `attempt`: envelope decode → reactor historical drop → unknown type no-op → major check → one transaction (inbox row `ON CONFLICT DO NOTHING`, 0 rows → duplicate; payload decode; handler); a rejection is recorded in its own transaction (a failed record write makes it retryable). Parked messages get no inbox row, so a redrive applies them.
- `Consumer.Run(ctx, Deps)`: franz-go group, earliest reset, manual commits per record (bounded detached ctx), `BlockRebalanceOnPoll`; stopping mid-retry commits nothing.
- DLQ: depth = end offset − max(`dlq-redrive` group commit, start offset), per partition; `DLQDepth`, `NonEmptyDLQs`, `WatchDLQDepth` (gauge), `Redrive` (reads the pending count first, republishes without `dlq-*` headers, commits per batch). Metrics: `inbox_messages_total{consumer,topic,outcome}` (retryable counts every failed attempt), `inbox_dlq_parked_total{topic}`, `inbox_dlq_depth{topic}`.
- The tracer is looked up per message: a package-level `otel.Tracer` is bound to the first global provider and ignored later ones (found by a failing test).
- `pkg/app`: `Service.Consumers` (needs Migrations and UsesKafka; `INBOX_MAX_ATTEMPTS` validated before startup); consumers, the relay and a 15 s DLQ depth watcher share one background context stopped after the servers drain.
- `tools/dlq-redrive` (new module in go.work and `MODULES`): `-topic <t>` redrives, `-check` exits 1 listing non-empty DLQs; compose service under the `tools` profile; `make dlq-redrive TOPIC=…`, `make dlq-check`; scaffold `tools` list and a Kafka/Makefile rule; CI runs inbox and tool tests with Kafka and builds the image.
- Mutation checks: disabling the duplicate check fails the redelivery test and the Kafka test (`[1 1 2 3]`); never committing fails the Kafka test.
- Live check: `make up`; a record put on `order.events.dlq` made `make dlq-check` fail naming it; `make dlq-redrive TOPIC=order.events` → "redrove 1 records"; the record was on `order.events`; `make dlq-check` then "every DLQ is empty". `make dlq-redrive` without TOPIC prints the usage. The probe messages stay on the local topics (unknown types: no-ops for consumers).

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge, vgap | A non-uuid `event_id` fails the uuid insert, so it is retried, parked and re-parked after every redrive | medium | Probe returned retryable `invalid input syntax for type uuid`; now rejected before any DB write (test failed first) | patch |
| 2 | blind, edge | Reason `undecodable envelope: <nil>` for a valid JSON envelope missing fields | low | Missing `event_type` / bad id now named (test failed first) | patch |
| 3 | edge | A handler panic crashes the service and crash-loops on redelivery | medium | Test panicked the test binary; panics are now retryable and end on the DLQ | patch |
| 4 | vgap | Stopping mid-retry (interrupted backoff, DLQ down) never tested | medium | Harness test with an interrupted sleep asserts no final outcome | patch |
| 5 | vgap | `inbox_dlq_parked_total` and a non-zero `inbox_dlq_depth` never asserted | low | Counter asserted after a failed and a good DLQ produce; watcher gauge reaches 1 then 0 in the Kafka test (zeroing it fails the test) | patch |
| 6 | vgap | Redrive's header stripping untested | low | Unit test keeps `traceparent` and other headers, drops `dlq-*` | patch |
| 7 | blind | Parked records do not say which consumer failed; re-parking stacks headers | low | `dlq-consumer` header added; old `dlq-*` headers replaced (test failed first) | patch |
| 8 | blind, edge | Unbounded `INBOX_MAX_ATTEMPTS`; a 100-record poll can block a rebalance past its timeout | low | Max 20 (test failed first); one record per poll | patch |
| 9 | blind, edge | One failing DLQ partition freezes every topic's depth gauge | low | Gauges set for topics that answered | patch |
| 10 | edge | Redrive fails with "0 of N arrived" when joining the group takes over 10 s | medium | First empty poll returned an error; now keeps polling until the caller's deadline | patch |
| 11 | edge | Consumers with no group, no topics or a shared group start silently | low | Rejected at startup (test failed first) | patch |
| 12 | edge | Redrive tool test fails on a broker with other non-empty DLQs | low | Now asserts only its own topic is gone | patch |
| 13 | blind | Redrive reorders a parked event after newer ones of its Order; README omits the parked metric; Makefile variable named `IMPORTER` | low | Documented in `Redrive` doc and README; variable renamed `TOOLS` | patch |
| 14 | blind | `processed_events` grows forever | medium | Outside this story's intent | defer |
| 15 | edge | Redrive progress lost if the `dlq-redrive` group's offsets expire | low | Needs durable progress or DLQ truncation | defer |
| 16 | blind, edge | A consumer whose `Run` fails leaves the service ready | low | `Run` fails only on client construction, with the same brokers `KafkaCheck` already built a client from at startup | reject |
| 17 | blind | Undecodable envelopes are acked, not parked | low | Frozen intent: undecodable → rejected, never retried | reject |
| 18 | blind | Unknown-type and historical no-ops counted as `applied` | low | Frozen intent: "acked as an applied no-op" | reject |
| 19 | blind | `park` retries the DLQ forever; DLQ topics not provisioned | low | Never skipping a message is the intent; topic management is out of scope (Story 2.1) | reject |
| 20 | blind | Shutdown counts a cancelled attempt as retryable | low | Metric noise only at shutdown | reject |
| 21 | blind | Per-record commit round trip | low | Throughput is not a goal; per-record commit keeps the at-most-one-redelivery window small | reject |
| 22 | edge | Fetch errors loop without backoff | low | franz-go backs off retriable fetch errors itself; non-retriable ones are configuration errors | reject |
| 23 | blind | Redrive edge tests (second run, partitions, timeouts) | low | Second run returning 0 is already asserted; per-partition depth is summed by `depths` | reject |

## Design Notes

- Rejected messages are recorded in `processed_events` so a redelivered reject stays a `duplicate`; an undecodable envelope has no `event_id` and is only logged and counted.
- DLQ depth uses the redrive group's committed offset because DLQ topics are never truncated: after a redrive the depth is 0 while the history stays inspectable.

## Verification

**Commands:**
- `make build && make test` with `DBMIGRATE_TEST_DSN` and `OUTBOX_TEST_KAFKA` -- expected: exit 0, inbox integration tests run.
- `make up`, park a record on `order.events.dlq`, `make dlq-redrive TOPIC=order.events`, `make dlq-check` -- expected: record back on `order.events`, `make dlq-check` exits 1 before the redrive and 0 after.
