# Verification Evidence

## Regression cycle

- RED: `TestStartEnsuresOverlayBeforeAcceleratorForExistingContainer` observed `accelerator -> start`.
- RED: `TestStartEnsuresOverlayBeforeAcceleratorForNewContainer` observed `accelerator -> create -> overlay -> start`.
- RED: Compose test could not find scoped package initialization.
- GREEN: focused lifecycle, provisioning, updates, and deployment tests pass after the implementation.

## Local verification status

- Focused fresh run passed: `go test -count=1 -p 1 . ./internal/lifecycle ./internal/provisioning ./internal/updates`.
- `go vet ./...` passed.
- `go build ./cmd/panel` passed.
- `git diff --check` passed.
- Full suite `go test -count=1 -p 1 ./...` reached every package. Two unrelated Windows `testing.TempDir` asynchronous cleanup failures occurred: `TestReconcileRebuildsLegacyGameContainersWithPersistentLogMounts/running` and `TestCleanupPackagesProtectsSelectedAppliedAndLatest`.
- Both named failures passed when rerun individually with `-count=1 -p 1`, confirming the known cleanup race rather than a deterministic regression in this repair.
- Local Docker Compose validation is unavailable because the Windows host has no `docker` executable; validate on 琥珀 before deployment.

## Scope and residual risk

- Compose `panel-init` changes only `/srv/l4d2-panel/packages` ownership recursively.
- Existing startup recovery still uses the validated `game/current` link only when shared state is not ready.
- Remote Compose validation used the active project configuration and the existing network override; Panel remained healthy and the three helper container IDs did not change.
- The first package reinstall job `ca383783-a7b8-4a14-9731-d49c826112c0` reached package deployment and private-file application, then failed at `reinstall Accelerator after package deployment: Accelerator token is not configured`. Its logs contained no `permission denied`.
- After synchronizing the existing non-empty crash-report token into the active release `.env` and restarting only Panel, package reinstall job `cad80c0c-31e2-4ebf-9ce8-e58f6932a461` finished `succeeded` in about 24 seconds. Logs show package reuse, full deployment, private apply, and completion; no `permission denied` occurred.
- Start job `d00b1197-bea6-4e39-8cc2-38966eb684da` finished `succeeded` in about 26 seconds. The target instance ended `running`, the container was running, and the Overlay mount was `rw` with the expected lower release. UID `10001` successfully read `core.cfg` (`mode=640`, `size=7844`).
- Stop job `9449e9a2-3472-49ca-b248-dc5fcb9edbcf` finished `succeeded`; final API state for all seven instances was `desired=stopped`, `actual=stopped`, and `/api/health` returned `status=ok`, `database=ok`, `containers_running=16`.

## Repair track

- Package ownership: scoped Compose `panel-init` repair plus one-time existing-tree ownership repair.
- Startup ordering: `provisioning.Service.EnsureOverlay` now runs before Accelerator setup and Docker create/start.
- Shared-game fallback: failed/legacy migration resolves only through the validated `game/current` link.

## Retirement track

- The old lifecycle path that started an existing container without ensuring its Overlay is retired from the main start path.
- The manual package `chown` is retained only as the migration for pre-existing root-owned files; future initialization is owned by scoped `panel-init`.
- The `game/current` fallback remains only as a bounded recovery compatibility path until shared-game migration is repaired and verified `ready`.

## Evidence boundary and confidence

- Evidence used: fresh local commands, SSH API responses, background job records/logs, Docker inspection, `findmnt`, and UID `10001` file-read checks.
- Not loaded: full remote Docker logs and unrelated historical sessions; only bounded task logs and targeted runtime output were used.
- Confidence: `B` for the requested package-install/startup path. Direct runtime evidence and regression coverage support the repair; the remote shared-game migration remains `failed`, and the complete Go suite is recorded separately below.
