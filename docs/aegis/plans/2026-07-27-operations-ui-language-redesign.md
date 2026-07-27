# Operations UI Language Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the remaining legacy-looking presentation with a coherent high-density operations interface while preserving all behavior.

**Architecture:** Keep React state and API owners intact. Introduce explicit visual roles in existing page DOM, replace generic button/card ownership in the stylesheet, and verify every slice against the deployed application with Playwright MCP.

**Tech Stack:** React, TypeScript, CSS, Vitest, Testing Library, Playwright, Lucide React.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-27-operations-ui-language-redesign.md`, seven `docs/PowerToys_Paste_*.png` images, current remote screenshots, and existing behavior tests.

**Compatibility Boundary:** Backend/API/auth/data contracts and every existing operation remain stable.

**Verification:** Focused RED/GREEN tests, `npm test -- --run`, `npm run build:web`, Go fixture tests, desktop/mobile E2E, and remote Playwright screenshots at 1463x800.

---

### Task 1: Shared control language

**Files:** Modify `web/src/styles/app.css`, `web/src/styles/app.test.ts`.

**Repair Track:** Establish tool, primary, secondary, danger and status roles with fixed dimensions and interaction states.

**Retirement Track:** Retire generic raised rectangular button styling, `.create` visual ownership and nested card appearance.

- [ ] Add failing CSS contract assertions for fixed icon tools, single primary role, outline danger role, status badges and unframed sections.
- [ ] Run `npm test -- --run src/styles/app.test.ts` and confirm RED.
- [ ] Replace shared primitives and run the focused test to GREEN.

### Task 2: Overview instance workspace

**Files:** Modify `web/src/app/App.tsx`, `web/src/app/PerformancePanel.tsx`, their tests, and `web/src/styles/app.css`.

**Repair Track:** Make one stable header, one eight-column key-metric row and one independent chart.

**Retirement Track:** Remove the flowing ten-metric block, storage-note text from layout, absolute action pile and separate map/player row.

- [ ] Add failing structural assertions for the eight-key-metric row and command grouping.
- [ ] Implement semantic metric composition while preserving source data and callbacks.
- [ ] Run focused component tests and capture the remote overview at 1463x800.

### Task 3: File, log and repository workspaces

**Files:** Modify relevant page JSX only where structure is required and shared CSS.

- [ ] Add/adjust structure contracts for one toolbar, split workspace and non-overlapping status footer.
- [ ] Replace old generic button/action styling with tool-role classes.
- [ ] Verify private files, content repository and game logs through focused tests and remote screenshots.

### Task 4: Task surfaces and overlays

**Files:** Modify `JobsPage.tsx`, `JobLogsPage.tsx`, overlay components and shared CSS.

- [ ] Lock stable table columns, isolated expanded logs and overlay framing with tests.
- [ ] Replace button-like statuses and mixed action groups.
- [ ] Verify jobs, task logs and console screenshots.

### Task 5: Responsive and release verification

**Files:** Modify responsive CSS, E2E assertions and evidence record only as required.

- [ ] Check every navigation item and primary command at 390x844.
- [ ] Run full unit, build, Go fixture and desktop/mobile E2E verification.
- [ ] Hot-deploy static assets to 安可服 with a rollback directory, capture all target pages, and verify Panel/game container IDs remain unchanged.
