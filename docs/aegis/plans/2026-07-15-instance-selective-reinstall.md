# Instance Selective Reinstall Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an administrator force-reinstall game files, the instance's currently selected plugin package, or both from one instance update operation.

**Architecture:** Extend `updates.GameCoordinator` into the composite manual-reinstall owner while preserving its existing game-only `Update` entry point. The coordinator will use the existing game updater and package deployment transaction directly so lifecycle stop/start happens at most once, then replay private files once after all selected lower layers complete.

**Tech Stack:** Go 1.24, chi HTTP API, SQLite-backed instance repository, React 19, TypeScript, Vitest/Testing Library, Playwright.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-15-instance-selective-reinstall-design.md`, `CONTEXT.md`, `internal/updates/game.go`, `internal/updates/coordinator.go`, `internal/updates/pipeline.go`, `internal/httpapi/server.go`, `web/src/app/App.tsx`.

**Compatibility Boundary:** Preserve `/api/instances/{id}/game-update`, omitted-field game-only behavior, per-instance Job serialization, existing scheduled game updates, package transaction rollback, private overlay precedence, and independent content repository hot/full actions.

**Verification:** `go test ./internal/updates ./internal/httpapi ./cmd/panel ./cmd/e2e-fixture -count=1`, `go test ./... -count=1`, `npm test -- --run`, `npm run build`, and focused Playwright desktop/mobile instance-update coverage.

---

### Task 1: Composite Reinstall Coordinator

**Files:**
- Modify: `internal/updates/game.go`
- Modify: `internal/updates/game_test.go`

**Why this task exists:** Combined game and package reinstall must be one lifecycle operation, and package reinstall must force a full deployment even when the same package ID is already applied.

**Impact / Compatibility:** `GameCoordinator.Update(ctx,id)` remains game-only. A new options-based method becomes the manual API owner. Existing `Coordinator.ApplyPackage` remains the owner of independent content and scheduled package deployments.

**Repair Track:** Replace manual-update composition through lifecycle-owning coordinators with direct use of the existing game updater and package deployment transaction.

**Retirement Track:** No old public path is removed. The HTTP handler retires its assumption that manual instance update always means game-only.

**Verification:** `go test ./internal/updates -run 'TestGame(Update|Reinstall)' -count=1`

- [ ] Add failing tests for game-only compatibility, package-only forced full deployment, combined ordering with one stop/start, package rollback, missing package, and desired-state preservation.
- [ ] Run the focused tests and confirm failures are caused by the missing selective API.
- [ ] Add `ReinstallOptions`, package lookup/deployer dependencies, and `Reinstall(ctx,id,options)` to `GameCoordinator`; keep `Update` delegating to game-only options.
- [ ] Use `Deployment.Commit`/`Rollback` for package work, replay private content once, and fault from fresh instance state on failure.
- [ ] Run focused and full `internal/updates` tests until green.

### Task 2: Selective HTTP Contract

**Files:**
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`
- Modify: `cmd/panel/main.go`
- Modify: `cmd/e2e-fixture/main.go`

**Why this task exists:** The browser needs one authenticated request that expresses either reinstall component or both and receives one serialized Job.

**Impact / Compatibility:** Requests containing only `confirm:true` remain game-only. Explicit `reinstall_game:false,reinstall_package:false` is rejected. Production and fixture coordinators receive package manager and pipeline dependencies.

**Repair Track:** Decode optional booleans with presence information so omitted fields and explicit false values are distinguishable.

**Retirement Track:** Retire the hard-coded game-only reporter stage only for selective manual requests; scheduled game update reporting remains unchanged.

**Verification:** `go test ./internal/httpapi ./cmd/panel ./cmd/e2e-fixture -run 'Test.*Update' -count=1`

- [ ] Add failing API tests for omitted-field compatibility, three valid selection combinations, and explicit empty selection rejection.
- [ ] Run the focused API tests and confirm the new payload assertions fail.
- [ ] Add pointer boolean request fields, normalize omitted fields to game-only, validate explicit empty selection, and call `GameCoordinator.Reinstall` inside one Job.
- [ ] Wire `Packages` and `Deployer` into production and E2E fixture `GameCoordinator` construction.
- [ ] Run focused HTTP and command-package tests until green.

### Task 3: Instance Update Dialog

**Files:**
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles/app.css`

**Why this task exists:** Administrators need an explicit, ergonomic choice of forced reinstall targets before initiating disruptive work.

**Impact / Compatibility:** The shared confirmation dialog remains unchanged for stop and package actions. A focused reinstall dialog owns checkbox state and prevents an empty submission.

**Verification:** `npm test -- --run src/app/App.test.tsx`

- [ ] Replace the existing game-only UI test with failing tests for default dual selection, game-only, package-only, and disabled empty confirmation.
- [ ] Run the focused Vitest test and confirm failures reflect the absent controls/payload.
- [ ] Add a focused instance reinstall dialog using checkboxes, default both true, submit `{confirm:true,reinstall_game,reinstall_package}`, and disable confirmation when both are false.
- [ ] Add restrained dialog layout styles consistent with existing modal controls.
- [ ] Run focused Vitest tests until green.

### Task 4: Browser Journey And Regression

**Files:**
- Modify: `web/e2e/control-panel.spec.ts`
- Modify: `README.md`
- Create: `docs/aegis/work/2026-07-15-instance-selective-reinstall/50-evidence.md`

**Why this task exists:** The main user journey must prove that a selection produces one background Job through the real browser/API fixture path.

**Impact / Compatibility:** E2E additions use the existing local fixture and do not alter production contracts.

**Verification:** Full Go, Vitest, build, and desktop/mobile Playwright commands listed in the plan header.

- [ ] Add a Playwright assertion that opens an instance update dialog, submits a selective reinstall, and observes one Job.
- [ ] Update README manual-update behavior to describe forced selectable reinstall.
- [ ] Run `gofmt` on changed Go files and `git diff --check`.
- [ ] Run focused tests, then the complete Go, Vitest, build, desktop, and mobile verification suite.
- [ ] Record exact commands, outcomes, and residual risks in the evidence file.
