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

