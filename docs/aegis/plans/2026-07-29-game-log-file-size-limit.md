# Game Log File Size Limit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Limit every persistent game log file to a configurable size while retaining its newest content.

**Architecture:** Extend the existing system-settings and game-log API contract, then make the existing maintenance manager perform age deletion followed by atomic tail trimming. Reuse the daily scheduler and immediate cleanup workflow; expose the value through the existing settings form.

**Tech Stack:** Go, SQLite, robfig/cron, React, TypeScript, Vitest, Testing Library

**Baseline / Authority Refs:** `CONTEXT.md`, `docs/aegis/specs/2026-07-29-game-log-file-size-limit-design.md`, existing persistent game-log contracts

**Compatibility Boundary:** Existing retention, tree, preview, download, cleanup scheduling, and job reporting behavior must remain available; only the settings JSON contract gains a required field.

**Verification:** `go test ./internal/store ./internal/gamelogs ./internal/httpapi`, frontend targeted tests, `go test ./...`, `npm test -- --run`, and `npm run build` from `web`.

---

### Task 1: Persist The Size Setting

**Files:** Modify `internal/store/job_history.go`; test `internal/store/job_history_test.go`.

**Why this task exists:** Every maintenance run needs one validated, durable maximum with a safe 10 MiB default.

**Impact / Compatibility:** Reuse `system_settings`; no schema migration or existing key changes.

**Verification:** `go test ./internal/store -run GameLog`

- [ ] Write failing tests for the default, persisted value, and rejection outside 1-1024 MiB.
- [ ] Run the focused tests and confirm missing-method or wrong-default failures.
- [ ] Add constants, key, getter, setter, and shared validation.
- [ ] Run focused and package tests to green.

### Task 2: Trim Oversized Files Safely

**Files:** Modify `internal/gamelogs/manager.go`; test `internal/gamelogs/manager_test.go`.

**Why this task exists:** Maintenance must retain the newest diagnostic content without crossing controlled filesystem boundaries.

**Impact / Compatibility:** Expand cleanup input/result while preserving expired-file deletion and instance locking.

**Verification:** `go test ./internal/gamelogs -run 'Cleanup|Trim'`

- [ ] Write failing tests for exact limit, oversized tail retention, combined expiry and trim statistics, symlink handling, and a changed file before replacement.
- [ ] Run tests and confirm failures come from absent trimming behavior.
- [ ] Add max-size validation, bounded in-place tail copy, identity recheck, truncation, and result counters while preserving active writer file handles.
- [ ] Run focused and package tests to green.

### Task 3: Propagate Settings Through Scheduler And API

**Files:** Modify `internal/gamelogs/scheduler.go`, `internal/gamelogs/scheduler_test.go`, `internal/httpapi/game_logs.go`, and `internal/httpapi/server_test.go`.

**Why this task exists:** Daily, manual, and save-triggered maintenance must all use the configured limit.

**Impact / Compatibility:** Extend repository interfaces and JSON bodies; preserve the existing route paths and enqueue response.

**Verification:** `go test ./internal/gamelogs ./internal/httpapi -run 'GameLog|Cleanup'`

- [ ] Write failing scheduler tests proving the stored byte limit reaches cleanup.
- [ ] Write failing API tests for GET/PUT validation and immediate enqueue only when either policy becomes stricter.
- [ ] Run focused tests and confirm contract failures.
- [ ] Implement repository propagation, request/response fields, validation, and enqueue comparison.
- [ ] Run focused and package tests to green.

### Task 4: Add The Settings Control

**Files:** Modify `web/src/app/App.tsx` and `web/src/app/App.test.tsx`.

**Why this task exists:** Administrators need to view and change the per-file maximum alongside retention.

**Impact / Compatibility:** Preserve existing form pending, rollback, notice, and accessibility behavior.

**Verification:** `npm test -- --run src/app/App.test.tsx` from `web`.

- [ ] Write failing UI tests for loading 10 MiB, rejecting values outside 1-1024, and sending both settings.
- [ ] Run the focused test and confirm the new labeled input is absent.
- [ ] Add confirmed/draft state, load validation, numeric input, save payload, and error rollback.
- [ ] Run the focused frontend tests to green.

### Task 5: Regression And Evidence

**Files:** Update `docs/aegis/work/2026-07-29-game-log-file-size-limit/50-evidence.md` only if persistent evidence is needed.

**Why this task exists:** The change spans storage, filesystem maintenance, HTTP, scheduling, and UI.

**Impact / Compatibility:** No new behavior; verify all prior contracts and production build.

**Verification:** Complete command suite below.

- [ ] Run `gofmt` on changed Go files.
- [ ] Run `go test ./...`.
- [ ] Run `npm test -- --run` from `web`.
- [ ] Run `npm run build` from `web`.
- [ ] Review the final diff for unrelated changes and record residual risks.
