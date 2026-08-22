# Baseline Read Set

## Authority and terminology

- `CONTEXT.md` - instance, desired state, actual state, and plugin package terms.
- `docs/aegis/specs/2026-07-28-overlay-recovery-design.md` - startup recovery contract.
- `docs/aegis/plans/2026-07-28-overlay-recovery.md` - existing recovery implementation boundary.
- `docs/aegis/specs/2026-07-29-plugin-package-source-design.md` - package ownership and full-update boundary.

## Code owners inspected

- `internal/provisioning/service.go` - shared release resolution, Overlay ensure, and package deployment.
- `internal/provisioning/service_test.go` - release fallback and per-instance Overlay tests.
- `internal/lifecycle/service.go` - instance start, Accelerator ordering, and container creation/start.
- `internal/lifecycle/service_test.go` - lifecycle ordering and failure tests.
- `cmd/panel/main.go` - process startup recovery before reconciliation.
- `docker-compose.yml` and `deployment_test.go` - initializer ownership and security assertions.
- `internal/overlayfs/client.go` and `internal/overlayfs/server.go` - privileged mount-helper contract.

## External evidence already collected

- Background job failed reading a package manifest with `permission denied`.
- Package manifests were `root:root` and not readable by Panel UID `10001`.
- Shared-game update failure left migration state `failed` while `game/current` still pointed to a valid release.
- Existing container startup reached Accelerator before restoring its Overlay mount; `core.cfg` was present under `overlay/upper` but absent from unmounted `overlay/merged`.
