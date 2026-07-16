# Restart and Unix Socket Console Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore reliable instance restart and browser console attach after the Docker proxy moved to a shared Unix Socket.

**Architecture:** Keep `internal/docker.Engine` as the single owner of Docker transport and response semantics. Store the direct hijack dialer selected by `NewEngine`, use it for supervisor attach, and expose Docker response status through an internal error type so Stop alone can accept the documented already-stopped response.

**Tech Stack:** Go 1.25, `net/http`, Unix domain sockets, Docker Engine API v1.44, SQLite-backed lifecycle tests.

**Baseline / Authority Refs:** `CONTEXT.md`, `docker-compose.yml`, `runtime/supervisor.py`, `internal/docker/client.go`, `internal/lifecycle/service.go`, and `10-baseline-readset.md` in this task directory.

**Compatibility Boundary:** Preserve TCP proxy attach, all existing Docker endpoint paths and error text, lifecycle/API JSON contracts, and the supervisor protocol. Do not accept 304 for operations other than idempotent container stop.

**Verification:** Targeted RED/GREEN tests in `internal/docker` and `internal/lifecycle`, related `internal/httpapi` regression, `go test ./... -count=1`, frontend unit tests/build, and desktop/mobile Playwright journeys.

---

### Task 1: Route supervisor attach through the selected Docker transport

**Files:**
- Modify: `internal/docker/client.go`
- Test: `internal/docker/client_test.go`

**Why this task exists:** The production engine uses a Unix Socket, but console attach currently TCP-dials the synthetic HTTP hostname and cannot connect.

**Impact / Compatibility:** The engine transport is the canonical owner. TCP keeps its current direct dial behavior; Unix attach must use the configured socket. The socket proxy and supervisor protocol remain unchanged.

**Repair Track:** Add one direct dial function to `Engine`, initialize it from the configured Docker host, and use it for hijacked exec streams.

**Retirement Track:** Retire URL parsing and TCP dialing from `AttachSupervisor`; no fallback remains because transport selection belongs to `NewEngine`.

**Verification:** `go test ./internal/docker -run 'TestUnixEngineAttachSupervisorUsesSocketTransport|TestAttachSupervisorHijacksFixedExecStream' -count=1`

- [x] Add `TestUnixEngineAttachSupervisorUsesSocketTransport` using an `httptest` server bound to a Unix listener; create the exec, hijack it, send `status\n`, and require `echo:status\n`.
- [x] Run the target test and confirm it fails because the current attach path attempts TCP resolution of `docker`.
- [x] Add a direct dial function to `Engine`; select Unix or TCP in `NewEngine`; replace the attach-local URL parsing/dialing with that function.
- [x] Run both Unix and existing TCP attach tests and confirm they pass.

### Task 2: Make restart tolerate supervisor-first container exit

**Files:**
- Modify: `internal/docker/client.go`
- Test: `internal/docker/client_test.go`
- Test: `internal/lifecycle/service_test.go`

**Why this task exists:** The supervisor may terminate PID 1 before Docker processes `/stop`; Docker then reports 304, which currently faults the instance and prevents the Start half of Restart.

**Impact / Compatibility:** Only container Stop gains idempotent 304 handling. Authentication, proxy, network, timeout, 4xx, and 5xx failures remain errors.

**Repair Track:** Preserve Docker status codes in an internal API error and let `Engine.Stop` accept only 304. Add a lifecycle-level restart test that proves Start is reached and actual/desired state become running.

**Retirement Track:** Retire the accidental interpretation of already-stopped as failure. No retry or state-specific fallback is added.

**Verification:** `go test ./internal/docker ./internal/lifecycle -run 'TestStop|TestRestart' -count=1`

- [x] Add `TestStopTreatsAlreadyStoppedContainerAsSuccess` and a non-304 error assertion in `internal/docker/client_test.go`.
- [x] Add `TestRestartContinuesWhenSupervisorAlreadyStoppedContainer` with a fake Docker HTTP server returning 304 from stop and 204 from start.
- [x] Run both tests and confirm they fail with the current generic non-2xx error.
- [x] Add an internal status-carrying Docker API error; update `Engine.Stop` to accept only 304.
- [x] Run Docker and lifecycle targets and confirm they pass.

### Task 3: Regression and user journey verification

**Files:**
- Modify: `docs/aegis/work/2026-07-16-restart-console-unix-socket/50-evidence.md`
- Test: `web/e2e/control-panel.spec.ts`

**Why this task exists:** Restart and console cross Docker, lifecycle, API, and browser boundaries.

**Impact / Compatibility:** Verification must show that the repair does not widen the socket proxy policy or break existing TCP fixtures.

**Repair Track:** Run targeted, related, full, build, and desktop/mobile E2E checks and record exact results.

**Retirement Track:** No temporary instrumentation or test-only production hooks may remain.

**Verification drift:** Playwright exposed an existing strict-locator race because import success and reload progress can both have `role=status`. Narrow only that assertion to the exact success text; do not alter product status behavior.

**Verification:** `go test ./internal/docker ./internal/lifecycle ./internal/httpapi -count=1`; `go test ./... -count=1`; `npm test -- --run`; `npm run build`; `npm run e2e -- --project=desktop`; `npm run e2e -- --project=mobile`.

- [x] Run targeted and related Go tests.
- [x] Run the full Go and frontend unit suites.
- [x] Run the production frontend build.
- [x] Run desktop and mobile E2E sequentially.
- [x] If the ZIP import status locator matches concurrent live regions, prove the imported tree and diff are correct, then scope the assertion to the exact success text.
- [x] Confirm the worktree is clean except intentional repair files and record residual runtime risk.
