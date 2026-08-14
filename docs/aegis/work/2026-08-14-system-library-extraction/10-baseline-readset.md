# Baseline Read Set

- `CONTEXT.md`: project terminology and managed game-instance boundary.
- `docs/aegis/specs/2026-08-10-accelerator-management-analysis-design.md`: crash artifact contracts, security invariants, and Stackwalk flow.
- `internal/crashreports/manager.go`: report persistence and analysis enqueue timing.
- `internal/crashreports/artifacts.go`: content-addressed binary and generated-symbol storage.
- `internal/crashreports/signature.go`: module Debug ID, platform, and architecture fields.
- `internal/crashanalysis/worker.go`: Stackwalk preparation boundary.
- `internal/crashsymbols/indexer.go`: Breakpad `dump_syms` generation and MODULE parsing.
- `internal/docker/client.go`: managed container labels and Docker API transport.
- `internal/docker/lifecycle.go`: game container role and instance label contract.
- `Dockerfile` and `docker-compose.yml`: Panel/Docker isolation and socket-proxy boundary.

## Facts, assumptions, unknowns

- Fact: game containers are labeled with the managed instance ID and `role=game`.
- Fact: Docker exposes `GET /containers/{id}/archive?path=...` for container filesystem reads.
- Assumption: the Accelerator v2 crash signature includes the module Debug ID for the missing system library.
- Unknown: whether every target image exposes libc as a regular file or through one or more symlinks; the archive reader will safely resolve bounded relative symlinks.
