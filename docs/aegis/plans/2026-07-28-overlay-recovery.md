# OverlayFS Restart Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore every persisted instance's OverlayFS mount before startup container reconciliation.

**Architecture:** Add a focused provisioning recovery method using the existing shared-state repository and overlay-helper client, then invoke it from Panel startup before lifecycle reconciliation. Overlay-helper remains the privileged mount owner.

**Tech Stack:** Go, SQLite-backed instance repository, Unix-socket overlay-helper API, Docker Compose.

**Baseline / Authority Refs:** `CONTEXT.md`, `docs/aegis/specs/2026-07-28-overlay-recovery-design.md`, `internal/provisioning/service.go`, `cmd/panel/main.go`.

**Compatibility Boundary:** No schema, API, instance layout, desired-state, container-label or data changes.

**Verification:** `go test ./internal/provisioning ./cmd/panel -count=1`, `go test ./... -count=1`, `go vet ./...`, then remote mount/container/port checks.

---

### Task 1: Recover persisted instance overlays

**Files:**
- Modify: `internal/provisioning/service.go`
- Test: `internal/provisioning/service_test.go`

**Why this task exists:** Host/Docker restarts remove kernel mounts while leaving persistent instance state intact.

**Impact / Compatibility:** Recovery uses current `SharedState`, `Instances`, and `Overlay` owners and does not mutate instance records.

**Repair Track:** The missing recovery step is added to provisioning, the owner already responsible for shared-game readiness and overlay setup.

**Retirement Track:** The manual production mount repair retires after the deployed startup recovery passes a Panel restart test.

- [ ] Add a failing test that expects every instance mount to be ensured using the active release.
- [ ] Add failing tests for non-ready state and an instance-specific ensure failure.
- [ ] Implement the smallest `RecoverOverlays` method.
- [ ] Run `go test ./internal/provisioning -count=1`.

### Task 2: Run recovery before container reconciliation

**Files:**
- Modify: `cmd/panel/main.go`

**Why this task exists:** Containers and health checks must not observe empty merged directories during startup.

**Impact / Compatibility:** Existing reconciliation remains unchanged and is merely deferred when overlay recovery fails.

- [ ] Invoke recovery immediately before `life.Reconcile`.
- [ ] Preserve the existing deferred-reconciliation log behavior.
- [ ] Run focused and full Go verification.

### Task 3: Deploy and verify restart recovery

**Files:**
- Deploy the reviewed revision to a new immutable release directory on 琥珀.

**Why this task exists:** Unit tests cannot prove mount propagation through the privileged helper and Docker containers.

**Impact / Compatibility:** Rebuild/recreate only Panel as needed; preserve game containers, `.env.runtime`, shared release and instance data.

- [ ] Deploy with the reachable Go proxy override used on 琥珀.
- [ ] Restart only Panel and verify all seven overlay mounts remain present.
- [ ] Verify all seven supervisors run and UDP ports `27081-27087` listen.
- [ ] Confirm Panel health and absence of new fatal/error logs.
