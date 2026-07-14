# L4D2 Control Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a deployable single-admin control panel for managing multiple persistent L4D2 containers on one Linux Docker host.

**Architecture:** A Go HTTP service owns SQLite state, restricted Docker access, jobs, content deployment, A2S and console proxying. A React SPA consumes JSON APIs plus SSE/WebSocket streams. Runtime and deployment assets isolate SRCDS behind a fixed supervisor interface and a Docker socket proxy.

**Tech Stack:** Go 1.24, chi, modernc SQLite, gorilla/websocket, cron/v3, React 19, TypeScript, Vite, Vitest, Playwright, Docker Compose.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`.

**Compatibility Boundary:** Keep host networking, persistent `/srv/l4d2-panel` data, fixed supervisor exec operations, managed-container labels, content precedence `package < shared VPK < private`, and no arbitrary shell/RCON/firewall control.

**Verification:** `go test ./...`, `go vet ./...`, `npm test -- --run`, `npm run build`, `docker compose config`, plus Linux Docker smoke steps documented in the README.

---

### Task 1: Backend foundation and persistence

**Files:** Create `go.mod`, `cmd/panel/main.go`, `internal/config/config.go`, `internal/store/store.go`, `internal/store/migrations.go`, `internal/domain/models.go`; test `internal/config/config_test.go`, `internal/store/store_test.go`.

**Why this task exists:** Establish the durable source of truth and safe configuration defaults for every later workflow.

**Impact / Compatibility:** SQLite uses WAL and preserves `node_id`; data paths remain rooted under the configured data directory.

**Verification:** `go test ./internal/config ./internal/store`.

- [ ] Write failing tests for environment defaults, directory creation, WAL mode, migrations, and instance CRUD.
- [ ] Run the tests and confirm failures are caused by missing implementation.
- [ ] Implement the minimum config, schema, models, and store behavior.
- [ ] Re-run the target tests and commit the green slice.

### Task 2: Authentication and HTTP contract

**Files:** Create `internal/auth/service.go`, `internal/httpapi/server.go`, `internal/httpapi/auth.go`, `internal/httpapi/instances.go`, `internal/httpapi/jobs.go`; test corresponding `_test.go` files.

**Why this task exists:** Protect the single-admin panel and expose stable instance/job contracts to the SPA.

**Impact / Compatibility:** Argon2id password hashes and strict cookies; mutating routes require authenticated same-origin sessions and structured errors.

**Verification:** `go test ./internal/auth ./internal/httpapi`.

- [ ] Write failing handler tests for bootstrap, login/logout/session, instance CRUD, validation, confirmations, and job retrieval.
- [ ] Verify RED, implement the routes and middleware, then verify GREEN.
- [ ] Commit the authenticated API slice.

### Task 3: Container lifecycle, ports, and jobs

**Files:** Create `internal/docker/client.go`, `internal/docker/lifecycle.go`, `internal/ports/checker.go`, `internal/jobs/manager.go`, `internal/jobs/pipeline.go`; add unit tests with fake Docker and listener probes.

**Why this task exists:** Create/install/start/stop/rebuild instances without exposing arbitrary Docker operations or deleting persistent data.

**Impact / Compatibility:** Only correctly labeled containers are touched; host port collisions reject starts; fixed supervisor commands are the only exec surface.

**Verification:** `go test ./internal/docker ./internal/ports ./internal/jobs`.

- [ ] Write failing tests for labels, mounts, host mode, collision rejection, stop escalation, serialized mutations, progress persistence, and restart recovery.
- [ ] Verify RED, implement minimal interfaces/pipelines, verify GREEN, and commit.

### Task 4: Safe content and update pipelines

**Files:** Create `internal/safepath/safepath.go`, `internal/archive/inspect.go`, `internal/content/manifest.go`, `internal/content/overlay.go`, `internal/content/uploads.go`, `internal/updates/pipeline.go`; test malicious archives, ownership, precedence, resumable chunks, rollback.

**Why this task exists:** Safely deploy packages, VPKs, and private files while preserving ownership and rollback semantics.

**Impact / Compatibility:** Reject traversal, links and archive bombs; never delete Valve/shared/private owners; hot updates use the approved path allowlist.

**Verification:** `go test ./internal/safepath ./internal/archive ./internal/content ./internal/updates`.

- [ ] Write and run failing safety and pipeline tests.
- [ ] Implement safe joining, inspection, manifests, atomic files, overlays, backup/restore, and update stages.
- [ ] Run target and regression tests; commit.

### Task 5: Console, A2S, players, scheduler, and audit

**Files:** Create `internal/console/hub.go`, `internal/a2s/client.go`, `internal/players/service.go`, `internal/scheduler/scheduler.go`, `internal/audit/service.go`, plus API route files and tests.

**Why this task exists:** Complete native operations, online player actions, scheduling, and traceability.

**Impact / Compatibility:** Console attaches only through `l4d2-supervisor attach`; player names are never interpolated into commands; missed cron runs are skipped.

**Verification:** `go test ./internal/console ./internal/a2s ./internal/players ./internal/scheduler ./internal/audit ./internal/httpapi`.

- [ ] Write failing protocol, mapping, command, cron, serialization and audit tests.
- [ ] Verify RED, implement services/routes, verify GREEN, and commit.

### Task 6: React administration interface

**Files:** Create `web/package.json`, Vite/TypeScript config, `web/src/api`, `web/src/app`, `web/src/components`, `web/src/pages`, `web/src/styles`, and Vitest tests.

**Why this task exists:** Give the administrator a clear main journey from login through instance control, console, content, players, jobs and scheduling.

**Impact / Compatibility:** Responsive keyboard-accessible UI; destructive operations require confirmation; long jobs survive refresh through server state and SSE.

**Verification:** `cd web && npm test -- --run && npm run build`.

- [ ] Write failing component and state tests for login, dashboard, instance actions, job progress, content forms and scheduling.
- [ ] Verify RED; implement the industrial control-room design system and functional routes.
- [ ] Verify GREEN and production build; commit.

### Task 7: Runtime images, deployment, docs, and cross-layer verification

**Files:** Create `Dockerfile`, `docker-compose.yml`, `deploy/socket-proxy.env`, `runtime/Dockerfile`, `runtime/supervisor/*`, `.env.example`, `README.md`, `Makefile`; update evidence records.

**Why this task exists:** Turn the application into a reproducible single-host deployment with explicit security boundaries and operator recovery steps.

**Impact / Compatibility:** Panel never mounts the raw socket; game containers remain unprivileged with persistent bind mounts and host networking.

**Verification:** `go test ./...`, `go vet ./...`, `cd web && npm test -- --run && npm run build`, `docker compose config`.

- [ ] Add failing supervisor contract tests before its implementation.
- [ ] Implement images, health checks, socket-proxy allowlist, compose wiring and operator documentation.
- [ ] Run the complete verification bundle and record Linux-only residual checks.
- [ ] Commit the release-ready slice.

### Task 8: Repair browser contracts and destructive-operation safety

**Files:** Modify `internal/domain/models.go`, `internal/httpapi/server_test.go`, `web/src/api/client.ts`, `web/src/app/App.tsx`, `web/src/app/App.test.tsx`, `web/index.html`; test the same Go/React packages.

**Why this task exists:** The real browser currently sends schedule field names rejected by strict Go decoding, bypasses required confirmations, and cannot upload a VPK larger than the backend chunk limit.

**Impact / Compatibility:** Keep existing endpoint paths and response fields. Make `ScheduledTask` JSON explicit, preserve confirmation status codes, upload VPK in bounded 8 MiB chunks, surface API errors, and treat `interrupted` as a terminal Job state.

**Repair Track:** Canonical contract ownership moves to explicit Go JSON tags plus matching TypeScript request types. React owns confirmation UI and chunk iteration; the server remains the enforcement boundary.

**Retirement Track:** Retire snake_case requests against untagged structs, direct `confirm: true` calls without user confirmation, one-shot VPK PATCH, swallowed Cron/player errors, and unconditional health claims.

**Verification:** `go test ./internal/httpapi`; `cd web && npm test -- --run && npm run build`.

- [x] Add failing Go tests that decode the public snake_case schedule contract and reject unknown fields.
- [x] Add failing React tests for Cron success/error, update/player confirmation, interrupted Jobs, health error state, and multi-chunk VPK offsets.
- [x] Run target tests and confirm each failure represents the audited production behavior.
- [x] Add explicit `ScheduledTask` JSON tags and minimal React state/confirmation/chunk helpers.
- [x] Re-run Go/React targets, build the SPA, and commit the green slice.

### Task 9: Declare and reserve SourceTV and plugin ports

**Files:** Modify `internal/domain/models.go`, `internal/store/migrations.go`, `internal/store/store.go`, `internal/ports/checker.go`, `internal/lifecycle/service.go`, `internal/docker/lifecycle.go`, `runtime/supervisor.py`, `internal/httpapi/server.go`, `web/src/api/client.ts`, `web/src/app/App.tsx`; add focused tests in the corresponding packages.

**Why this task exists:** `SourceTVPort` is persisted but never accepted, checked or passed to SRCDS; plugin ports are absent; the production configured-port provider currently returns no reservations.

**Impact / Compatibility:** Add an additive `instance_plugin_ports` table. Keep game-port behavior and host networking; exclude the current instance from its own reservation check and reject duplicates across game, SourceTV and plugin declarations before start.

**Repair Track:** `ports.Checker` becomes the sole configured/listening conflict owner. Instance API and UI expose `sourcetv_port` and `plugin_ports`; runtime enables SourceTV only when its port is nonzero.

**Retirement Track:** Retire `Configured: func() []int { return nil }`, single-port lifecycle checks, and the dormant SourceTV field. Extra SRCDS args remain for unrelated advanced flags, not as a second port owner.

**Verification:** `go test ./internal/store ./internal/ports ./internal/lifecycle ./internal/docker ./internal/httpapi ./runtime`; `cd web && npm test -- --run && npm run build`.

- [x] Write failing migration/CRUD tests for plugin-port round trips on new and reopened databases.
- [x] Write failing reservation tests for cross-kind conflicts, self exclusion and host listeners.
- [x] Write failing Docker/runtime/API/React tests for SourceTV and plugin-port propagation.
- [x] Implement the additive data contract and multi-port checker, then run all target tests.
- [x] Commit the port-management slice.

### Task 10: Persist update transactions and drain Panel shutdown

**Files:** Modify `internal/updates/pipeline.go`, `internal/updates/coordinator.go`, `internal/jobs/manager.go`, `cmd/panel/main.go`; add journal helpers and tests under `internal/updates` and `internal/jobs`.

**Why this task exists:** A process exit during package deployment can leave a mixed game tree, and Panel currently exits through `log.Fatal` without HTTP shutdown or Job drain.

**Impact / Compatibility:** Journal files live below each instance backup root and record affected paths, prior existence, backup root and stage before mutation. Startup rolls back uncommitted transactions. Existing Job JSON/SSE fields and package precedence remain stable.

**Repair Track:** Durable journal data replaces the in-memory-only rollback set. `http.Server.Shutdown`, scheduler stop and `jobs.Manager.Wait` provide a bounded graceful path.

**Retirement Track:** Retire success-time backup deletion before full-update health confirmation, blanket reliance on `RecoverJobs()` as recovery, ignored initial Job persistence failures, and `log.Fatal(server.ListenAndServe())`.

**Verification:** `go test ./internal/updates ./internal/jobs ./cmd/panel`; targeted process-restart fixture; `go vet ./...`.

- [x] Add failing tests for recovering a partially applied transaction and full-update health failure rollback.
- [x] Add failing tests for initial Job persistence failure and bounded Job drain.
- [x] Implement atomic journal writes, startup recovery, commit/rollback ownership and Panel signal shutdown.
- [x] Re-run targets and a disposable Linux Panel SIGTERM smoke, then commit.

### Task 11: Recover maintenance writers and interrupted artifacts

**Files:** Modify `internal/docker/client.go`, `internal/lifecycle/service.go`, `internal/updates/game.go`, `internal/content/uploads.go`, `internal/releases/github.go`, `internal/maintenance/manager.go`; add regression tests in each package.

**Why this task exists:** Orphaned SteamCMD maintenance containers can continue writing after Panel restart, VPK part/meta state can diverge, and incomplete backups can look complete.

**Impact / Compatibility:** Preserve managed labels, fixed maintenance commands, Valve no-downgrade policy, upload IDs/offsets/SHA-256, and GitHub Release sources.

**Repair Track:** Reconciliation lists maintenance roles separately and blocks conflicting instance mutations until the writer exits or is classified. File size is the recoverable VPK offset authority; backups publish by atomic rename.

**Retirement Track:** Retire game-only managed-container scans, JSON-offset-only recovery, direct final backup writes, stale instance fault writes and non-idempotent game-update retry assumptions.

**Verification:** `go test ./internal/docker ./internal/lifecycle ./internal/updates ./internal/content ./internal/releases ./internal/maintenance ./runtime`.

- [x] Add failing maintenance adoption, desired-running recovery and idempotent game-update retry tests.
- [x] Add failing VPK append/metadata crash and partial-backup publication tests.
- [x] Implement the smallest recovery controller and atomic artifact rules.
- [x] Run package regressions and disposable Linux interruption smokes, then commit.

### Task 12: Real-browser and fault-injection acceptance

**Files:** Create `web/playwright.config.ts`, `web/e2e/*.spec.ts`, an `e2e`-tagged Go fixture command, and test scripts in `web/package.json`; update `README.md` and evidence records.

**Why this task exists:** Vitest cannot prove strict API contracts, Secure cookies, SSE, WebSocket, refresh recovery or responsive keyboard journeys.

**Impact / Compatibility:** The fixture uses real HTTP, SQLite, auth, content, Job/SSE and WebSocket routes while replacing Docker, SRCDS, A2S, Steam and GitHub boundaries with deterministic fakes. Production binaries do not include fixture code.

**Verification:** `cd web && npm test -- --run && npm run build && npm run e2e`; complete Go tests; Linux Docker smoke matrix recorded in evidence.

- [ ] Install pinned Playwright test tooling and add a Chromium webServer configuration.
- [ ] Build a per-worker temporary Go fixture with deterministic success, failure, slow and interrupted Jobs.
- [ ] Cover login/create/refresh/Job recovery, console reconnect, player confirmations, multi-chunk VPK/private/package flows and Cron persistence.
- [ ] Run desktop and 390x844 mobile keyboard journeys; verify dialog focus and visible error states.
- [ ] Run safe Panel/VPK/package/SRCDS interruption smokes on `sirphomesv`; document Docker-daemon restart and ENOSPC as isolated-host-only if not executed.
- [ ] Run the complete release bundle and request final specification/code review.
