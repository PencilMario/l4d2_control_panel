# Task Live Log Stream Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist and stream detailed output for nearly every background task, with a dedicated searchable log page, secure Docker log capture, downloads, and a per-terminal-task 10 MiB retention cap.

**Architecture:** Add a file-backed `joblogs.Manager` as the sole owner of JSONL logs, sequencing, redaction, truncation, history reads and subscriptions. Feed task events and Docker maintenance output into it, expose authenticated history/SSE/download endpoints, then add a React viewer while retaining the existing structured task timeline.

**Tech Stack:** Go 1.25, SQLite, JSON Lines, Docker Engine API v1.44, Chi, SSE, React 19, TypeScript, Vitest, Playwright.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-16-task-live-log-stream-design.md`, `docs/aegis/specs/2026-07-16-recent-job-logs-retention-design.md`, `CONTEXT.md`, `README.md`.

**Compatibility Boundary:** Existing job IDs, status transitions, `job_events`, summary SSE, successful-job pruning, lifecycle serialization, audit events and game console transport remain stable. Logging failure must not change a successful business operation into a failed task, and logs must never expose arbitrary files or containers.

**Verification:** Targeted red-green tests per task, then `go test ./... -count=1`, `go vet ./...`, frontend unit/build/E2E, remote deployment, real SteamCMD output, security checks and `git diff --check`.

---

## Scope Check

Facts: jobs already persist structured `job_events`; pruning deletes task rows; SteamCMD containers are deleted after waiting; the proxy currently rejects container logs; the UI already has bottom-follow behavior for game console output.

Assumptions: JSONL is the persisted format; terminal tasks retain the newest 10 MiB; running logs are not truncated; history pages by sequence; downloads use a stable snapshot; carriage-return progress is rate-limited to four persisted updates per second per source.

Bounded unknown: Docker multiplexed framing differs for TTY and non-TTY streams. Tests must cover the non-TTY maintenance container created by Panel.

Repair track: first commit the current maintenance-container entrypoint repair. Retirement track: `steamcmd exited with code N` remains the summary, but stops being the only diagnostic surface.

## File Ownership Map

- `internal/joblogs/manager.go`: JSONL append/read/subscribe, truncation, recovery and deletion.
- `internal/joblogs/redactor.go`: centralized secret/header redaction.
- `internal/jobs/manager.go`: task lifecycle and Reporter integration.
- `internal/store/job_history.go`: post-commit pruned-job notification.
- `internal/docker/client.go`: maintenance entrypoint and Docker log following.
- `internal/socketproxy/policy.go`, `cmd/socket-proxy/main.go`: labeled maintenance log authorization.
- `internal/httpapi/server.go`: task log history, SSE and download APIs.
- Task owners under `internal/updates`, `internal/content`, `internal/releases`, `internal/automation`: sourced logs.
- `web/src/app/JobLogsPage.tsx`: independent viewer.
- `web/src/app/App.tsx`, `JobsPage.tsx`, `web/src/api/client.ts`, CSS: navigation and client integration.
- `cmd/e2e-fixture/main.go`, `web/e2e/control-panel.spec.ts`: deterministic acceptance flow.

### Task 1: Commit the SteamCMD Entrypoint Repair

**Files:** `internal/docker/client.go`, `internal/docker/client_test.go`

**Why:** Docker `Cmd` does not replace the runtime supervisor `ENTRYPOINT`; the SteamCMD script also requires its absolute image path.

**Impact / Compatibility:** Only maintenance creation changes. Game containers keep the supervisor entrypoint.

**Repair Track:** `createRequest` and `runMaintenance` become the canonical owners of the explicit maintenance entrypoint.

**Retirement Track:** Retire the assumption that `Cmd[0]` replaces image entrypoint. No fallback remains.

**Verification:** `go test ./internal/docker -count=1`

- [ ] Run the targeted entrypoint tests and confirm they protect `/home/steam/steamcmd/steamcmd.sh`.
- [ ] Keep `Entrypoint: command[:1]` and `Cmd: command[1:]` in maintenance create requests.
- [ ] Run the entire Docker module.
- [ ] Commit only these two files as `fix(docker): 修复 SteamCMD 维护容器入口`.

### Task 2: Build the File-Backed Log Manager and Redactor

**Files:** create `internal/joblogs/manager.go`, `manager_test.go`, `redactor.go`, `redactor_test.go`

**Why:** All producers need one sequence, persistence, retention and secret boundary. Logs must survive restart without bloating SQLite.

**Impact / Compatibility:** New data lives under `<data-root>/panel/job-logs`; no existing API changes yet.

**Verification:** `go test ./internal/joblogs -count=1 -race`

- [ ] Write a failing restart test: append seq 1 and 2, reopen, read both, append seq 3.
- [ ] Write failing concurrent append and subscribe-after-history tests proving strict ordering and no replay/live gap.
- [ ] Define `Record{Seq,Timestamp,Source,Level,Message}`, `Query{AfterSeq,BeforeSeq,Limit,Sources,Levels}`, `Page`, and levels `output/info/warn/error`.
- [ ] Implement per-task mutexes, complete-JSON-line persistence before broadcast, bounded subscribers and safe task-ID path ownership.
- [ ] Write a failing terminal-retention test with a 1 KiB test limit: running file remains large; `Finalize` preserves newest complete records and prepends a truncation warning.
- [ ] Implement atomic same-directory rewrite, `Delete`, orphan cleanup and stable download snapshots.
- [ ] Write failing redaction tests for known secrets, `Authorization`, `Cookie`, `*_PASSWORD`, `*_TOKEN` and `*_SECRET`.
- [ ] Implement redaction inside `Append` before serialization; expose no raw-write API; replace values with `[REDACTED]` and process known secrets longest-first.
- [ ] Run race tests and commit as `feat(joblogs): 增加任务日志持久化与脱敏`.

### Task 3: Integrate Logs with Jobs and Record Pruning

**Files:** modify `internal/jobs/manager.go`, `manager_test.go`, `internal/store/job_history.go`, `internal/store/store_test.go`, `cmd/panel/main.go`

**Why:** Every task must automatically log queued, started, progress and terminal states, finalize retention, and delete logs with its task record.

**Impact / Compatibility:** `Reporter.Progress` remains unchanged; add `Reporter.Log(source, level, message)`.

**Repair Track:** `jobs.Manager` becomes the canonical owner of task-to-log lifecycle coupling.

**Retirement Track:** Callers stop constructing duplicate lifecycle lines. Store remains the database transaction owner.

**Verification:** `go test ./internal/jobs ./internal/store ./cmd/panel -count=1`

- [ ] Add a fake log sink and failing tests for queued, started, progress, success, failure and finalize ordering.
- [ ] Add `Reporter.Log` and inject the sink into `jobs.Manager`; logging errors go to diagnostics and never replace operation errors.
- [ ] Add failing Store tests proving prune callbacks receive committed deleted IDs and are not called after rollback.
- [ ] Return deleted IDs after successful prune commit, then call `joblogs.Delete`; log file deletion failure without rolling back task pruning.
- [ ] In `cmd/panel/main.go`, open/recover job logs, provide current secrets to the redactor, wire jobs and cleanup orphan files.
- [ ] When startup recovery interrupts pending/running jobs, append an interruption record and finalize each log.
- [ ] Run tests and commit as `feat(jobs): 关联任务生命周期与持久日志`.

### Task 4: Securely Follow Docker Maintenance Logs

**Files:** modify `internal/socketproxy/policy.go`, tests, `cmd/socket-proxy/main.go`, tests, `internal/docker/client.go`, tests

**Why:** SteamCMD stdout/stderr disappears when the maintenance container is deleted. The proxy must enforce labels internally.

**Impact / Compatibility:** No TCP Docker endpoint. Existing allow-list routes remain unchanged.

**Repair Track:** Consume log EOF before deleting maintenance containers while preserving concise exit-code errors.

**Retirement Track:** Retire wait-then-delete without output consumption.

**Verification:** `go test ./internal/socketproxy ./cmd/socket-proxy ./internal/docker -count=1`

- [ ] Add failing policy/handler tests for `GET /containers/{id}/logs` with fixed stdout, stderr, follow, timestamps and bounded tail parameters.
- [ ] Mock inspect results and prove only `managed=true`, `role=maintenance` containers pass; reject game, unmanaged and unknown containers.
- [ ] Implement proxy-side inspect and label verification before forwarding the stream.
- [ ] Add failing Docker client tests with multiplexed stdout/stderr frames, split lines, partial final lines, cancellation and EOF-before-delete ordering.
- [ ] Implement `FollowLogs(ctx, containerID, emit)` with Docker eight-byte frame decoding and independent stdout/stderr buffers.
- [ ] In `runMaintenance`, start follow after container start, wait for exit, drain logs with a bounded timeout, log capture warnings, delete, then return the SteamCMD exit result.
- [ ] Run tests and commit as `feat(docker): 安全采集 SteamCMD 实时日志`.

### Task 5: Add Sourced Logs Across Task Owners

**Files:** modify `internal/httpapi/server.go` and tests; relevant owners/tests in `internal/updates`, `internal/content`, `internal/releases`, `internal/automation`

**Why:** “尽量所有任务” requires meaningful output outside SteamCMD without manufacturing noisy pseudo-detail.

**Impact / Compatibility:** Existing progress events, percentages and messages stay unchanged. Logs are additive.

**Verification:** `go test ./internal/httpapi ./internal/updates ./internal/content ./internal/releases ./internal/automation -count=1`

- [ ] Add failing fake-Reporter tests for release fetch, package deployment, private apply, reconfigure and game update.
- [ ] Require each path to log start detail, irreversible boundaries and final summary with stable sources such as `release`, `package`, `private-files` and `task`.
- [ ] Do not log request headers, full environments, credentials, credential-bearing URLs or arbitrary command lines.
- [ ] Run existing progress tests to prove `job_events` counts and order did not change unintentionally.
- [ ] Run package tests and commit as `feat(tasks): 记录后台任务来源化执行日志`.

### Task 6: Add Authenticated History, SSE and Download APIs

**Files:** modify `internal/httpapi/server.go`, `internal/httpapi/server_test.go`

**Why:** The browser needs paged history, gap-free live records and safe downloads.

**Impact / Compatibility:** Existing `/api/jobs`, `/api/jobs/{id}` and `/api/jobs/events` remain compatible.

**Verification:** `go test ./internal/httpapi -run 'TestJobLog' -count=1`

- [ ] Add failing history tests for newest page, `after_seq`, `before_seq`, limit validation, filters, empty/legacy/truncated logs, missing jobs and authentication.
- [ ] Register `GET /api/jobs/{id}/logs`, `/logs/stream` and `/logs/download`.
- [ ] Validate job existence before file access and cap each response by record count and encoded bytes.
- [ ] Add failing SSE tests for replay/live boundary, `Last-Event-ID`, exact-once order and slow-subscriber disconnect.
- [ ] Implement subscribe-under-boundary, replay persisted records, then consume live records with SSE ID equal to log sequence.
- [ ] Add download tests for fixed `job-<id>.jsonl` names and stable snapshots during concurrent append.
- [ ] Run tests and commit as `feat(api): 提供任务日志历史流与下载`.

### Task 7: Build the Dedicated React Log Viewer

**Files:** create `web/src/app/JobLogsPage.tsx` and test; modify `JobsPage`, `App`, API client, tests and `web/src/styles/app.css`; reuse `useConsoleFollow`

**Why:** Administrators need a stable surface for long logs: live following, pause on scroll, search, filters, copy and download.

**Impact / Compatibility:** JobsPage accordion and structured events remain available. The log viewer is additive.

**Verification:** `cd web && npm test -- --run src/app/JobLogsPage.test.tsx src/app/JobsPage.test.tsx src/app/App.test.tsx && npm run build`

- [ ] Write failing tests for newest-page load, EventSource append, default follow, scroll-up pause, unread count, resume, older history, search, filters, copy, download, reconnect, legacy empty logs and truncation warning.
- [ ] Add typed `JobLogRecord` and history/blob helpers in the API client.
- [ ] Implement an unframed full-height page with compact task header, search input, source/level menus, follow control, copy and download icon buttons.
- [ ] Reuse or generalize `useConsoleFollow` without creating a second scroll owner; replay must not steal the viewport.
- [ ] Add JobsPage “查看日志” entry and App navigation/back behavior.
- [ ] Add responsive stable dimensions, wrapped long lines and no nested cards or overlapping mobile controls.
- [ ] Run tests/build and commit as `feat(web): 增加独立任务实时日志页面`.

### Task 8: End-to-End and Remote Acceptance

**Files:** modify `cmd/e2e-fixture/main.go`, tests, `web/e2e/control-panel.spec.ts`; create `docs/aegis/work/2026-07-16-task-live-log-stream/50-evidence.md`

**Why:** Unit tests cannot prove persistence, SSE, viewport behavior, downloads and Docker deployment work together.

**Impact / Compatibility:** Extend existing desktop/mobile journeys without weakening lifecycle, private-files, schedules or console assertions.

**Verification:** Full matrix plus deployed-server checks.

- [ ] Add an E2E-only task that emits sourced lines over time, a redacted sentinel, more than one history page and a deterministic failed terminal result.
- [ ] Extend desktop/mobile Playwright to verify live append, scroll pause, unread count, resume, filter, search, copy, download, legacy/truncated states and secret absence.
- [ ] Run `go test ./... -count=1`, `go vet ./...`, frontend unit tests, build and desktop/mobile E2E sequentially.
- [ ] If Windows temp cleanup flakes, record it and require three clean targeted reruns; do not call the full suite clean unless a fresh full run exits 0.
- [ ] Deploy tracked source to 安可服, rebuild `socket-proxy` and `panel`, preserve `/home/steam/l4d2-panel-data`, and restart only Compose services.
- [ ] Verify health, proxy socket numeric ownership/mode, no raw Docker socket in Panel, and only `CAP_NET_RAW` added to proxy.
- [ ] Retry the real instance start and confirm SteamCMD output appears live and remains downloadable after container deletion.
- [ ] Verify a controlled secret is absent from history, SSE, download and disk; trigger successful-job pruning and confirm only pruned-task logs disappear.
- [ ] Record evidence, run `git diff --check`, and commit as `test(joblogs): 验证实时日志完整链路`.

## Rollback Surface

- UI/API can be removed while leaving JSONL files inert.
- Job logging can be disabled without changing task execution, but Docker follow and its proxy route must roll back together.
- Existing `job_events` remain the diagnostic fallback.
- Do not delete retained log files during rollback unless explicitly requested.

## Completion Gate

- Every spec requirement maps to a task above.
- No producer bypasses centralized redaction.
- No proxy route reads non-maintenance container logs.
- Running logs remain untruncated; each terminal log is capped at 10 MiB.
- Pruning deletes logs only after database commit.
- Desktop/mobile main journeys and real SteamCMD output are verified on 安可服.
