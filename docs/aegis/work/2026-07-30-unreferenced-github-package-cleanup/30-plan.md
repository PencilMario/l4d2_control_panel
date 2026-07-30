# Unreferenced GitHub Package Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the existing cleanup schedule to delete local GitHub package versions that are neither referenced by an instance nor the latest version of their repository.

**Architecture:** `content.PackageManager` owns deterministic candidate selection and paired `.zip`/`.json` removal. `maintenance.Manager` resolves protected instance package IDs and invokes that operation after existing retained-file cleanup; `cmd/panel` injects the store and package manager. Release synchronization stops calling its historical no-op cleanup hook.

**Tech Stack:** Go, filesystem package storage, SQLite-backed instance repository, Go tests.

**Baseline / Authority Refs:** `CONTEXT.md`; `docs/aegis/specs/2026-07-30-unreferenced-github-package-cleanup-design.md`; `internal/content/packages.go`; `internal/maintenance/manager.go`; `internal/store/store.go`.

**Compatibility Boundary:** Keep schedule types, payloads, API/UI, retention behavior, GitHub synchronization, and all regular uploaded packages unchanged. Never delete an instance's selected or applied package, or a repository's latest package.

**Verification:** `go test ./internal/content ./internal/releases ./internal/maintenance ./internal/automation ./cmd/panel -count=1`, followed by `go test ./... -count=1`; rerun any known Windows `t.TempDir` cleanup race separately and record both results.

---

### Task 1: Package Cleanup Owner

**Files:**
- Modify: `internal/content/packages.go`
- Modify: `internal/content/packages_test.go`
- Modify: `internal/releases/github.go`
- Modify: `internal/releases/github_test.go`

**Why this task exists:** Package metadata and paired files must be evaluated and removed by the component that owns their layout.

**Impact / Compatibility:** Source synchronization continues to retain historical releases; only the scheduled cleanup invokes deletion. Regular packages and the per-repository latest version remain protected.

**Repair Track:** Replace the deliberately empty `RemoveSourceVersionsExcept` hook with a context-aware bulk cleanup operation returning counts and bytes.

**Retirement Track:** Remove the synchronization-time call and its no-op retention test; scheduled maintenance becomes the sole cleanup owner.

**Verification:** `go test ./internal/content ./internal/releases -count=1`.

- [ ] Add tests creating multiple source repositories, selected/applied protected IDs, regular packages, equal timestamps, cancellation, and injected removal failure; assert deterministic retention, paired deletion, continuation, results, and errors.
- [ ] Run `go test ./internal/content -run TestCleanupUnreferencedSourceVersions -count=1`; expect compile failure because the cleanup API does not exist.
- [ ] Add `PackageCleanupResult` and `CleanupUnreferencedSourceVersions(context.Context, map[string]bool)`, with deterministic latest selection, safe paired removal, aggregated errors, logging, and cancellation.
- [ ] Remove `RemoveSourceVersionsExcept` and its call from `releases.Client.FetchLatest`; update release tests to require historical retention without the hook.
- [ ] Run `gofmt` and `go test ./internal/content ./internal/releases -count=1`; expect PASS.

### Task 2: Maintenance Integration

**Files:**
- Modify: `internal/maintenance/manager.go`
- Modify: `internal/maintenance/manager_test.go`

**Why this task exists:** Only maintenance can safely resolve every instance reference before authorizing package removal.

**Impact / Compatibility:** Existing backup and upload temporary-file retention stays unchanged. Failure to enumerate instances prevents package deletion and returns an error.

**Repair Track:** Add optional package cleanup dependencies to `maintenance.Manager`, preserving `New(root)` for existing callers and tests.

**Retirement Track:** No existing file cleanup branch retires; the package phase is appended to the canonical cleanup operation.

**Verification:** `go test ./internal/maintenance -count=1`.

- [ ] Add tests with fake instance repositories proving both `SelectedPackageID` and `PackageVersion` are protected, regular packages remain, candidate packages are deleted, repository failure blocks package deletion, and summary logs contain no absolute root.
- [ ] Run `go test ./internal/maintenance -run TestCleanupPackages -count=1`; expect compile or behavioral failure before integration.
- [ ] Add `WithPackageCleanup` option/dependencies, build the protected-ID set from `Instances`, invoke the package cleanup phase, combine errors, and log its summary.
- [ ] Run `gofmt` and `go test ./internal/maintenance -count=1`; expect PASS.

### Task 3: Production Wiring And Regression

**Files:**
- Modify: `cmd/panel/main.go`
- Modify: `internal/automation/dispatcher_test.go` only if construction coverage requires it.
- Create: `docs/aegis/work/2026-07-30-unreferenced-github-package-cleanup/50-evidence.md`

**Why this task exists:** The scheduled cleanup must receive the real SQLite store and package manager in production.

**Impact / Compatibility:** Dispatcher and scheduler contracts remain unchanged; only dependency construction changes.

**Repair Track:** Construct maintenance with `maintenance.WithPackageCleanup(db, packageManager)`.

**Retirement Track:** The unconfigured production manager construction retires; unconfigured construction remains supported for focused file-maintenance tests.

**Verification:** Target regression command plus full Go suite.

- [ ] Add or adjust a construction-level test if needed to prove the injected dependencies compile through the dispatcher.
- [ ] Wire `maintenance.New(cfg.DataRoot, maintenance.WithPackageCleanup(db, packageManager))` in `cmd/panel/main.go`.
- [ ] Run `gofmt` and `go test ./internal/content ./internal/releases ./internal/maintenance ./internal/automation ./cmd/panel -count=1`; expect PASS.
- [ ] Run `go test ./... -count=1`; expect PASS, or separately verify and record only the documented Windows temp-directory cleanup race.
- [ ] Record commands, outputs, scope, drift check, and residual risk in `50-evidence.md`.
