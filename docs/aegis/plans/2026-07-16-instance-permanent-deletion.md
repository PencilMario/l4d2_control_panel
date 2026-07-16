# Instance Permanent Deletion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a typed-confirmation delete action to instance cards that permanently removes the instance record, managed container, and instance data directory.

**Architecture:** Reuse the existing `DELETE /api/instances/{id}` lifecycle job with a fixed `{confirm:true, delete_data:true}` body. Keep confirmation and pending state in the Overview UI, wait for the returned job through the existing queue-and-wait path, then refresh instances only after success.

**Tech Stack:** React, TypeScript, Vitest, Testing Library, Playwright, Go HTTP/lifecycle APIs.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-16-instance-permanent-deletion-design.md`, `internal/httpapi/server.go`, `internal/lifecycle/service.go`, `web/src/app/App.tsx`.

**Compatibility Boundary:** Do not change the delete API or lifecycle cleanup order. Existing start, stop, update, console, player, configuration, task logging, and mobile navigation behavior must remain stable.

**Verification:** `npm test -- --run`, `npm run build`, `npx playwright test e2e/control-panel.spec.ts`, `go test ./internal/lifecycle ./internal/httpapi`.

---

### Task 1: Typed Permanent-Delete Confirmation

**Files:**
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/styles/app.css`

**Why this task exists:** Protect irreversible deletion with explicit instance-name confirmation and clear consequences.

**Impact / Compatibility:** Only Overview instance-card UI state changes. The backend remains the canonical cleanup owner.

**Verification:** `npm test -- --run src/app/App.test.tsx`

- [ ] **Step 1: Write the failing component test**

Render an instance, click `删除实例`, assert the dialog lists container and data deletion, assert `永久删除` is disabled until the exact instance name is entered, then assert the request is `DELETE /api/instances/{id}` with `{confirm:true, delete_data:true}`.

- [ ] **Step 2: Run the component test and verify RED**

Run `npm test -- --run src/app/App.test.tsx`; expect failure because no delete control or dialog exists.

- [ ] **Step 3: Implement the minimal UI**

Add `deleting` and `deleteName` state to Overview, a trash icon button in the instance action group, and a project-styled confirmation dialog. Disable confirmation unless `deleteName === deleting.name` or a deletion is pending.

- [ ] **Step 4: Submit through the existing task path**

Call the existing queue-and-wait helper with method `DELETE` and body `{confirm:true, delete_data:true}`. On success close the dialog and reload instances; on failure keep the dialog open and surface the existing error.

- [ ] **Step 5: Run the component test and verify GREEN**

Run `npm test -- --run src/app/App.test.tsx`; expect all App tests to pass.

### Task 2: Responsive Destructive UI

**Files:**
- Modify: `web/src/styles/app.css`
- Modify: `web/e2e/control-panel.spec.ts`

**Why this task exists:** Ensure the destructive action remains legible, keyboard usable, and non-overflowing on desktop and mobile.

**Impact / Compatibility:** Reuse the existing modal and danger palette; do not introduce a second modal system.

**Verification:** `npx playwright test e2e/control-panel.spec.ts`

- [ ] **Step 1: Extend the Playwright journey and verify RED**

Open the delete dialog for a fixture instance, verify the disabled state, enter the exact name, confirm deletion, and assert the instance disappears. Run desktop first and expect the missing control to fail.

- [ ] **Step 2: Add scoped styles**

Add styles for consequence text, confirmation input, danger button, focus state, and narrow viewport wrapping without changing shared card dimensions.

- [ ] **Step 3: Run desktop and mobile Playwright and verify GREEN**

Run `npx playwright test e2e/control-panel.spec.ts`; expect both projects to pass without horizontal overflow.

### Task 3: Regression, Evidence, and Integration

**Files:**
- Create: `docs/aegis/work/2026-07-16-instance-permanent-deletion/50-evidence.md`

**Why this task exists:** Prove the full UI request and existing backend deletion contract remain correct before integration.

**Impact / Compatibility:** No production behavior beyond Tasks 1-2.

**Verification:** Full commands below.

- [ ] **Step 1: Run frontend regression and build**

Run `npm test -- --run` and `npm run build`; expect zero failures and a successful production bundle.

- [ ] **Step 2: Run backend contract regression**

Run `go test ./internal/lifecycle ./internal/httpapi`; expect both packages to pass.

- [ ] **Step 3: Record evidence**

Record RED/GREEN results, test counts, build result, Playwright desktop/mobile result, compatibility boundary, and remaining deployment status in the evidence file.

- [ ] **Step 4: Commit implementation**

Commit production code, tests, and evidence with a Conventional Commit describing permanent instance deletion.

## Repair Track

- Root cause: backend deletion exists but the instance list has no owner for exposing it.
- Canonical owner changed: Overview instance-card UI gains the action and confirmation state.
- Smallest fix: add one permanent-delete flow; do not duplicate lifecycle cleanup in the browser.
- Verification: component request assertion plus desktop/mobile deletion journey.

## Retirement Track

- No existing delete UI or fallback is active.
- The backend `delete_data` optionality remains for API compatibility, but this UI always sends `true`.
- No additional cleanup branch is introduced.
