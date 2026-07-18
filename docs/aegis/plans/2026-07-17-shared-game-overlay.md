# Shared Game Body And Instance Overlay Implementation Plan

**Design:** `docs/aegis/specs/2026-07-17-shared-game-overlay-design.md`

**Goal:** Replace per-instance installations with one versioned shared installation, per-instance OverlayFS layers, and global game updates with aggregate player gating.

**Compatibility boundary:** Preserve package/private source data and precedence, Jobs/logs, shared VPK, Steam credentials, and desired-state restoration. Do not alter unrelated Docker changes already in the worktree.

## Task 1: Shared Paths And State

**Files:** `internal/config/config.go`, `internal/domain/models.go`, `internal/store/migrations.go`, `internal/store/store.go`, and adjacent tests.

Define canonical helpers for `game/releases`, `game/staging`, `game/current`, and `overlay/{upper,work,merged}`. Add singleton shared-game state containing active/previous release, migration stage, operation journal identity, and timestamps. Keep scheduled `instance_id` non-null; empty means global.

**Repair track:** Replace ad hoc ownership of `instances/<id>/game` paths.

**Retirement track:** Only migration may read the legacy path; cleanup removes it after the rollback grace period.

**Verification:** `go test ./internal/config ./internal/store -count=1`

## Task 2: Restricted Overlay Helper

**Files:** Create `cmd/overlay-helper/main.go`, `internal/overlayfs/{paths,server,client,mount_linux,mount_stub}.go`, tests, `overlay-helper/Dockerfile`; modify `docker-compose.yml` and `Makefile`.

Implement an allowlisted Unix-socket protocol for `preflight`, `ensure`, `inspect`, `reset-managed-paths`, and `unmount`. Confine all resolved paths beneath the configured root, reject symlinks/traversal, verify lower immutability and upper/work filesystem constraints, and inspect `/proc/self/mountinfo` before mutation.

Run with no network, read-only root, `CAP_SYS_ADMIN`, `user: 0:10001`, and only the data-root `rshared` bind plus socket volume. Do not widen Docker socket-proxy access.

**Verification:** `go test ./internal/overlayfs ./cmd/overlay-helper -count=1; docker compose config`

## Task 3: Separate Runtime And SteamCMD Paths

**Files:** `internal/docker/lifecycle.go`, `internal/docker/client.go`, `internal/provisioning/service.go`, and tests.

Bind game containers from `instances/<id>/overlay/merged`. Replace instance-oriented SteamCMD methods with installation into an explicit staging release. Preserve cache, credentials, anonymous retry classification, maintenance adoption, and bounded logs.

Provisioning ensures one shared release, mounts the instance overlay, then applies selected package and private content. Concurrent first-instance creation collapses to one shared install.

**Retirement track:** Remove instance-based `InstallGame`/`UpdateGame` after all callers move.

**Verification:** `go test ./internal/docker ./internal/provisioning -count=1`

## Task 4: Overlay-Aware Reconciliation

**Files:** Create `internal/content/reconciler.go` and tests; modify `internal/updates/pipeline.go`, `internal/content/private_state.go`, and tests.

Combine old/new package and private manifests. Hot apply clears whiteouts and resets only owned paths before writing new content. Full apply prepares an empty upper generation, writes package then private content, and atomically remounts while stopped.

Extend existing journals with overlay generation and mount identity so rollback restores content and the prior mount. Reject writes while a global exclusive operation owns the maintenance gate.

**Verification:** `go test ./internal/content ./internal/updates -count=1`

Expected: private beats package beats shared; managed mutations are corrected; unrelated runtime files survive hot apply and disappear after full reconciliation.

## Task 5: Global Gate And Update Coordinator

**Files:** Create `internal/maintenance/gate.go` and tests; refactor `internal/updates/game.go`; modify `internal/players/service.go`, `cmd/panel/main.go`, and tests.

Add cancellable shared/exclusive leases. Instance lifecycle, content, deletion, and provisioning use shared leases; game update/migration use exclusive leases.

Refactor game update to list instances, aggregate player checks, repeat checks under the exclusive gate, record/stop active instances, stage/validate a release, fully reconcile/remount all overlays, publish `current`, and restore only current desired-running instances. Persist stages for startup rollback/resume.

**Verification:** `go test ./internal/maintenance ./internal/updates ./internal/players -count=1`

Expected: starts cannot cross final player validation; partial remount/restart failures restore the prior release.

## Task 6: Global Schedule Contract

**Files:** `internal/automation/dispatcher.go`, `internal/scheduler/service.go`, `internal/store/migrations.go`, and tests.

Dispatch `game_update` without an instance. Apply `skip`, `wait`, or `force` across all active dependent instances; query failure never counts as empty. Reject new global tasks with non-empty instance IDs.

Migrate legacy game-update rows idempotently to empty instance IDs. If multiple enabled rows collapse to equivalent global schedules, retain the oldest enabled and disable others with audit events.

**Verification:** `go test ./internal/automation ./internal/scheduler ./internal/store -count=1`

## Task 7: Global HTTP And Overview

**Files:** `internal/httpapi/server.go`, `internal/httpapi/server_test.go`, `web/src/api/client.ts`, `web/src/app/App.tsx`, `web/src/app/App.test.tsx`.

Add `GET /api/game` and `POST /api/game/update`, queueing one Job with empty instance ID. Change overview Update to package reinstall only. For one compatibility release, reject game requests on `/api/instances/{id}/game-update` with `409 game_update_is_global` and route package-only payloads normally.

Show active release/global update status in the content or system area, not each instance card.

**Verification:** `go test ./internal/httpapi -count=1; npm --prefix web test -- --run src/app/App.test.tsx`

## Task 8: Schedule Editor Scope

**Files:** `web/src/app/SchedulesPage.tsx`, `web/src/app/SchedulesPage.test.tsx`, `web/src/styles/app.css`.

Hide the instance selector for `game_update`, clear stale instance state when type changes, submit `instance_id:""`, and state that policy covers all dependent servers. Continue requiring instances for instance-scoped tasks.

**Verification:** `npm --prefix web test -- --run src/app/SchedulesPage.test.tsx`

## Task 9: Resumable Legacy Migration

**Files:** Create `internal/migration/sharedgame.go` and tests; modify `cmd/panel/main.go`, `internal/httpapi/server.go`, and tests.

Implement preflight, fresh shared install, legacy-directory rename, overlay mount, package/private replay, smoke checks, completion marker, and rollback. Expose authenticated status and explicit confirmed migration; do not auto-run destructive migration at startup. Block instance starts while migration is required or incomplete.

Keep `legacy-game.<migration-id>` until explicit cleanup verifies migration completion, no references, elapsed grace period, and a previous known-good release.

**Verification:** `go test ./internal/migration ./internal/httpapi ./cmd/panel -count=1`

Expected: failure injection at each journal stage restores legacy directories and leaves old-version rollback possible.

## Task 10: Deployment And End-To-End Proof

**Files:** `deployment_test.go`, `cmd/e2e-fixture/*`, `web/e2e/control-panel.spec.ts`, `README.md`; create `docs/aegis/work/2026-07-17-shared-game-overlay/50-evidence.md`.

Document Linux/OverlayFS/shared-propagation requirements, layout, migration/rollback, cleanup, and undeclared-runtime-file loss. Use a fake mount manager in portable tests and Linux-only real OverlayFS integration coverage.

Run:

```powershell
gofmt -w cmd internal
go test ./... -count=1
go vet ./...
npm --prefix web test -- --run
npm --prefix web run build
npm --prefix web run test:e2e
git diff --check
```

Record two-instance isolation, aggregate player gating, publication rollback, helper/Panel restart recovery, schedule migration, and desktop/mobile UI evidence.

## Delivery Sequence

Tasks 1-4 land behind a disabled capability. Tasks 5-8 enable shared mode only after helper preflight. Task 9 explicitly migrates existing installations. Remove the compatibility route and legacy cleanup path only in a later release after field evidence.
