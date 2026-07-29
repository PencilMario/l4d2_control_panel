# Detailed Background Task Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every task-center background task log its target, non-sensitive inputs, phases, affected objects, detailed result, duration, and failure context, with five-second download progress.

**Architecture:** Extend the existing context-attached job reporter with non-failing semantic logging helpers and task-wide summaries. Put transfer measurement in a focused `jobs` helper, while each domain owner logs facts only it knows; keep existing progress events and subprocess output intact.

**Tech Stack:** Go 1.24, `context`, `io`, `net/http`, existing `internal/jobs`, `internal/joblogs`, SQLite-backed job history, Go `testing` and `httptest`.

**Baseline / Authority Refs:** `CONTEXT.md`; `docs/aegis/specs/2026-07-30-detailed-background-task-logging-design.md`; current contracts in `internal/jobs/manager.go`, `internal/joblogs/manager.go`, and `internal/domain/models.go`.

**Compatibility Boundary:** Do not change job/event/SSE/frontend schemas, task status or progress behavior, operation cancellation and locking, cleanup continuation rules, secret redaction, raw command output, or the 10 MiB terminal task-log limit.

**Verification:** Targeted package tests after each task, then `go test ./...`, `go vet ./...`, `git diff --check`, and an audit proving every `jobs.Manager.Start` operation reaches the common summaries and every task family emits semantic context.

---

## File Ownership Map

- `internal/jobs/manager.go`: reporter context, task-wide start/success/failure summaries, current phase tracking.
- `internal/jobs/logging.go`: deterministic semantic log functions, byte/duration formatting, safe labels.
- `internal/jobs/transfer.go`: throttled byte-counting reader and transfer start/progress/completion records.
- `internal/releases/github.go`: GitHub Release and asset selection plus package download/reuse detail.
- `internal/gamelogs/manager.go`: per-file game-log delete/trim/skip/failure detail.
- `internal/maintenance/manager.go`: backup and retained-backup cleanup detail.
- `internal/lifecycle`, `internal/updates`, `internal/migration`, `internal/content`, `internal/players`: domain-owned lifecycle, deployment, migration, private-file, and player-operation facts.
- `internal/httpapi/server.go`, `internal/automation/dispatcher.go`, `internal/vpkrestart/coordinator.go`: request/schedule/deferred task parameters and decisions.

### Task 1: Shared Task Logging Contract

**Files:**
- Create: `internal/jobs/logging.go`
- Create: `internal/jobs/logging_test.go`
- Modify: `internal/jobs/manager.go`
- Modify: `internal/jobs/manager_test.go`

**Why this task exists:** Every job needs reliable baseline context even when its domain operation has sparse logging.

**Impact / Compatibility:** The attached reporter remains the canonical owner. Existing `Reporter.Progress`, `Reporter.Log`, job events, and result statuses stay unchanged; new helpers silently do nothing outside a job context.

**Repair Track:** The current manager writes generic “Task started/completed” records and loses task kind, target, duration, and active phase on failure. Add those facts at the common owner.

**Retirement Track:** Keep generic persisted job-event messages for API compatibility. Replace only duplicate generic JSONL terminal messages with richer common summaries after tests prove event snapshots are unchanged.

**Verification:** `go test ./internal/jobs -run 'Test(ContextLog|TaskSummary|Format)' -count=1`

- [ ] **Step 1: Write failing formatting and context tests**

Add table tests asserting `FormatBytes(1536) == "1.50 KiB (1536 bytes)"`, empty context logging is harmless, and `LogContext` records source/level/message through an attached reporter. Add a manager test that starts kind `delete` for instance `alpha`, advances phase `filesystem`, and asserts JSONL messages contain task kind, target, success duration; repeat with a sentinel failure and assert phase plus error.

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test ./internal/jobs -run 'Test(ContextLog|TaskSummary|Format)' -count=1`

Expected: FAIL because formatting helpers and detailed lifecycle summaries do not exist.

- [ ] **Step 3: Implement the minimal shared API**

Create helpers with these public signatures:

```go
func LogContext(ctx context.Context, source string, level joblogs.Level, message string)
func Logf(ctx context.Context, source string, level joblogs.Level, format string, args ...any)
func FormatBytes(value int64) string
func FormatDuration(value time.Duration) string
```

Extend the internal reporter with `kind`, `instanceID`, `startedAt`, and last phase updated by `Progress`. In `Manager.Start`, log a detailed start record before calling the operation and a detailed success or failure record afterward. Use `global` when `instanceID` is empty. Do not put these extra messages into `job_events`.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `go test ./internal/jobs -count=1`

Expected: PASS with existing event/status assertions unchanged.

- [ ] **Step 5: Commit**

```text
feat(jobs): 增加后台任务通用阶段与结果日志
```

### Task 2: Throttled Transfer Logging And GitHub Metadata

**Files:**
- Create: `internal/jobs/transfer.go`
- Create: `internal/jobs/transfer_test.go`
- Modify: `internal/releases/github.go`
- Modify: `internal/releases/github_test.go`

**Why this task exists:** Administrators need periodic size, speed, percentage, and ETA while a large asset downloads, plus the exact Release/tag and asset chosen.

**Impact / Compatibility:** The wrapper must return the exact underlying byte stream and errors. `FetchLatest` and `FetchResult` remain source compatible unless metadata fields are added without changing existing meanings.

**Repair Track:** Current GitHub downloads use `io.Copy` and expose no progress or selected Release identity in task logs.

**Retirement Track:** Retire the direct `io.Copy` only at the GitHub asset copy site. Keep HTTP validation, redirect policy, authentication, archive verification, atomic publication, and package reuse behavior.

**Verification:** `go test ./internal/jobs ./internal/releases -run 'Test(Transfer|FetchLatest)' -count=1`

- [ ] **Step 1: Write failing transfer tests with a fake clock**

Define a test clock advanced explicitly and a recording reporter. Assert start is immediate; no periodic record before five seconds; a record at five seconds only when bytes increased; known totals include exact/human sizes, percent, rate, and ETA; total `-1` omits percent/ETA; completion is immediate; read errors include transferred bytes.

- [ ] **Step 2: Verify transfer tests fail**

Run: `go test ./internal/jobs -run TestTransfer -count=1`

Expected: FAIL because `TransferOptions` and `CopyWithProgress` are undefined.

- [ ] **Step 3: Implement transfer measurement**

Provide:

```go
type TransferOptions struct {
    Source, Filename string
    Total int64
    Interval time.Duration
    Now func() time.Time
}

func CopyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, options TransferOptions) (int64, error)
```

Default `Interval` to five seconds. Calculate speed from bytes and time since the previous emitted sample; emit only when both interval and byte-change conditions hold. Check cancellation without swallowing source or destination errors.

- [ ] **Step 4: Write failing GitHub selection tests**

Extend the `httptest` fixture Release JSON with `name`, `tag_name`, `published_at`, asset `name`, and `size`. Capture task logs and assert repository, Release name, tag, publication time, matched asset name/size, reuse decision, and final bytes appear while token and signed URL query values do not.

- [ ] **Step 5: Verify GitHub tests fail**

Run: `go test ./internal/releases -run TestFetchLatest -count=1`

Expected: FAIL because `FetchLatest` does not log Release metadata or use transfer progress.

- [ ] **Step 6: Integrate and verify GREEN**

Decode the approved metadata fields, log selection before downloading, call `jobs.CopyWithProgress`, and log download/reuse/publication decisions. Keep filenames from structured JSON rather than parsing URLs.

Run: `go test ./internal/jobs ./internal/releases -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```text
feat(downloads): 记录下载进度与 GitHub 发布版本
```

### Task 3: Per-File Cleanup And Backup Detail

**Files:**
- Modify: `internal/gamelogs/manager.go`
- Modify: `internal/gamelogs/manager_test.go`
- Modify: `internal/gamelogs/scheduler_test.go`
- Modify: `internal/maintenance/manager.go`
- Modify: `internal/maintenance/manager_test.go`

**Why this task exists:** A cleanup summary cannot explain what data was deleted or how much each action released.

**Impact / Compatibility:** Preserve path validation, identity rechecks, scan counters, aggregate errors, strict cutoff semantics, in-place trimming, and returned result types. Log only controlled-root-relative labels already used by the managers.

**Repair Track:** Existing game-log cleanup records only aggregate counts; maintenance cleanup returns a count without file identities.

**Retirement Track:** Keep aggregate summaries because they are useful and tested. Add item records at the actual successful/failed action sites; do not add a second filesystem walk.

**Verification:** `go test ./internal/gamelogs ./internal/maintenance -count=1`

- [ ] **Step 1: Write failing item-detail tests**

Run cleanup under a job context with an expired file, oversized file, unchanged file, and injected removal failure. Assert safe relative paths, original/result sizes, released bytes, skip/failure reasons, and final existing counters. Add maintenance fixtures with old and retained archives and assert each removal plus backup archive name/size is logged.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/gamelogs ./internal/maintenance -run 'Test.*Log' -count=1`

Expected: FAIL because item-level messages are absent.

- [ ] **Step 3: Add logging at canonical action points**

Use `jobs.Logf(ctx, "cleanup", ...)` immediately after delete/trim success and at material skip/failure branches. In maintenance, log backup target and final `os.FileInfo.Size`, policy/cutoff, each deleted archive, and retained/deleted totals. Never log absolute `root` paths.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/gamelogs ./internal/maintenance -count=1`

Expected: PASS, including existing race-safety and partial-failure tests.

- [ ] **Step 5: Commit**

```text
feat(cleanup): 逐项记录清理文件与释放空间
```

### Task 4: Lifecycle, Update, Deployment, And Migration Semantics

**Files:**
- Modify: `internal/lifecycle/service.go`
- Modify: `internal/lifecycle/service_test.go`
- Modify: `internal/updates/pipeline.go`
- Modify: `internal/updates/pipeline_test.go`
- Modify: `internal/updates/coordinator.go`
- Modify: `internal/updates/game.go`
- Modify: `internal/updates/shared_game.go`
- Modify: related `internal/updates/*_test.go`
- Modify: `internal/migration/sharedgame.go`
- Modify: `internal/migration/sharedgame_test.go`

**Why this task exists:** Long install/update/migration jobs need decisions and phase meaning in addition to raw SteamCMD/container output.

**Impact / Compatibility:** Preserve maintenance gates, instance locks, desired/actual state updates, stop/start recovery, release activation, rollback, overlay reconciliation, and error composition.

**Repair Track:** Domain decisions are currently implicit, so a generic progress percentage cannot distinguish selection, deployment, restart, rollback, or activation.

**Retirement Track:** Existing HTTP-level `reporter.Progress` calls remain for UI percentages. Domain logging becomes the owner of detailed facts; remove an old generic line only when the new line contains strictly more context at the same action.

**Verification:** `go test ./internal/lifecycle ./internal/updates ./internal/migration -count=1`

- [ ] **Step 1: Write failing semantic log tests**

For lifecycle start/stop/restart/rebuild/delete, assert instance ID, requested transition, container action, data-deletion flag, and resulting decision. For package/game/shared-game paths, assert selected package/release, mode/options, online policy, affected instances, stop/start, activation, and rollback. For migration, assert migration ID, prepared instances, publication, rollback, and final state.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/lifecycle ./internal/updates ./internal/migration -run 'Test.*Log' -count=1`

Expected: FAIL because domain facts are not emitted.

- [ ] **Step 3: Instrument domain owners**

Add `jobs.Logf` calls adjacent to validated decisions and successful state changes. Use `warn` before fallback/rollback, `error` when recording a recoverable sub-operation failure, and `info` for normal phases. Log IDs and logical relative release/package names, never root paths or credentials.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/lifecycle ./internal/updates ./internal/migration -count=1`

Expected: PASS with existing behavior assertions unchanged.

- [ ] **Step 5: Commit**

```text
feat(tasks): 细化实例更新与迁移阶段日志
```

### Task 5: HTTP, Scheduled, Private-File, And Deferred Task Context

**Files:**
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`
- Modify: `internal/automation/dispatcher.go`
- Modify: `internal/automation/dispatcher_test.go`
- Modify: `internal/content/private_state.go`
- Modify: `internal/content/private_test.go`
- Modify: `internal/players/service.go`
- Modify: `internal/players/service_test.go`
- Modify: `internal/vpkrestart/coordinator.go`
- Modify: `internal/vpkrestart/coordinator_test.go`

**Why this task exists:** Entry points own request/schedule inputs, while player, private-file, and deferred-restart owners know affected objects and readiness decisions.

**Impact / Compatibility:** Preserve endpoint responses, scheduled task payload schema, online policy behavior, private-file journaling/recovery, restart coalescing, and existing progress percentages.

**Repair Track:** Several closures ignore their reporter, and scheduled dispatch currently drops the reporter before nested work.

**Retirement Track:** Keep useful progress calls for UI state. Replace raw stage-name messages such as `stage` with descriptive messages only where tests establish the same percentage/stage contract.

**Verification:** `go test ./internal/httpapi ./internal/automation ./internal/content ./internal/players ./internal/vpkrestart -count=1`

- [ ] **Step 1: Write failing entry-point coverage tests**

Capture job logs for reconfigure, delete, instance action, player match/kick/ban operations, instance package update, global game update, shared migration, GitHub fetch/check, private apply, scheduled release/package/game/backup/cleanup, and deferred VPK restart. Assert each includes safe input/target context and nested semantic logs. Assert player identity and command outcome, online wait/skip/force decisions, and private snapshot/apply/rollback object counts.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/httpapi ./internal/automation ./internal/content ./internal/players ./internal/vpkrestart -run 'Test.*(JobLog|TaskLog|Scheduled.*Log)' -count=1`

Expected: FAIL for missing task-specific context.

- [ ] **Step 3: Connect context without changing contracts**

At each job closure, log validated request options before calling the domain owner. Change `Dispatcher.run` to accept the context already containing the reporter and log schedule ID/type/target/policy plus non-sensitive decoded fields. Add player target/action/result, semantic private-file journal/snapshot/application, and deferred-restart registration/readiness/action records.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/httpapi ./internal/automation ./internal/content ./internal/players ./internal/vpkrestart -count=1`

Expected: PASS with HTTP payloads, task status, and progress events unchanged.

- [ ] **Step 5: Commit**

```text
feat(tasks): 补全计划任务与内容操作日志
```

### Task 6: Completeness Audit And Full Regression

**Files:**
- Modify as findings require: task owner tests only
- Create: `docs/aegis/work/2026-07-30-detailed-background-task-logging/50-evidence.md`

**Why this task exists:** “All tasks” is a coverage requirement; package-local success does not prove every registered job path was inspected.

**Impact / Compatibility:** This task adds no new behavior except closing evidence-backed omissions found by the audit.

**Verification:** `go test ./...`; `go vet ./...`; `git diff --check`.

- [ ] **Step 1: Audit every job registration**

Run `rg -n "\.Start\(|startJob\(" internal cmd -g '*.go'` and build a checklist mapping each production registration to: common task summary, target/options log, domain phase log, success/failure test. Also audit direct transfer and deletion sites with `rg -n "io\.Copy|os\.Remove(All)?|FetchLatest" internal -g '*.go'` and distinguish task-center operations from explicit non-goals.

- [ ] **Step 2: Add a failing test for each uncovered task-center path**

Place the test in the canonical owner package and assert the missing observable log fact. Run that exact test and confirm it fails for the omission, not fixture setup.

- [ ] **Step 3: Add the smallest missing instrumentation and rerun targets**

Use the established helpers; do not introduce another logging abstraction or change response schemas.

- [ ] **Step 4: Run full verification**

Run:

```text
go test ./...
go vet ./...
git diff --check
```

Expected: all commands exit 0 with no test failure, vet diagnostic, or whitespace error.

- [ ] **Step 5: Record evidence**

Write the command results, audited registration list, changed files, compatibility checks, and residual risk that externally spawned tools may not expose byte totals into `docs/aegis/work/2026-07-30-detailed-background-task-logging/50-evidence.md`.

- [ ] **Step 6: Commit**

```text
test(tasks): 验证所有后台任务的详细日志覆盖
```

## Risks And Rollback Surface

- Extra JSONL volume is bounded by the existing 10 MiB terminal compaction; per-file cleanup can still truncate early history for unusually large cleanups.
- GitHub `Content-Length` may be absent or differ after redirects; the transfer helper must degrade to byte count/rate only.
- Logging inside locks adds small synchronous I/O overhead. Logs must be placed around material operations, not inside byte-copy loops except the five-second transfer callback.
- Rollback is commit-by-commit: common summaries, transfers/GitHub, cleanup, domain operations, entry points, then audit. No storage migration or API rollback is required.
