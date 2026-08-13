# Shared Cleanup Retention Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `clear` schedule retention value govern maintenance files, game logs, and crash reports while retaining only the game-log file-size system setting.

**Architecture:** Replace the game-log scheduler's independent Cron/Job enqueue path with a synchronous cleaner owned by the scheduled `clear` job. Pass the same validated day count into maintenance, game-log, and crash-report cleaners; remove the crash-report manager's stored retention duration and independent ticker.

**Tech Stack:** Go 1.25, SQLite store, chi HTTP API, React/TypeScript, Vitest, Go test.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-08-13-shared-cleanup-retention-design.md`, `docs/aegis/specs/2026-07-16-schedule-management-design.md`, `internal/automation/dispatcher.go`, `internal/maintenance/manager.go`, `internal/gamelogs/scheduler.go`, `internal/crashreports/manager.go`, `internal/store/job_history.go`, `internal/httpapi/game_logs.go`, `cmd/panel/main.go`, `web/src/app/App.tsx`.

**Compatibility Boundary:** Preserve the scheduled-task database/API shape, cleanup payload key, default 30-day fallback, existing maintenance file scope, game-log size policy, crash-report receive/analyze/download behavior, and `deployment_test.go` user change. Retire only independent retention owners and manual game-log cleanup.

**Verification:** Target Go tests plus related package tests, `go test ./...`, `go vet ./...`, `npm test -- --run`, `npm run build:web`, and `git diff --check`.

---

### Task 1: Record baseline and add failing cleanup contract tests

**Files:**
- Modify: `internal/automation/dispatcher_test.go`
- Modify: `internal/gamelogs/scheduler_test.go`
- Modify: `internal/crashreports/manager_test.go`
- Modify: `internal/httpapi/server_test.go`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/app/SchedulesPage.test.tsx`

**Why this task exists:** Lock the shared retention and retired-interface behavior before implementation.

**Impact / Compatibility:** Tests must express that one `retention_days` value reaches all cleaners, that game-log settings reject the removed field, that independent timers/routes disappear, and that cleanup schedule editing changes its payload.

**Verification:** Run each changed target test before implementation and confirm failures are caused by the missing new contract, not test setup errors.

### Task 2: Replace independent game-log scheduler with clear-owned cleaner

**Files:**
- Modify: `internal/gamelogs/scheduler.go` or replace with `internal/gamelogs/cleaner.go`
- Modify: `internal/gamelogs/scheduler_test.go`
- Modify: `internal/gamelogs/manager.go`

**Why this task exists:** Game logs must use the retention value from the scheduled `clear` task and must not enqueue their own per-instance jobs or daily Cron.

**Impact / Compatibility:** Reuse `Manager.Maintain`; aggregate per-instance results and errors; retain the configured maximum file size. Expand the valid shared retention range to `1..3650`.

**Verification:** Run `go test ./internal/gamelogs -run 'Test(Cleaner|Maintain|Cleanup)' -count=1` and the full package test.

### Task 3: Make crash-report cleanup explicit and retire its timer/config retention

**Files:**
- Modify: `internal/crashreports/config.go`
- Modify: `internal/crashreports/manager.go`
- Modify: `internal/crashreports/manager_test.go`
- Modify: `internal/crashreports/artifacts_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `cmd/panel/main.go`

**Why this task exists:** Crash reports need the scheduled day count and must no longer be cleaned by startup or a hidden daily loop.

**Impact / Compatibility:** Change cleanup to accept explicit retention days, preserve 24-hour pending-token retention and artifact protections, remove `StartCleanup`, stored duration, report-cleanup callback, and obsolete environment parsing/wiring.

**Verification:** Run `go test ./internal/crashreports ./internal/config -count=1` and confirm no production caller starts an independent crash cleaner.

### Task 4: Wire clear to all three cleaners

**Files:**
- Modify: `internal/automation/dispatcher.go`
- Modify: `internal/automation/dispatcher_test.go`
- Modify: `cmd/panel/main.go`
- Modify: `cmd/e2e-fixture/main.go`
- Modify: `cmd/e2e-fixture/main_test.go`

**Why this task exists:** The scheduled `clear` job is the single owner of retention-based cleanup.

**Impact / Compatibility:** Keep maintenance cleanup and its existing default; invoke game-log and crash-report cleaners with the same day count; join errors after attempting all stages; keep job logs safe and structured.

**Verification:** Run automation, fixture, and panel-related Go tests, including a fake-cleaner test that records the exact day count.

### Task 5: Remove game-log retention setting and manual cleanup API

**Files:**
- Modify: `internal/store/job_history.go`
- Modify: `internal/store/store_test.go`
- Modify: `internal/httpapi/game_logs.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`

**Why this task exists:** The system setting must retain only maximum file size and cannot expose a second cleanup trigger without a retention argument.

**Impact / Compatibility:** Keep `system_settings` storage and max-size validation; GET/PUT only use `max_file_size_mb`; remove `retention_days` storage helpers, cleanup endpoint, scheduler option, and route. Existing databases may retain an unused legacy key without affecting behavior.

**Verification:** Run store and HTTP target tests, then `go test ./internal/store ./internal/httpapi -count=1`.

### Task 6: Update settings UI, cleanup schedule editing, docs, and deployment contract

**Files:**
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/app/SchedulesPage.tsx`
- Modify: `web/src/app/SchedulesPage.test.tsx`
- Modify: `web/e2e/control-panel.spec.ts`
- Modify: `README.md`
- Modify: `deployment_test.go`

**Why this task exists:** The administrator must see one authoritative retention field on cleanup plans and no obsolete game-log retention controls.

**Impact / Compatibility:** Preserve file-size input and settings save states; allow cleanup-plan edit to parse and write `retention_days`; remove obsolete environment/deployment documentation and immediate cleanup flow while preserving the user's dump_syms assertion in `deployment_test.go`.

**Verification:** Run focused Vitest tests, full frontend tests, production web build, and relevant Playwright/E2E checks if fixture prerequisites are available.

### Task 7: Full regression and evidence checkpoint

**Files:**
- Modify: `docs/aegis/work/2026-08-13-shared-cleanup-retention/50-evidence.md`

**Why this task exists:** Cross-module contract changes need broad verification before handoff.

**Verification:** Run `gofmt`, `go test ./...`, `go vet ./...`, `npm test -- --run`, `npm run build:web`, and `git diff --check`; inspect `git status` to ensure unrelated user changes remain.
