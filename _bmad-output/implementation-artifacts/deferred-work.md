# Deferred Work

- source_spec: `_bmad-output/implementation-artifacts/spec-1-1-align-the-repo-scaffold-and-start-the-platform-with-one-command.md`
  summary: Data services need a readiness signal so `make up --wait` proves migrations finished, not only that containers are running.
  evidence: Data-service containers have no healthcheck or port in Story 1.1, so `--wait` returns once they are running; Story 1.2 adds `/readyz` on an ops port, which compose healthchecks should use.

- source_spec: `_bmad-output/implementation-artifacts/spec-1-2-observability-baseline-in-every-service.md`
  summary: Tie each service's grpc.health.v1 status to its readiness checks and set NOT_SERVING before GracefulStop.
  evidence: grpcx.NewServer registers a health server that always reports SERVING, so the gateway debug route shows SERVING while /readyz fails on the DB, and clients get no drain signal on shutdown; matters once the gateway routes real traffic (Story 1.6).
- source_spec: `_bmad-output/implementation-artifacts/spec-1-3-import-catalog-reference-data.md`
  summary: Protect catalog's CatalogAdminService (auth interceptor requiring an Admin JWT, or a separate internal listener) once JWT verification exists.
  evidence: The admin RPCs are registered on the same gRPC server as future public RPCs with no access control; only the unpublished port limits callers today (Story 1.6).
- source_spec: `_bmad-output/implementation-artifacts/spec-1-3-import-catalog-reference-data.md`
  summary: Tie the Makefile OLIST_FILES list to the files olist.Load reads (e.g. a test inside tools/olist-importer that parses the Makefile).
  evidence: tests/scaffold pins OLIST_FILES to its own literal list, so adding a CSV to olist.Load would pass CI and fail make import at runtime.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-4-register-and-log-in.md`
  summary: Throttle Login (per email and per client) against online password guessing, most likely in the gateway.
  evidence: Login has no rate limit or lockout; bcrypt cost 10 only slows each guess. Not in Story 1.4's intent.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-4-register-and-log-in.md`
  summary: Support signing-key rotation (publish every stored key in the JWKS, sign with the newest) and keep the private key out of plain database storage.
  evidence: `SigningKey` loads only the newest key and stores PKCS#8 DER unencrypted; fine for one local key, not for rotation or production.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-5-seed-demo-accounts.md`
  summary: Web Login page hint showing two demo Accounts per role plus the Admin (Story 1.5 AC), built with the Login page.
  evidence: The web app and its Login page arrive in Story 1.7; the README already lists all 41 demo logins.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-5-seed-demo-accounts.md`
  summary: Protect identity's IdentityAdminService like catalog's admin service (Admin JWT or a separate internal listener) and keep the gateway from routing it.
  evidence: ImportAccounts can reset every demo password and create Admins for anyone who reaches identity's gRPC port; only the unpublished port limits callers today (Story 1.6).
- source_spec: `_bmad-output/implementation-artifacts/spec-1-5-seed-demo-accounts.md`
  summary: Test the importer's run() wiring end to end (env keys, both admin calls) against a fixture dataset, in a test or a CI smoke step.
  evidence: Only the units and a manual live run cover it; dropping the ImportAccounts call or misspelling DEMO_PASSWORD would pass CI.


- source_spec: `_bmad-output/implementation-artifacts/spec-2-1-event-envelope-and-transactional-outbox-relay.md`
  summary: Delete or archive sent outbox rows (retention), so the outbox table stops growing forever.
  evidence: Nothing removes rows with sent_at set; every publishing service's outbox grows with full jsonb envelopes (Story 2.1 review).
- source_spec: `_bmad-output/implementation-artifacts/spec-2-1-event-envelope-and-transactional-outbox-relay.md`
  summary: Strengthen the CI smoke outbox check to prove a row reaches Kafka once order-service writes events (Story 2.3).
  evidence: The smoke step now proves the relay counted (gauge NaN until the first count), not that it published; no compose writer exists yet.
- source_spec: `_bmad-output/implementation-artifacts/spec-2-1-event-envelope-and-transactional-outbox-relay.md`
  summary: Let the catalog hold two major versions of one event type (key by type and major) when the first breaking change lands.
  evidence: Lookup/Check match on event_type alone, while AD-9 asks for a new major version on rename/remove; schemagen already names files by major.
