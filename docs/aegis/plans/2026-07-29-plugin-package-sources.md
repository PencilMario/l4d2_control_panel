# Plugin Package Sources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Separate fixed packages from GitHub package sources and make every instance plugin update a full deployment that resolves and, for GitHub sources, synchronizes the latest Release.

**Architecture:** `domain.Instance` owns one stable selection (`SelectedPackageID` or `PackageSourceID`) and one applied concrete package ID. A single `GameCoordinator.Reinstall` path resolves the selection, asks an injected GitHub synchronizer to fetch source releases when needed, and always deploys with `updates.Full`; content-source checks remain repository-only operations.

**Tech Stack:** Go 1.24, SQLite/modernc, chi HTTP API, React 19, TypeScript, Vitest.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-29-plugin-package-source-design.md`, `CONTEXT.md`, `internal/updates/pipeline.go`, and existing instance desired-state/transaction contracts.

**Compatibility Boundary:** Preserve instance job serialization, deployment rollback, private-file replay, desired-state restoration, shared-game updates, existing applied package IDs, and read migration from `package_source_repository`. Do not deploy instances from the content repository source-sync action.

**Verification:** `go test ./...`, `npm test -- --run`, `npm run build`, plus focused red/green commands listed below.

---

### Task 1: Stable Package Source Identity And Migration

**Files:**
- Modify: `internal/domain/models.go`
- Modify: `internal/store/migrations.go`
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Why this task exists:** Instances currently persist a repository string and concrete package ID as competing owners of selection state. A stable `source_id` is required so Release versions can change without changing the instance choice.

**Impact / Compatibility:** Existing repository strings must migrate only on a unique `github_sources.repository` match. Fixed package selections and `package_version` remain unchanged.

**Repair Track:** Make `PackageSourceID` the canonical GitHub selection owner and enforce package/source mutual exclusion on writes.

**Retirement Track:** Stop writing `package_source_repository`; retain the column only as migration input until a future table-rebuild migration removes it.

- [ ] Write store tests that open a legacy database, assert a unique repository becomes `PackageSourceID`, assert old hot schedule types become full types, and assert ambiguous/unmatched repositories remain diagnosable instead of guessed.
- [ ] Run `go test ./internal/store -run 'Test.*(PackageSource|HotSchedule)' -count=1`; expect failure because `package_source_id` and migration version 9 do not exist.
- [ ] Add `PackageSourceID string \`json:"source_id"\``; add `package_source_id` to schema, SQL fields, scans, inserts and updates; implement migration 9 in one transaction:

```sql
ALTER TABLE instances ADD COLUMN package_source_id TEXT NOT NULL DEFAULT '';
UPDATE instances
SET package_source_id = COALESCE((
  SELECT MIN(id) FROM github_sources
  WHERE repository = instances.package_source_repository
  HAVING COUNT(*) = 1
), '')
WHERE package_source_repository <> '';
UPDATE scheduled_tasks SET type='package_full' WHERE type='package_hot';
UPDATE scheduled_tasks SET type='release_full' WHERE type='release_hot';
```

- [ ] Normalize `UpdateInstance`/`CreateInstance` so non-empty `PackageSourceID` clears `SelectedPackageID`, and a fixed selection clears `PackageSourceID`.
- [ ] Run `go test ./internal/store ./internal/domain -count=1`; expect pass.

### Task 2: One Full Instance Plugin Update Coordinator

**Files:**
- Modify: `internal/updates/game.go`
- Modify: `internal/updates/coordinator.go`
- Modify: `internal/provisioning/service.go`
- Modify: `internal/updates/shared_reconciler.go`
- Test: `internal/updates/game_test.go`
- Test: `internal/provisioning/service_test.go`
- Test: `internal/updates/shared_reconciler_test.go`

**Why this task exists:** Instance updates must resolve a stable source to a concrete package while preserving selection, and must have no hot-deployment branch.

**Impact / Compatibility:** Deployment still uses the existing full transaction pipeline and lifecycle recovery. Shared-game reconciliation and first provisioning resolve sources without changing the selected source.

**Repair Track:** `GameCoordinator.Reinstall` becomes the canonical manual/scheduled instance plugin deployment owner.

**Retirement Track:** Remove the `Hot` coordinator branch and stop `markApplied` from changing `SelectedPackageID`; keep `Mode` in the low-level pipeline only where old journal decoding needs it.

- [ ] Add failing tests for fixed package full reinstall, source resolver full reinstall, selection preservation, applied ID update only after commit, and failure preserving the applied ID.
- [ ] Run `go test ./internal/updates -run 'TestGameReinstallPackage' -count=1`; expect source-ID and selection-preservation failures.
- [ ] Define a resolver contract used by provisioning/reconciliation:

```go
type PackageResolver interface {
    Get(string) (content.PackageVersion, error)
    LatestSourceVersion(string) (content.PackageVersion, error)
}
```

Resolve GitHub selections by looking up `GitHubSource` from `PackageSourceID`, then `LatestSourceVersion(source.Repository)`. Always call `Deployer.Begin(..., Full)`.
- [ ] Change applied-state writes to update only `PackageVersion`. Remove all production calls that pass `updates.Hot`.
- [ ] Run `go test ./internal/updates ./internal/provisioning -count=1`; expect pass.

### Task 3: GitHub-Aware Instance Update API

**Files:**
- Modify: `internal/httpapi/server.go`
- Modify: `cmd/panel/main.go`
- Test: `internal/httpapi/server_test.go`
- Test: `cmd/panel/main_test.go`

**Why this task exists:** Clicking an instance update must work even when the latest Release was not pre-synchronized, while the content source check must remain repository-only.

**Impact / Compatibility:** Preserve `/api/instances/{id}/game-update` as the instance plugin update route and `/api/github-sources/{id}/check` as sync-only. Instance configuration accepts exactly one of `package_id` and `source_id`.

**Repair Track:** Inject a source synchronizer into `GameCoordinator`; fetch before stopping the instance so network/archive errors do not interrupt a running game.

**Retirement Track:** Retire `/api/instances/{id}/updates` mode selection and repository-string instance writes. Keep a temporary endpoint alias only if existing frontend/e2e tests prove it is needed, and route it to full update without accepting `hot`.

- [ ] Add failing HTTP/integration tests that configure `source_id`, run instance plugin update, observe one Release fetch plus one full deployment, and verify source check performs zero deployments.
- [ ] Run `go test ./internal/httpapi ./cmd/panel -run 'Test.*(Source|PackageUpdate)' -count=1`; expect failures from the old request and coordinator contracts.
- [ ] Validate selection with this invariant:

```go
if (input.PackageID == "") == (input.SourceID == "") {
    return errors.New("exactly one package_id or source_id is required")
}
```

For source selection, load the source and synchronize it inside the instance Job before full deployment. Return a validation error for a missing source.
- [ ] Keep `checkGitHubSource` limited to `FetchLatest`; add response/job messaging that says “syncing GitHub source”.
- [ ] Run `go test ./internal/httpapi ./cmd/panel -count=1`; expect pass.

### Task 4: Scheduled Task Convergence

**Files:**
- Modify: `internal/automation/dispatcher.go`
- Modify: `internal/scheduler/service.go`
- Test: `internal/automation/dispatcher_test.go`
- Test: `internal/scheduler/service_test.go`

**Why this task exists:** Scheduled plugin work must use the same full instance update semantics and must not retain a hidden hot-update owner.

**Impact / Compatibility:** Existing hot task rows are migrated in Task 1. `release_check` remains global sync-only; game body, backup and cleanup schedules remain unchanged.

**Repair Track:** Dispatch both `package_full` and `release_full` through the canonical instance package reinstall operation, with Release synchronization driven by the instance source selection.

**Retirement Track:** Remove `package_hot`/`release_hot` from accepted task types and dispatcher branches; remove `updates.Mode` from the dispatcher interface.

- [ ] Replace existing hot-mode tests with failing tests asserting hot types are rejected and full types invoke one canonical reinstall after the online-player policy passes.
- [ ] Run `go test ./internal/automation ./internal/scheduler -count=1`; expect failures while hot types are still accepted.
- [ ] Reduce task types to `game_update`, `package_full`, `release_check`, `release_full`, `backup`, `cleanup`; route full plugin tasks to `GameCoordinator.Reinstall(..., ReinstallOptions{Package:true})`.
- [ ] Run `go test ./internal/automation ./internal/scheduler -count=1`; expect pass.

### Task 5: Frontend Source IDs And Full-Only Language

**Files:**
- Modify: `web/src/api/client.ts`
- Modify: `web/src/app/InstanceConfigModal.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/SchedulesPage.tsx`
- Test: `web/src/app/InstanceConfigModal.test.tsx`
- Test: `web/src/app/App.test.tsx`
- Test: `web/src/app/SchedulesPage.test.tsx`

**Why this task exists:** Users must select a stable GitHub source and see one unambiguous instance update command. The source page must say sync and must never imply instance deployment.

**Impact / Compatibility:** Preserve current dialogs, pending-state guards and instance form ergonomics. Remove only hot-update controls and obsolete payload fields.

**Repair Track:** Load GitHub sources directly into the instance form, submit `source_id`, and make the instance update button trigger one full reinstall that auto-syncs GitHub selections.

**Retirement Track:** Remove `hot_compatible`, repository-derived source options, hot buttons, hot schedule options and `mode` payloads.

- [ ] Update tests first to assert source options use source IDs, package rows have no update buttons, instance update submits `{confirm:true,reinstall_package:true,reinstall_game:false}`, and schedule UI exposes only full plugin tasks.
- [ ] Run `npm test -- --run src/app/InstanceConfigModal.test.tsx src/app/App.test.tsx src/app/SchedulesPage.test.tsx`; expect failures against the old controls and payloads.
- [ ] Add `source_id` to normalized instance/API types; pass `GitHubSource[]` to `InstanceConfigModal`; remove `hot_compatible` from `PackageVersion` and all rendering branches.
- [ ] Rename source action to “同步”, keep it calling only `/api/github-sources/{id}/check`, and remove instance target controls from repository package rows.
- [ ] Remove `package_hot`/`release_hot` schedule metadata and keep `package_full`/`release_full` with full-update descriptions.
- [ ] Run focused Vitest command again; expect pass.

### Task 6: Cross-Layer Verification And Retirement Scan

**Files:**
- Modify: `CONTEXT.md`
- Create: `docs/aegis/work/2026-07-29-plugin-package-sources/50-evidence.md`

**Why this task exists:** This contract and retirement change needs fresh evidence that no hot-update owner or repository-string write path remains.

**Impact / Compatibility:** Documentation records the new ubiquitous language; it does not claim runtime deployment beyond automated evidence.

- [ ] Add definitions for “常规插件包”, “GitHub 源插件包”, “插件来源”, “已部署插件包” and “实例插件更新” to `CONTEXT.md`.
- [ ] Run `gofmt` on changed Go files and `git diff --check`.
- [ ] Run `rg -n 'updates\.Hot|package_hot|release_hot|hot_compatible|package_source_repository' internal web/src`; expect no active production write/update branches (migration/read compatibility occurrences must be individually justified in evidence).
- [ ] Run `go test ./...`; expect all Go packages pass. If the known Windows temp cleanup race recurs, rerun the named test separately and record both results without hiding the suite failure.
- [ ] Run `npm test -- --run`; expect all frontend tests pass.
- [ ] Run `npm run build`; expect TypeScript and Vite build success.
- [ ] Record commands, outputs, retirement scan exceptions and residual browser/E2E risk in `50-evidence.md`.
