# Deferred Work

- source_spec: `_bmad-output/implementation-artifacts/spec-1-1-align-the-repo-scaffold-and-start-the-platform-with-one-command.md`
  summary: Data services need a readiness signal so `make up --wait` proves migrations finished, not only that containers are running.
  evidence: Data-service containers have no healthcheck or port in Story 1.1, so `--wait` returns once they are running; Story 1.2 adds `/readyz` on an ops port, which compose healthchecks should use.

- source_spec: `_bmad-output/implementation-artifacts/spec-1-2-observability-baseline-in-every-service.md`
  summary: Tie each service's grpc.health.v1 status to its readiness checks and set NOT_SERVING before GracefulStop.
  evidence: grpcx.NewServer registers a health server that always reports SERVING, so the gateway debug route shows SERVING while /readyz fails on the DB, and clients get no drain signal on shutdown; matters once the gateway routes real traffic (Story 1.6).
