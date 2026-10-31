---
title: 'Story 1.2: Observability baseline in every service'
type: 'feature'
created: '2026-10-31'
status: 'done'
baseline_commit: '96c62a348319383e97b2f3fd008f4c9650766112'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Services from Story 1.1 log with `log.Printf`, emit no traces or metrics, have no health/readiness endpoints or gRPC servers, and compose cannot tell when a data service is actually ready.

**Approach:** Shared `pkg` helpers give every service the same slog JSON logger (trace-id aware), OTel tracing to Jaeger over OTLP, one ops port with `/healthz`, `/readyz` and `/metrics`, and gRPC server/client constructors with tracing, metrics and `grpc.health.v1`. Prometheus scrapes all services, Grafana gets both data sources, and a gateway debug route proves one trace spans gateway → service.

## Boundaries & Constraints

**Always:** same helpers in all seven services; ops port is internal (not published); readiness checks the DB for data services and Kafka for order, payment, shipping-simulator and analytics (the services that use Kafka per the communication matrix); OTel Go SDK v1.47, otelgrpc/otelhttp v0.72, client_golang v1.25, franz-go v1.22. Test first for every behavior.

**Never:** publish ops or gRPC ports; add business RPCs, Kafka producers/consumers, or an OTel Collector; bind-mount config into containers (Docker here cannot mount under `~/Documents`, so bake config into images as in 1.1).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Log inside span | `slog.InfoContext(ctx, …)` with active span | JSON line has `trace_id`, `span_id` | N/A |
| Log outside span | no span in ctx | JSON line without trace fields | N/A |
| Ready | all checks pass | `/readyz` 200 | N/A |
| Not ready | DB or Kafka check fails or exceeds 2s | `/readyz` 503, body names failing check | logged at warn |
| Alive | process up, DB down | `/healthz` 200 | N/A |
| Container healthcheck | `/server healthcheck` | exit 0 iff own `/readyz` is 200 | exit 1 |
| Debug health | `GET /debug/grpc-health/{service}` | 200 with health status; one trace with gateway client span → service server span | unknown service 404; unreachable 502 |

</frozen-after-approval>

## Code Map

- `services/*/cmd/server/main.go` -- Story 1.1 wiring: `bootstrap.Migrate`, `log.Printf`, wait on SIGTERM; gateway serves empty mux on `:8080`.
- `pkg/bootstrap`, `pkg/config`, `pkg/dbmigrate` -- reuse; `config.Require` for env.
- `pkg/logger/doc.go`, `pkg/tracing/doc.go` -- doc-only seeds to fill.
- `proto/gen/go/*/v1` -- empty service stubs; register nothing, only `grpc.health.v1`.
- `deployments/docker-compose.yml` -- `x-service` anchor; services have no healthcheck yet (deferred from 1.1); Kafka at `kafka:9092`, Jaeger at `jaeger`.
- `deployments/prometheus/` -- image with baked `prometheus.yml` (self-scrape only).
- `tests/scaffold/scaffold_test.go` -- extend for scrape targets, data sources, healthchecks.

## Tasks & Acceptance

**Execution:**
- [x] `pkg/logger` -- `New(service) *slog.Logger`: JSON to stdout, `service` attr, handler adds `trace_id`/`span_id` from ctx; sets default.
- [x] `pkg/tracing` -- `Init(ctx, service) (shutdown, error)`: OTLP/HTTP exporter (env `OTEL_EXPORTER_OTLP_ENDPOINT`), batch provider, `service.name` resource, W3C propagator.
- [x] `pkg/ops` -- ops HTTP server: `/healthz`, `/readyz` (named checks, 2s timeout, 503 + names), `/metrics` (own registry incl. Go/process collectors); `HTTPMetrics` middleware (count + latency by method, route, code); `Probe` for the `healthcheck` subcommand.
- [x] `pkg/grpcx` -- `NewServer` (otelgrpc stats handler, metrics interceptor, health SERVING) and `Dial` (otelgrpc client handler).
- [x] `pkg/bootstrap` -- `OpenDB` (pgxpool) and `KafkaCheck` (franz-go ping) readiness checks.
- [x] `services/*/cmd/server/main.go` -- wire logger, tracing, ops (`:8090`), migrations, gRPC `:9090` for identity/catalog/order/payment/analytics; graceful shutdown.
- [x] `services/api-gateway` -- otelhttp + metrics on API mux; `GET /debug/grpc-health/{service}` using `grpcx.Dial` to `<service>:9090`.
- [x] `deployments/docker-compose.yml` -- env (`OTEL_*`, `KAFKA_BROKERS`), `healthcheck: ["CMD","/server","healthcheck"]` on all seven services.
- [x] `deployments/prometheus/prometheus.yml`, `deployments/grafana/{Dockerfile,datasources.yml}` -- scrape `<service>:8090`; Prometheus + Jaeger data sources.
- [x] `tests/scaffold/scaffold_test.go` -- scrape targets cover every service; data sources present; every service has a healthcheck.

**Acceptance Criteria:**
- Given `make up`, when it returns, then all seven services report healthy via `/readyz`, Prometheus targets are all up, and Grafana lists Prometheus and Jaeger data sources.
- Given the stack, when `curl 127.0.0.1:8080/debug/grpc-health/catalog-service` runs, then Jaeger shows one trace containing an api-gateway span and a catalog-service child span.
- Given any service log line written with a span context, then it is JSON with the trace id.

## Implementation Notes

- Implemented directly in the orchestrating session with TDD (red run before every behavior); same approach as Story 1.1.
- Added `pkg/app` (`Main`/`Run`/`Healthcheck`) so every `main.go` is one `app.Main(app.Service{...})` call; the wiring is tested once there (including a Postgres-backed run that migrates, serves gRPC health and reaches ready).
- Ops metrics use one Prometheus registry per process; HTTP route label is the ServeMux pattern (`unmatched` otherwise) to bound cardinality.
- Gateway error bodies follow the architecture convention `{"error":{"code","message"}}`.
- Compose: `x-env` / `x-healthcheck` / `x-kafka-service` anchors; Kafka-using services also wait for Kafka healthy. Grafana config is baked into an image (`deployments/grafana/`), like Postgres and Prometheus.
- Mutation check: removing the client stats handler from `grpcx.Dial` fails the parent/child trace test.

## Spec Change Log

## Review Triage Log

| # | Layer | Finding | Verdict | Evidence | Route |
|---|-------|---------|---------|----------|-------|
| 1 | blind, edge, vgap | `Run` leaks already-started servers when a later listen fails | low | Early return left goroutines serving; now binds every port before serving any | patch |
| 2 | blind, edge | gRPC health always SERVING; no NOT_SERVING on shutdown | medium | `health.NewServer()` never updated; tie-in needs readiness polling | defer |
| 3 | blind, edge | Gateway gRPC conns never closed | low | `API` had no cleanup hook; now returns a closer, also on partial-dial error | patch |
| 4 | blind | Gateway healthy before its upstreams | medium | No checks and no `depends_on`; compose now waits for the five gRPC services healthy | patch |
| 5 | blind, edge | Span names use raw path; disagree with metric route | medium | `WithSpanNameFormatter` used `r.URL.Path`, and otelhttp re-applies the formatter after routing, so renaming elsewhere was overwritten (seen live in Jaeger); formatter now prefers `r.Pattern`; Run-level test uses a wildcard route (failed first) | patch |
| 6 | blind, edge, vgap | Streaming RPCs not metered | low | Only unary interceptor; stream interceptor added (Watch test failed first) | patch |
| 7 | blind | Readiness checks sequential (2 failing ≈ 4s vs 5s probe) | medium | Three 1s checks took 3s; now concurrent (test failed first) | patch |
| 8 | blind, edge | `WithGroup` nests trace fields; test name claims groups | low | No caller uses groups; fix needs group tracking; misleading test renamed | patch (rename) / reject (fix) |
| 9 | blind | Migration-failure test passes for any error | low | Now asserts a connect error | patch |
| 10 | blind, vgap | No Run-level test of HTTP tracing/metrics wiring | medium | Removing otelhttp passed all tests; new test with fake OTLP receiver asserts server→client parentage and ops metrics (mutation now fails) | patch |
| 11 | edge | Debug route returns 200 for NOT_SERVING | low | Status only in body; now 503 (test failed first) | patch |
| 12 | blind, edge | `KAFKA_BROKERS` entries not trimmed | low | `" , "` produced empty seeds; trimmed, empty list errors (test failed first) | patch |
| 13 | edge | grpcx trace test can race server span end | low | Server `End` can follow client return; test now waits up to 2s | patch |
| 14 | edge | Global propagator not restored in test | low | Cleanup restored only the provider; now restores both | patch |
| 15 | vgap | `pkg/app` DB test skipped in CI | medium | `integration` job ran only dbmigrate/bootstrap; `./app/...` added | patch |
| 16 | vgap | Compose OTEL endpoint per service untested | medium | Removing `<<: *env` passed; scaffold test added (mutation fails) | patch |
| 17 | vgap | "Prometheus targets up" only checked statically | medium | Smoke job now polls `/api/v1/targets` and curls the debug route | patch |
| 18 | blind | Upstream addresses hard-coded | low | Compose-fixed names; no override needed yet | reject |
| 19 | blind | Debug route on API port returns raw gRPC errors | low | Port bound to 127.0.0.1; local demo only | reject |
| 20 | blind | Diff omits spec, sprint status, go.sum | false | Deliberately excluded from the review diff; files exist | reject |
| 21 | blind | No Jaeger `depends_on`, no `start_period` | low | Batch exporter retries; 30×2s retries cover migrations | reject |
| 22 | edge | SIGTERM during startup exits 1 | low | Cosmetic exit code during a stop | reject |
| 23 | edge | Arbitrary HTTP methods inflate label cardinality | low | API bound to localhost | reject |
| 24 | edge | `freeAddr` port reuse race | low | Rare in local/CI runs | reject |
| 25 | blind | No tests for `UsesKafka` wiring / mid-run server failure / explicit OPS host | low | Kafka check covered in bootstrap tests and live readiness; others rare | reject |

## Design Notes

- Ops port `:8090` separate from API/gRPC ports so probes and scrapes never hit business middleware; ops handlers are not traced.
- Distroless has no curl, so the binary probes itself (`/server healthcheck`), closing the 1.1 deferred readiness item.
- shipping-simulator has no gRPC contract (Kafka only), so it gets ops + readiness but no gRPC server.

## Verification

**Commands:**
- `make build && make test && make proto` -- expected: exit 0, DB tests run with `DBMIGRATE_TEST_DSN`.
- `make up` -- expected: exit 0 with all services healthy.
- `curl -s 127.0.0.1:9090/api/v1/targets` -- expected: every service target `up`.
- `curl -s -u admin:admin 127.0.0.1:3000/api/datasources` -- expected: Prometheus and Jaeger.
- `curl 127.0.0.1:8080/debug/grpc-health/catalog-service` then Jaeger API `127.0.0.1:16686/api/traces?service=api-gateway` -- expected: trace with spans from both services.
