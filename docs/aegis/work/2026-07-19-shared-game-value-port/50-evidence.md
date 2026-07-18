# Shared Game Value Port Evidence

Date: 2026-07-19

## Scope

- Derive the installed shared-game `PatchVersion` for `/api/game` and restore its status/update UI.
- Count managed Overlay instance storage from `overlay/upper` and include all storage fields in React memo comparison.
- Execute package and Release update schedules from the target instance's current package configuration.
- Preserve explicit GitHub source selection for `release_check`.

## TDD evidence

- `go test ./internal/httpapi ./cmd/panel` failed with undefined `readSharedGameVersion` and `WithSharedGamePath`, then passed after implementation.
- `npm test -- --run src/app/App.test.tsx` failed because the shared-game version helper and status UI were absent, then passed 57/57 after implementation.
- `go test ./internal/metrics -run TestDirectoryStorageCountsOverlayUpperInsteadOfMerged` failed with `game usage = 0, want 5`, then `go test ./internal/metrics` passed.
- `npm test -- --run src/app/PerformancePanel.test.tsx` failed because storage-only changes compared equal, then passed 12/12 after adding the storage keys.
- `go test ./internal/automation` failed because `Dispatcher.Instances` was absent, then `go test ./internal/automation ./cmd/panel` passed.
- `npm test -- --run src/app/SchedulesPage.test.tsx` failed while the old package and source selectors remained, then passed 7/7 after the form contract changed.

## Full verification

- `$env:GOTMPDIR = Join-Path $env:TEMP 'l4d2-panel-gotmp-shared-port'; go test -p 1 ./...`
  - Exit: 0.
  - All Go packages passed, including `internal/migration`.
- `go vet ./...`
  - Exit: 0 with no diagnostics.
- `npm test -- --run`
  - First full run exposed one stale App integration assertion that still expected a GitHub source selector for `release_hot`.
  - After aligning that assertion with the approved instance-owned contract, the fresh run passed 12 files and 153 tests.
- `npm run build`
  - Exit: 0.
  - Existing warning remains for a minified JavaScript chunk larger than 500 kB.
- `git diff --check`
  - Exit: 0 before each implementation commit.

## Remove / Restore and drift

- No files under `internal/gamelogs` or `web/src/app/GameLogsPage*` changed.
- The older `persistent-game-logs` worktree still has its pre-existing uncommitted `Dockerfile` GOPROXY change; this task did not stage or modify it.
- The incomplete `size_bytes` field from `feature/shared-game-overlay` was intentionally not ported because that branch supplied no canonical size calculation.
- Existing schedule rows and payloads remain readable. For `package_hot`, `package_full`, `release_hot`, and `release_full`, stored package/source IDs are compatibility data and no longer own execution selection.
- Browser-level deployment verification is deferred to the subsequent game-log styling and remote Playwright acceptance requested by the user.

## Confidence

Grade: A for the ported API, metrics, scheduling contracts, unit/integration tests, and build. Runtime browser presentation remains a later acceptance step and is not claimed here.
