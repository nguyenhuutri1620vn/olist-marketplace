# Deferred Work

- source_spec: `_bmad-output/implementation-artifacts/spec-1-1-align-the-repo-scaffold-and-start-the-platform-with-one-command.md`
  summary: Data services need a readiness signal so `make up --wait` proves migrations finished, not only that containers are running.
  evidence: Data-service containers have no healthcheck or port in Story 1.1, so `--wait` returns once they are running; Story 1.2 adds `/readyz` on an ops port, which compose healthchecks should use.
