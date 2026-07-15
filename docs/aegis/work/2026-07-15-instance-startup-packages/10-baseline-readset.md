# Baseline Read Set

## Authority

- `docs/aegis/specs/2026-07-15-instance-startup-package-design.md`: approved behavior and compatibility boundary.
- `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`: Host networking, persistent data, fixed Supervisor and safe update invariants.
- `README.md`: deployment, first-install and verification contract.

## Runtime Owners

- `internal/domain/models.go`: instance configuration and state.
- `internal/store/migrations.go`, `internal/store/store.go`: SQLite schema and transactional instance persistence.
- `internal/httpapi/server.go`: strict create/update/action contracts and persistent Job admission.
- `internal/jobs/manager.go`: per-instance mutation serialization.
- `internal/lifecycle/service.go`: port/space checks, container creation, rebuild and health state.
- `internal/docker/client.go`, `internal/docker/lifecycle.go`: restricted maintenance containers and game container environment.
- `internal/content/packages.go`, `internal/updates/pipeline.go`, `internal/updates/coordinator.go`: package lookup, deployment transaction and rollback.
- `runtime/supervisor.py`: final SRCDS argv and PTY process owner.
- `cmd/panel/main.go`: production dependency wiring.

## User Journey Owners

- `web/src/api/client.ts`: API normalization.
- `web/src/app/App.tsx`: instance cards, creation, Job polling and content repository.
- `web/src/styles/app.css`: responsive modal/card layout.
- `web/src/app/App.test.tsx`: component/API contract tests.
- `web/e2e/control-panel.spec.ts`: real-browser main journey.
- `cmd/e2e-fixture/main.go`: deterministic external-boundary substitutes.

## Baseline Evidence

- `go test -count=1 ./...`: PASS on 2026-07-15.
- `cd web && npm test -- --run`: PASS, 17 tests.
- `cd web && npm run build`: PASS.

## Compatibility Boundary

- Keep Host network, labels, UID/GID, persistent mounts, PTY-only console and per-instance Job serialization.
- Preserve existing `package_version` data as the applied package identity.
- Keep empty extra arguments behavior unchanged.
- Keep the content repository's explicit hot/full update operations.
- Retire runtime-owned SteamCMD bootstrap only after Panel-owned maintenance installation is verified.
