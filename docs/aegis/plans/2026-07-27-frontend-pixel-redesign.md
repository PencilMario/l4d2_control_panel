# Frontend Pixel Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the React frontend to match the seven approved desktop reference images while preserving every existing API and behavior contract.

**Architecture:** Keep page state, API calls, and feature components as the behavior owners. Replace the application shell and visual DOM landmarks, then replace the legacy stylesheet with one coherent reference-driven design system shared by all pages and overlays.

**Tech Stack:** React, TypeScript, Vite, Vitest, Testing Library, Playwright, Lucide React, Recharts.

**Baseline / Authority Refs:** `CONTEXT.md`, `docs/frontend-redesign-brief.md`, `docs/aegis/specs/2026-07-27-frontend-pixel-redesign-design.md`, and `docs/PowerToys_Paste_20260727124901.png` through `docs/PowerToys_Paste_20260727125116.png`.

**Compatibility Boundary:** Backend endpoints, request payloads, data models, authentication, polling, background-task behavior, confirmation guards, file operations, and existing accessible command names remain stable.

**Verification:** `npm test -- --run`, `npm run build:web`, `npm run e2e`, and Playwright screenshots at the approved desktop viewport plus a narrow-screen overlap check.

---

### Task 1: Lock the new visual contract

**Files:**
- Modify: `web/src/styles/app.test.ts`
- Test: `web/src/styles/app.test.ts`

**Why this task exists:** The stylesheet currently tests legacy card motion instead of the approved shell, dimensions, and reference palette.

**Impact / Compatibility:** CSS ownership changes only; log token and bounded-workspace contracts stay active.

**Verification:** `npm test -- --run src/styles/app.test.ts`

- [ ] Add failing assertions for `--canvas`, `--surface`, `--accent`, the 46px top bar, 200px sidebar, bounded page content, low-radius controls, active navigation, and terminal overlay structure.
- [ ] Run the focused test and confirm it fails because the new visual tokens and shell selectors do not exist.
- [ ] Keep the failing test in place while Tasks 2 and 3 implement the contract.

### Task 2: Replace the application shell and overview landmarks

**Files:**
- Modify: `web/src/app/App.tsx`
- Test: `web/src/app/App.test.tsx`

**Why this task exists:** The top bar, sidebar, summary strip, instance panels, and control-console composition define the most visible reference images.

**Impact / Compatibility:** Keep button accessible names, API callbacks, instance state, polling, modals, and navigation page values unchanged.

**Repair Track:** Replace the legacy header/navigation and decorative English eyebrow ownership with reference-driven Chinese shell landmarks.

**Retirement Track:** Remove visual reliance on old `shell`, `brand`, `metric`, `card`, and decorative `eyebrow` styling; no legacy theme path remains.

**Verification:** `npm test -- --run src/app/App.test.tsx`

- [ ] Add assertions for the product top bar, control-node status, administrator session controls, primary navigation, summary strip, and wide instance panel landmarks.
- [ ] Run the focused tests and confirm the new landmark assertions fail.
- [ ] Restructure only the relevant JSX while retaining existing handlers and accessible command names.
- [ ] Run the focused tests and confirm behavior and landmark assertions pass.

### Task 3: Replace the legacy stylesheet with the approved design system

**Files:**
- Modify: `web/src/styles/app.css`
- Test: `web/src/styles/app.test.ts`

**Why this task exists:** The old 2,746-line dark industrial stylesheet is the legacy design owner and must retire as a whole.

**Impact / Compatibility:** Preserve class coverage needed by existing components, focus visibility, reduced motion, scroll containment, modal stacking, and narrow-screen operability.

**Repair Track:** Implement one palette, spacing scale, shell geometry, controls, tables, workspaces, dialogs, logs, charts, status colors, and responsive fallback from the seven images.

**Retirement Track:** Delete legacy tokens, English decorative treatments, green card grid styling, theme remnants, and duplicate responsive rules.

**Verification:** `npm test -- --run src/styles/app.test.ts && npm run build:web`

- [ ] Replace root tokens and shell geometry, then run the CSS contract test until it passes.
- [ ] Implement overview, content repository, settings, schedules, jobs, and shared form/table styles.
- [ ] Implement private files, game logs, task logs, console, player, configuration, confirmation, upload, and snapshot overlay styles.
- [ ] Add the single narrow-screen fallback required to keep navigation and key commands reachable.
- [ ] Run the CSS test and TypeScript production build.

### Task 4: Align feature pages with their reference compositions

**Files:**
- Modify: `web/src/app/PrivateFilesPage.tsx`
- Modify: `web/src/app/GameLogsPage.tsx`
- Modify: `web/src/app/JobsPage.tsx`
- Modify: `web/src/app/JobLogsPage.tsx`
- Modify: `web/src/app/SchedulesPage.tsx`
- Modify: `web/src/app/InstanceConfigModal.tsx`
- Test: corresponding `*.test.tsx` files under `web/src/app`

**Why this task exists:** Existing feature components own the exact double-pane, compact-table, log-viewer, and modal DOM needed by five reference images.

**Impact / Compatibility:** API calls, race guards, focus traps, upload continuation, filtering, downloads, and action labels remain unchanged.

**Verification:** `npm test -- --run src/app/PrivateFilesPage.test.tsx src/app/GameLogsPage.test.tsx src/app/JobsPage.test.tsx src/app/JobLogsPage.test.tsx src/app/SchedulesPage.test.tsx src/app/InstanceConfigModal.test.tsx`

- [ ] Add one failing structural assertion per reference-backed page where the current DOM cannot express the screenshot.
- [ ] Run the focused page tests and confirm only the new structure assertions fail.
- [ ] Apply the smallest semantic wrappers/classes required for toolbar, split workspace, task table, task log, and modal compositions.
- [ ] Run all focused page tests and confirm they pass.

### Task 5: Visual regression and final verification

**Files:**
- Modify only files already listed when screenshot evidence reveals a mismatch.
- Record: `docs/aegis/work/2026-07-27-frontend-pixel-redesign/50-evidence.md`

**Why this task exists:** Pixel-oriented acceptance requires rendered evidence; DOM and unit tests cannot prove framing, spacing, overflow, or visual hierarchy.

**Impact / Compatibility:** Corrections are limited to visual structure and CSS. No new functionality or API changes are allowed.

**Verification:** Full unit suite, production build, E2E journey, desktop screenshots, and narrow-screen inspection.

- [ ] Start the existing Vite/fixture workflow on an available local port.
- [ ] Capture the overview, private files, content repository, game logs, background jobs, task log, and game console at the reference desktop dimensions.
- [ ] Compare shell geometry, content width, component positions, palette, typography, borders, spacing, overflow, and overlay framing against each PNG; correct material mismatches.
- [ ] Capture one narrow viewport and correct any overlapping text, unreachable navigation, or hidden primary command.
- [ ] Run `npm test -- --run` and confirm the complete Vitest suite passes.
- [ ] Run `npm run build:web` and confirm TypeScript and Vite production compilation pass.
- [ ] Run `npm run e2e` when the fixture prerequisites are available; otherwise record the exact blocker and completed substitute checks.
- [ ] Write commands, results, screenshot paths, and residual differences to the evidence file.

## Plan Self-Review

- The seven references map to Tasks 2, 3, 4, and 5.
- Existing business behavior remains owned by current React components and tests.
- The old stylesheet owner explicitly retires; no fallback theme remains.
- Unshown pages receive only the shared design primitives needed to keep current functions usable.
- No backend, API, data contract, or feature expansion is included.
