# Review — Stack Versions (ARCHITECTURE-SPINE.md)

**Reviewer:** version verification lens · **Date:** 2026-10-30 · **Spine not modified.**

**Verdict:** Stack choices are sound, all projects are alive and fit their stated use. Four pins are stale (Go, franz-go, golang-migrate, Kafka minor), and the local machine's Go 1.23.2 can't build the pinned libraries without an upgrade. Fix the pins before the first story; no technology needs replacing.

Method: queried authoritative endpoints directly where possible (proxy.golang.org `@latest` and `.mod`, go.dev/dl JSON, registry.npmjs.org, GitHub releases API, postgresql.org/versions.json, downloads.apache.org), web search for context.

## Version table

| Tech | Spine pin | Verified current (2026-10-30) | Source | Verdict |
|---|---|---|---|---|
| Go | 1.26.x | **1.27.3** (1.26.10 is previous, still supported) | https://go.dev/dl/?mode=json | Outdated: move to 1.27.x |
| PostgreSQL | 18.x | 18.6 (2026-08-13), supported to 2030-11 | https://www.postgresql.org/versions.json | OK |
| Apache Kafka (KRaft) | 4.3.x | **4.4.0** (Oct 2026); 4.3.2 latest 4.3 patch (2026-10-20) | https://downloads.apache.org/kafka/ ; https://kafka.apache.org/blog/2026/06/25/apache-kafka-4.3.1-release-announcement/ | OK-ish: pin 4.3.2 explicitly, or move to 4.4.x |
| franz-go | v1.19.x | **v1.22.2** (2026-10-13), go.mod `go 1.26.0` | https://proxy.golang.org/github.com/twmb/franz-go/@latest | Outdated: move to v1.22.x |
| franz-go kotel plugin (implied by trace rule) | not pinned | v1.7.1 | https://proxy.golang.org/github.com/twmb/franz-go/plugin/kotel/@latest | Consider adding |
| golang-migrate | v4.18.x | **v4.20.1** (2026-09-09), go.mod `go 1.25.11`, depends on pgx/v5 v5.9.2 and lib/pq | https://proxy.golang.org/github.com/golang-migrate/migrate/v4/@latest ; https://github.com/golang-migrate/migrate/releases | Outdated: move to v4.20.x |
| Jaeger | v2.x | v2.22.0 (2026-10-06); OTLP native on 4317/4318; v1 EOL 2025-12-31 | https://github.com/jaegertracing/jaeger/releases ; https://www.cncf.io/blog/2024/11/12/jaeger-v2-released-opentelemetry-in-the-core/ | OK (use image `jaegertracing/jaeger`, not `all-in-one`) |
| React | 19.x | 19.3.1 | https://registry.npmjs.org/react/latest | OK |
| Vite | 8.x | 8.3.5 | https://registry.npmjs.org/vite/latest | OK |
| kafbat kafka-ui | v1.5.x | v1.5.0 (2026-04-20) | https://github.com/kafbat/kafka-ui/releases | OK (last release 6 months old, still active) |
| OpenTelemetry Go SDK | latest stable | v1.47.0 (2026-10-02), go.mod `go 1.26.0` | https://proxy.golang.org/go.opentelemetry.io/otel/@latest | OK, but pin it |
| Prometheus | latest stable | v3.15.1 (2026-10-21) | https://github.com/prometheus/prometheus/releases | OK, but pin major (3.x) |
| prometheus/client_golang | not named | v1.25.0 | https://proxy.golang.org/github.com/prometheus/client_golang/@latest | Add to table |
| Grafana | latest stable | v13.3.0 (2026-10-27) | https://github.com/grafana/grafana/releases | OK, but pin major (13.x) |
| RS256 JWT (golang-jwt/jwt/v5) | not named | v5.3.1 | https://proxy.golang.org/github.com/golang-jwt/jwt/v5/@latest | Add to table |
| bcrypt (golang.org/x/crypto/bcrypt) | not named | x/crypto v0.58.0 | https://proxy.golang.org/golang.org/x/crypto/@latest | Add to table |
| pgx (Postgres driver, implied) | not named | v5.11.0 | https://proxy.golang.org/github.com/jackc/pgx/v5/@latest | Add to table |

## Local machine

- `go` on PATH is `/usr/local/go/bin/go`, **go1.23.2 darwin/arm64**. Go 1.23 has been out of support since Go 1.25 shipped (Aug 2025).
- A Homebrew Cellar `go/1.22.0` exists but isn't linked. `/Users/tringuyenhuu/go/bin/go` is a 2021 binary that fails with "exec format error" (likely amd64 or corrupt). It sits second on PATH; delete it to avoid confusion.
- Implication: franz-go v1.22, OTel v1.47 and later deps declare `go 1.26.0` in go.mod. Go 1.23.2 can still build by auto-downloading the toolchain (`GOTOOLCHAIN=auto`, the default since 1.21) once each service's go.mod says `go 1.27`/`toolchain go1.27.x`. That depends on network access and hides the real version. Install Go 1.27.x and set `go 1.27` in every module (plus `golang:1.27` in Dockerfiles) so builds behave the same everywhere.

## Findings

### High
1. **Go pin is a generation behind.** Go 1.27 is current (1.27.3). 1.26 gets patches only until Go 1.28 ships (~Feb 2027), which may fall inside the MVP window. **Pin: Go 1.27.x.**
2. **Local toolchain 1.23.2 can't natively build the pinned libraries** (franz-go and OTel need ≥1.26). **Action:** upgrade the local Go to 1.27.x before the first story, remove the broken `~/go/bin/go`, and add a toolchain line in go.mod.

### Medium
3. **franz-go v1.19.x is stale.** Current is v1.22.2, and the README claims support for "Kafka >= 0.8.0 through v4.4+", which covers both 4.3 and 4.4. **Pin: v1.22.x.**
4. **golang-migrate v4.18.x is stale.** Current is v4.20.1. It fits the stated use: the `source/iofs` driver supports `embed.FS` migrations, and Postgres is supported via `postgres` (lib/pq) or `pgx/v5` drivers. There's no explicit PG18 test matrix, but these drivers use the standard wire protocol and DDL-only migrations work with it. **Pin: v4.20.x**, and say which DB driver to use (recommend `pgx/v5` so it matches the service driver).
5. **Kafka 4.4.0 is out** (Oct 2026). 4.3.x is still a valid choice, though: it's maintained (4.3.2 shipped 2026-10-20) and less fresh. **Pin either `4.3.2` explicitly, or `4.4.x`** if the team wants the newest line. franz-go supports both. Note that 4.3 logs a warning for the classic consumer rebalance protocol, so consider the KIP-848 consumer protocol for new consumer groups.
6. **The Postgres 18 Docker image changed its volume layout.** `VOLUME` is now `/var/lib/postgresql` (not `/var/lib/postgresql/data`), with `PGDATA=/var/lib/postgresql/18/docker` (source: https://github.com/docker-library/postgres/blob/master/18/bookworm/Dockerfile). The docker-compose volume mount must follow, or data is silently lost on recreate.

### Low
7. **"Latest stable at implementation" isn't a pin.** Pin OTel Go v1.47.x, Prometheus 3.x (v3.15), Grafana 13.x and client_golang v1.25.x so compose and go.mod stay reproducible.
8. **Libraries the spine assumes but doesn't list:** golang-jwt/jwt/v5 (RS256), golang.org/x/crypto/bcrypt (mind its 72-byte password limit), pgx/v5, and franz-go `plugin/kotel` (puts the trace context in Kafka headers, which complements the envelope `trace_id` rule). Add them to the Stack table.
9. **Jaeger image name.** Use `jaegertracing/jaeger:2.22.x`. The legacy `jaegertracing/all-in-one` image is v1, and v1 reached EOL on 2025-12-31. OTLP gRPC 4317 and HTTP 4318 are native, so the spine's no-Collector decision holds.
10. **kafbat kafka-ui v1.5.0** is the latest release, from 2026-04-20. The project is active and fine for local dev.

## Recommended Stack table

| Name | Version |
|---|---|
| Go | 1.27.x |
| PostgreSQL | 18.x (18.6) |
| Apache Kafka (KRaft) | 4.3.2 (or 4.4.x) |
| franz-go (+ kotel) | v1.22.x (kotel v1.7.x) |
| golang-migrate | v4.20.x (iofs + pgx/v5) |
| pgx | v5.11.x |
| golang-jwt | v5.3.x |
| x/crypto (bcrypt) | latest (v0.58) |
| Jaeger | v2.22.x (`jaegertracing/jaeger`) |
| React | 19.x (19.3) |
| Vite | 8.x (8.3) |
| kafbat kafka-ui | v1.5.x |
| OpenTelemetry Go | v1.47.x |
| Prometheus / client_golang | 3.x / v1.25.x |
| Grafana | 13.x |
