# Aria2 Release Download Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Download GitHub Release assets through aria2 with eight configurable connections.

**Architecture:** Keep GitHub/package orchestration in Go and inject a focused aria2 transfer owner. Persist the connection setting in `system_settings` and expose it in the existing settings UI.

**Tech Stack:** Go, SQLite, aria2c, React/TypeScript, Vitest

**Baseline / Authority Refs:** `CONTEXT.md`; `docs/aegis/specs/2026-08-01-aria2-release-download-design.md`; existing `internal/releases`, `internal/content`, and task-log contracts

**Compatibility Boundary:** Existing Release routes, source selection, package identity, validation, progress visibility, proxy environment, and deployment semantics remain stable.

**Verification:** `go test ./internal/store ./internal/httpapi ./internal/releases`; `go test ./...`; frontend targeted tests and build; deployment tests.

---

### Task 1: Persistent Download Setting

**Files:** Modify `internal/store/job_history.go`, `internal/store/job_history_test.go`, `internal/httpapi/server.go`, and `internal/httpapi/server_test.go`.

**Why:** Administrators need a validated value that defaults to eight and affects the next transfer.

**Impact / Compatibility:** Reuse `system_settings`; no schema migration or existing key changes.

- [ ] Add failing store tests for default, persistence, and 1-16 validation.
- [ ] Run targeted tests and confirm the missing API failure.
- [ ] Add `ReleaseDownloadConnections` getters/setters and constants.
- [ ] Add failing HTTP GET/PUT tests with strict JSON validation.
- [ ] Implement `/api/settings/downloads`; rerun store and HTTP tests.

### Task 2: Aria2 Transfer Owner

**Files:** Create `internal/releases/downloader.go`, `internal/releases/aria2.go`, and `internal/releases/aria2_test.go`.

**Why:** Delegate splitting, retries, resume, and Range fallback to a mature tool while retaining cancellation, limits, and observability.

**Impact / Compatibility:** Preserve destination-file and byte-count contract. Retire only direct `io.Copy` for Release asset bodies.

- [ ] Add failing tests using a fake executable for safe arguments, eight connections, dynamic setting, cancellation, failure, and size enforcement.
- [ ] Run the tests and confirm missing downloader failures.
- [ ] Implement the minimal command runner and file-size progress monitor without parsing console output.
- [ ] Rerun downloader tests and related job progress tests.

### Task 3: GitHub Release Integration

**Files:** Modify `internal/releases/github.go`, `internal/releases/github_test.go`, `internal/releases/synchronizer.go`, `internal/automation/dispatcher.go`, `internal/httpapi/server.go`, and `cmd/panel/main.go` as required for one shared configured client.

**Why:** All manual, source, instance, and scheduled Release paths must use aria2 and current settings.

**Impact / Compatibility:** Keep Release selection/reuse/package publication unchanged; never pass the GitHub token to aria2.

- [ ] Add failing integration tests for signed redirect exchange, untrusted redirect rejection, downloader invocation, reuse, and token isolation.
- [ ] Run Release tests and confirm failures are caused by the old direct stream.
- [ ] Inject the downloader/settings reader and replace direct asset copying.
- [ ] Preserve public downloads and authenticated private downloads through trusted URL resolution.
- [ ] Rerun Release, automation, HTTP, and update tests.

### Task 4: System Settings UI

**Files:** Modify `web/src/app/App.tsx`, `web/src/app/App.test.tsx`, and styles only if existing setting styles are insufficient.

**Why:** Administrators need to view and change the connection count from the panel.

**Impact / Compatibility:** Add one compact settings card/control; existing cards and save locks remain unchanged.

- [ ] Add failing UI tests for loading default/current value, validation, and PUT payload.
- [ ] Run the targeted Vitest test and confirm the control is absent.
- [ ] Implement the numeric 1-16 setting and immediate next-download copy.
- [ ] Rerun targeted frontend tests and build.

### Task 5: Runtime And Regression Verification

**Files:** Modify `Dockerfile`, `deployment_test.go`, and evidence records.

**Why:** The production runtime must contain the required mature downloader.

**Impact / Compatibility:** Image grows by the Alpine aria2 package; the Go binary remains static.

- [ ] Add a failing deployment assertion for runtime `aria2` installation.
- [ ] Install aria2 in the final image and pass the assertion.
- [ ] Run `gofmt`, full Go tests, frontend tests/build, and inspect `git diff --check`.
- [ ] Record commands, results, residual risks, and retirement evidence.

