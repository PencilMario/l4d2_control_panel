# Manual AI Crash Analysis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:executing-plans to implement this plan task-by-task.

**Goal:** Prevent every Accelerator upload from creating an AI report while keeping AI generation available from the explicit manual analysis action.

**Architecture:** Keep the production `EnqueueAnalysis` callback as the upload-side Stackwalk entry point, but call the shared analysis Worker with `requestAI=false`. Manual API requests continue to call the same Worker with `requestAI=true`, so only the explicit action generates AI output. Existing instance auto-analysis storage remains for compatibility but is no longer an upload trigger.

**Tech Stack:** Go, SQLite-backed crash report manager, persistent Jobs, React/Vitest frontend tests, Docker Compose deployment.

**Baseline / Authority Refs:** `docs/aegis/work/2026-08-13-crash-diagnostics-rows/00-intent.md`, `internal/crashreports/config.go`, `internal/crashreports/manager.go`, `internal/crashanalysis/worker.go`, `internal/httpapi/accelerator.go`, `CONTEXT.md`.

**Compatibility Boundary:** Preserve Accelerator submit/pre-submit protocol, automatic Stackwalk behavior, stored `AutoCrashAnalysis` fields, manual `/api/crash-reports/{id}/analyze` request/response, `crash_analysis` Job type, and AI persistence semantics.

**Verification:** Focused Go upload and HTTP API tests, full Go tests/vet, frontend tests/typecheck/build, `git diff --check`, remote health/API checks, and Playwright confirmation that upload does not create a task while clicking “重新分析” does.

---

### Task 1: Lock the upload/manual trigger boundary with tests

**Files:**
- Modify: `internal/crashreports/manager_test.go`
- Modify: `internal/httpapi/accelerator_test.go`
- Modify: `web/src/app/CrashReportsPage.test.tsx`

**Why this task exists:** The user needs upload to be cheap and deterministic while retaining an explicit AI action. Tests must distinguish the upload path from the manual path.

**Impact / Compatibility:** Tests must prove an upload with an instance configured for old automatic analysis does not invoke an enqueue callback or create a `crash_analysis` Job, while the existing manual API still submits `requestAI=true` and the frontend still calls the API only after button activation.

**Verification:** Run the focused Go and Vitest tests and observe the new upload test fail before production code changes.

**Repair Track:** Add the smallest failing assertion at the canonical upload callback boundary and preserve the existing manual queue assertions.

**Retirement Track:** Retire only the test expectation that upload honors `AutoCrashAnalysis`; retain coverage for the field's storage and manual analysis.

- [ ] **Step 1: Add an upload callback regression test**

  Configure `crashreports.New` with an `EnqueueAnalysis` callback that records calls, upload a valid minidump and metadata, and assert the callback is not invoked after the production wiring is changed. The test must first be written with the current callback contract so it fails against current behavior when the configured production callback is exercised.

- [ ] **Step 2: Add the API/manual analysis assertion**

  Keep the existing `POST /api/crash-reports/{id}/analyze` assertions and add an explicit assertion that the queued request has `requestAI == true`; this guards the retained manual path.

- [ ] **Step 3: Add the frontend explicit-action assertion**

  Assert the initial report render does not call an `/analyze` endpoint, then click `重新分析` and assert the existing `POST` body is `{"ai":true}`.

- [ ] **Step 4: Run the focused tests**

  Run `go test ./internal/crashreports ./internal/httpapi` and `npm test -- --run src/app/CrashReportsPage.test.tsx` from `web`; the new upload test must fail for the current automatic wiring, while unrelated baseline tests should remain green.

### Task 2: Remove automatic AI request from upload wiring

**Files:**
- Modify: `cmd/panel/main.go`
- Modify: `internal/crashreports/manager_test.go` if the test needs a narrow fixture adjustment

**Why this task exists:** `cmd/panel/main.go` is the production owner that translates `AutoCrashAnalysis` into an upload-side `analysisWorker.Enqueue(..., true)` call.

**Impact / Compatibility:** Keep the upload enqueue callback so automatic Stackwalk remains intact, but force its Worker request to `false`. Manual HTTP analysis still receives the Worker with `true`.

**Verification:** Focused tests pass, and a source-level search confirms no upload initialization path calls `analysisWorker.Enqueue` based on `AutoCrashAnalysis`.

**Repair Track:** Remove the root cause in the production composition layer rather than adding a guard inside the Worker: the upload composition must request Stackwalk-only work, while manual callers retain their existing semantics.

**Retirement Track:** The upload-side `AutoCrashAnalysis` conditional and `requestAI=true` behavior are retired. The Stackwalk callback, field, and persistence contract stay until separately approved changes remove them.

- [ ] **Step 1: Change the upload callback to Stackwalk-only**

  Keep the `EnqueueAnalysis` function literal in `cmd/panel/main.go`, remove its database lookup and `AutoCrashAnalysis` gate, and call a small helper that invokes `analysisWorker.Enqueue(ctx, report.ID, false)`. Keep the callback's existing no-instance behavior and error logging.

- [ ] **Step 2: Run focused Go tests**

  Run `go test ./internal/crashreports ./internal/httpapi ./internal/crashanalysis`; confirm upload and manual analysis tests pass.

- [ ] **Step 3: Run the full verification suite**

  Run `go test ./...`, `go vet ./...`, `npm test -- --run`, `npx tsc --noEmit -p tsconfig.json`, `npm run build`, and `git diff --check`.

### Task 3: Document, commit, deploy, and verify the user journey

**Files:**
- Create: `docs/aegis/work/2026-08-13-crash-manual-ai/50-evidence.md`
- Modify: `docs/aegis/work/2026-08-13-crash-manual-ai/20-checkpoint.md`

**Why this task exists:** This is a user-visible behavior and a production trigger change; local tests alone do not prove the running Panel no longer creates AI work on upload.

**Impact / Compatibility:** Deploy only the Panel service, preserve remote `.env` and crash data, and keep a rollback copy of the currently running source until acceptance completes.

**Verification:** Build and deploy the isolated commit, check `/api/health`, inspect Panel logs and jobs, then use Playwright at `http://100.73.249.118:18081/` to confirm the explicit button remains available and no automatic analyze request is emitted during report selection/refresh.

- [ ] **Step 1: Commit implementation and evidence**

  Commit the behavior and tests with `fix(crash): 仅手动触发 AI 崩溃分析`; do not stage the main worktree's `deployment_test.go`.

- [ ] **Step 2: Build and deploy the Panel image**

  Stage a Git archive to a remote temporary directory, preserve `.env`, build the Panel image, switch the source directory with a rollback copy, and run `docker compose -p l4d2_control_panel --env-file .env up -d --no-deps --force-recreate panel`.

- [ ] **Step 3: Verify health and manual boundary**

  Check Panel health, `GET /api/health`, the report list/detail request set, and the manual analyze POST. Confirm no automatic `/analyze` request occurs before the explicit button click.

- [ ] **Step 4: Record evidence and final state**

  Write exact commands/results and residual risks to `50-evidence.md`, update `20-checkpoint.md`, run final `git status`, and retain only the rollback source copy remotely.
