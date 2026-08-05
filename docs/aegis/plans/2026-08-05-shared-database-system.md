# Shared Database System Implementation Plan

## Goal

Implement one visually editable SourceMod database configuration shared by every game instance, enforced after every lower-layer file mutation and exposed as an observable background Job.

## Invariants

- Structured panel configuration is authoritative.
- Raw `databases.cfg` is never accepted from the browser.
- Credentials never appear in logs or Job messages.
- Package and private layers run before reconciliation.
- Start and restart fail closed on reconciliation failure.
- Existing Job and instance API shapes remain compatible.

### Task 1: Model, Store, And Renderer

**Files:** create `internal/databaseconfig/model.go`, `internal/databaseconfig/render.go`, tests, and `internal/store/database_config.go`.

**Verification:** `go test ./internal/databaseconfig ./internal/store -run 'Test.*Database' -count=1`

- [ ] Write failing tests for exact defaults, validation, rendering, escaping, optional fields, SQLite localhost omission, persistence, and revisions.
- [ ] Implement structured normalization, validation, deterministic KeyValues rendering, and `system_settings` persistence.
- [ ] Run focused tests until green.

### Task 2: Atomic Instance Synchronizer

**Files:** create `internal/databaseconfig/synchronizer.go` and tests.

**Verification:** `go test ./internal/databaseconfig -run 'Test.*Sync' -count=1`

- [ ] Write failing tests for atomic replacement, installed/deferred instances, partial failure, path safety, and redacted progress.
- [ ] Implement full and single-instance synchronization using one rendered payload.
- [ ] Run focused tests until green.

### Task 3: Observable Job API

**Files:** modify `internal/httpapi/server.go`, tests, `cmd/panel/main.go`, and `cmd/e2e-fixture/main.go`.

**Verification:** `go test ./internal/httpapi ./cmd/panel ./cmd/e2e-fixture -run 'Test.*Database' -count=1`

- [ ] Write failing API tests for get/defaults/save/resync, validation, revision conflicts, global serialization, failure summaries, and credential redaction.
- [ ] Add authenticated routes and create existing-shape `database_sync` Jobs with visible stages and logs.
- [ ] Wire production and fixture dependencies, then run focused tests.

### Task 4: Reconciliation Hooks

**Files:** modify update pipeline, provisioning, lifecycle, overlay recovery, and their tests.

**Verification:** focused tests in `internal/updates`, `internal/provisioning`, and `internal/lifecycle`.

- [ ] Write failing ordering tests proving reconciliation runs after package/private writes.
- [ ] Write failing start/restart tests proving reconcile-first and fail-closed behavior.
- [ ] Add narrow reconciliation interfaces at every approved mutation boundary without nested Jobs.
- [ ] Run focused tests until green.

### Task 5: Database System Page

**Files:** create `web/src/app/DatabaseSettings.tsx` and tests; modify `App.tsx`, app tests, and CSS.

**Verification:** `npm test -- --run src/app/DatabaseSettings.test.tsx src/app/App.test.tsx`

- [ ] Write failing tests for navigation, structured editing, duplicates, SQLite behavior, password reveal, defaults, save, resync, conflicts, and Job handoff.
- [ ] Build the global page in the current industrial visual language with accessible controls.
- [ ] Connect returned Jobs to the existing live observer and show synchronization summaries.
- [ ] Run focused frontend tests until green.

### Task 6: Acceptance And Regression

**Files:** modify Playwright coverage and `README.md`; create `docs/aegis/work/2026-08-05-shared-database-system/50-evidence.md`.

**Verification:** `gofmt`, `git diff --check`, `go test ./...`, frontend tests, frontend build, and focused Playwright journey.

- [ ] Verify save creates an observable Job with progress and logs.
- [ ] Verify plugin reinstall cannot leave its packaged `databases.cfg` authoritative.
- [ ] Document overwrite guarantees, SQLite behavior, and retry workflow.
- [ ] Record exact verification evidence and residual risks.

## Self-Review

The plan covers persistence, rendering, background Jobs, UI, every overwrite boundary, failure semantics, security, and acceptance. Existing writers remain; only their ability to leave `databases.cfg` authoritative retires.

