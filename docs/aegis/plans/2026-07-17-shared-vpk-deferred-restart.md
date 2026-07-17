# Shared VPK Deferred Restart Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After a new shared VPK is published, persist one deferred restart per running game instance, wait indefinitely for an empty server or three consecutive player-query failures, then restart through the existing serialized Job system.

**Architecture:** A focused `internal/vpkrestart` coordinator owns registration, polling, recovery, deduplication, and restart submission. SQLite owns durable pending state; the upload completion handler only registers after a non-duplicate publish, and the existing Job manager owns the actual lifecycle restart serialization.

**Tech Stack:** Go 1.24, chi HTTP API, SQLite, existing A2S/player service, lifecycle service, persistent Job manager, React task views.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-17-shared-vpk-deferred-restart-design.md`, `CONTEXT.md`, `internal/content/uploads.go`, `internal/jobs/manager.go`, `internal/players/service.go`, `internal/lifecycle/service.go`.

**Compatibility Boundary:** Preserve VPK atomic publication/hash deduplication, existing completion response fields, read-only shared mount and supervisor symlink behavior, per-instance Job serialization, stopped-instance desired state, and scheduler `skip/wait/force` behavior.

**Verification:** Focused Go tests for store/coordinator/HTTP/wiring, `go test ./... -count=1`, `npm test -- --run`, `npm run build`, and `git diff --check`. Known baseline risk: intermittent Windows `testing.TempDir` cleanup failures in `internal/content` and `internal/updates` without assertion failures.

---

## File Ownership

- Create `internal/vpkrestart/coordinator.go`: state machine, polling loop, registration, recovery, Job submission.
- Create `internal/vpkrestart/coordinator_test.go`: deterministic state-machine and concurrency tests with injected clock/check trigger.
- Modify `internal/domain/models.go`: durable pending-restart record.
- Modify `internal/store/migrations.go`: pending-restart table and unique instance key.
- Modify `internal/store/store.go`: repository methods using conditional transitions/upsert.
- Modify `internal/store/store_test.go`: migration, upsert, recovery, and conditional-transition tests.
- Modify `internal/httpapi/server.go`: optional registrar dependency and post-publish registration.
- Modify `internal/httpapi/server_test.go`: new/duplicate/failure upload-trigger contracts.
- Modify `cmd/panel/main.go`: construct/start/stop coordinator.
- Modify `cmd/panel/main_test.go`: dependency-wiring smoke where practical.
- Modify `cmd/e2e-fixture/main.go`: provide compatible coordinator behavior for browser task visibility if required.
- Modify `README.md`: document automatic empty-server restart behavior.

### Task 1: Durable Pending-Restart Repository

**Why this task exists:** Waiting may be indefinite and must survive Panel restart while atomically deduplicating concurrent uploads.

**Impact / Compatibility:** Adds schema only; existing instance, Job, and scheduled-task tables remain unchanged.

**Repair Track:** Canonical ownership moves from nonexistent in-memory intent to SQLite-backed records; the smallest change is one table plus typed repository methods.

**Retirement Track:** No old persistence owner exists. Do not encode this state as long-running Job rows because Job recovery intentionally interrupts active Jobs.

**Verification:** `go test ./internal/store -run 'VPKRestart|Migration' -count=1`.

- [ ] Write failing store tests proving one active row per instance, later publication updates metadata without replacing the original container ID, conditional claim prevents double queueing, and incomplete rows reload after reopen.
- [ ] Run the focused tests and confirm compile/test failure because the record and repository API do not exist.
- [ ] Add `domain.VPKRestart` with instance/container/publication/status/failure-count/timestamps and add schema migration table `shared_vpk_restarts` keyed by `instance_id`.
- [ ] Implement `UpsertVPKRestart`, `PendingVPKRestarts`, `ClaimVPKRestart`, and `UpdateVPKRestart` with SQL transactions and status predicates.
- [ ] Run focused store tests and `go test ./internal/store -count=1`; expect PASS.
- [ ] Commit with `feat: persist shared VPK restart intents`.

### Task 2: Coordinator State Machine

**Why this task exists:** It implements infinite waiting without occupying the Job instance lock and applies the approved empty/failure/state rules.

**Impact / Compatibility:** Reads existing instance/player APIs and submits actual restarts to Jobs; it does not alter scheduler waiting semantics.

**Repair Track:** The coordinator becomes the sole owner of shared-VPK restart decisions. Polling must be triggerable in tests without real 30-second sleeps.

**Retirement Track:** Do not extend `automation.Dispatcher.waitForPlayers`; it remains active only for scheduled tasks.

**Verification:** `go test ./internal/vpkrestart -count=1`.

- [ ] Write failing tests for registration filters, merge/dedup, players-online wait, successful-query failure reset, zero-player queue, third consecutive failure queue, stopped cancellation, changed-container completion, restart failure retry, and recovered pending work.
- [ ] Run tests and confirm failure because `Coordinator` does not exist.
- [ ] Implement repository, instance, player, lifecycle, and Job starter interfaces plus a coordinator with `Register`, `Check`, `Start`, and `Stop`.
- [ ] Make `Register` accept a publication ID and create intents only for instances whose desired/actual states indicate running/startup and whose container ID is nonempty.
- [ ] Implement 30-second polling, three-failure threshold, conditional queue claims, pre-execution state/container recheck, `shared_vpk_restart` Job submission, and retry restoration on submit/restart failure.
- [ ] Ensure a stopped/missing instance cancels, a changed container completes, successful player checks reset failure count, and context cancellation stops all loops.
- [ ] Run focused tests with `-race` where supported; expect each instance to queue at most one restart.
- [ ] Commit with `feat: coordinate deferred shared VPK restarts`.

### Task 3: Upload Completion Integration

**Why this task exists:** Only a newly published, verified VPK should create pending restart work.

**Impact / Compatibility:** Existing `item` and `duplicate` fields remain; registration failure is diagnostic and does not roll back a published file.

**Repair Track:** The HTTP completion boundary is the first place that knows publication succeeded and was not a duplicate.

**Retirement Track:** No trigger exists today. VPK rename/delete/clean remain outside this trigger.

**Verification:** `go test ./internal/httpapi -run VPK -count=1`.

- [ ] Write failing HTTP tests proving new completion calls registrar once, duplicate completion does not call it, failed completion does not call it, and registrar failure returns a successful publication response with diagnostic fields.
- [ ] Run focused tests and confirm failure because the server has no registrar option.
- [ ] Add a narrow `VPKRestartRegistrar` interface and `WithVPKRestartRegistrar` option.
- [ ] After `uploads.Complete`, call `Register` only when `duplicate == false`; use the VPK hash as publication ID and return `restart_instances` plus optional `restart_warning` without removing `item`/`duplicate`.
- [ ] Run focused HTTP tests and existing upload tests; expect PASS.
- [ ] Commit with `feat: schedule restarts after VPK publication`.

### Task 4: Production Wiring and Recovery

**Why this task exists:** The coordinator must start after reconciliation and stop cleanly with the Panel.

**Impact / Compatibility:** Main assembly gains one service; lifecycle/player/job objects remain canonical and are reused.

**Verification:** `go test ./cmd/panel ./cmd/e2e-fixture -count=1` and `go test ./internal/vpkrestart -run Recovery -count=1`.

- [ ] Add a failing wiring or construction test proving the API receives the registrar and shutdown closes the coordinator.
- [ ] Construct the coordinator in `cmd/panel/main.go` after lifecycle reconciliation and Job/player creation, start its recovery loop, pass it to HTTP, and defer/attach shutdown before database close.
- [ ] Update the E2E fixture only as needed to keep the real upload response contract and task journey deterministic.
- [ ] Run panel, fixture, coordinator recovery, and HTTP tests; expect PASS.
- [ ] Commit with `feat: run shared VPK restart coordinator`.

### Task 5: Task Visibility and Documentation

**Why this task exists:** Administrators must see why an automatic restart is waiting or executing.

**Impact / Compatibility:** Reuse existing Job event/log UI; do not add a configuration page.

**Verification:** `npm test -- --run`, `npm run build`, and focused API Job tests.

- [ ] Add/adjust tests proving `shared_vpk_restart` Jobs render their waiting cause and restart result in existing task views; if pending intents are not represented by Job rows, add a minimal read API and task-row adapter for waiting status.
- [ ] Implement the smallest visibility path that shows online count, failure count, queued/retry state, and final Job result without holding a running Job during player wait.
- [ ] Update README content repository documentation with publish, one-intent-per-instance, empty-server waiting, three-query-failure fallback, and stopped-instance behavior.
- [ ] Run focused frontend/API tests and build; expect PASS with only the existing Vite bundle advisory if present.
- [ ] Commit with `docs: explain automatic VPK restart workflow` (include UI/API code in a preceding `feat` commit if code changes are required).

### Task 6: End-to-End Regression and Evidence

**Why this task exists:** Cross-layer concurrency and recovery need proof beyond unit slices.

**Impact / Compatibility:** No new behavior; verification and evidence only.

**Verification:** Commands below.

- [ ] Run `gofmt` on changed Go files and `git diff --check`.
- [ ] Run `go test ./internal/store ./internal/vpkrestart ./internal/httpapi ./cmd/panel -count=1` and confirm PASS.
- [ ] Run `go test ./... -count=1`; distinguish any known Windows TempDir cleanup-only failure from assertion failures and rerun affected focused packages once for evidence.
- [ ] Run `npm test -- --run` and `npm run build` in `web`; confirm PASS.
- [ ] Record commands, outputs, known baseline cleanup risk, compatibility checks, and residual risks in `docs/aegis/work/2026-07-17-shared-vpk-deferred-restart/50-evidence.md`.
- [ ] Commit evidence with `docs: record shared VPK restart verification`.

## Risks and Rollback Surface

- A false empty reading can restart an active match; using the existing merged player snapshot and three failures only for query errors limits this risk.
- Lifecycle state can change between polling and Job execution; the mandatory second state/container check prevents stale restarts.
- Upload publication cannot be rolled back after registrar failure; response diagnostics and logs expose manual remediation while preserving an already durable VPK.
- Reverting the feature requires disabling coordinator wiring first, then retaining the additive table harmlessly or removing it in a later explicit migration.
