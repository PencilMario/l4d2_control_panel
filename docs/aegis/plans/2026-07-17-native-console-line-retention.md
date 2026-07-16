# Native Console Line Retention Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Retain exactly the newest 1000 native-console lines per browser session.

**Architecture:** Replace the array of WebSocket chunks with a bounded text buffer owned by a small pure helper. The Terminal component appends decoded frames through the helper; WebSocket, PTY, command submission, and follow behavior remain unchanged.

**Tech Stack:** React, TypeScript, Vitest, Testing Library, Playwright.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-17-native-console-line-retention-design.md`, `web/src/app/App.tsx`, `web/src/app/App.test.tsx`.

**Compatibility Boundary:** Do not change the WebSocket protocol, backend console proxy, command input, or scroll-follow semantics.

**Verification:** `npm test -- --run`, `npm run build`, and the desktop/mobile Playwright console journey.

---

### Task 1: Bounded Console Text Buffer

**Files:**
- Create: `web/src/app/consoleBuffer.ts`
- Create: `web/src/app/consoleBuffer.test.ts`
- Modify: `web/src/app/App.tsx`

**Why this task exists:** WebSocket frames are not lines, so the current 500-frame slice does not enforce a meaningful output limit.

**Impact / Compatibility:** Only browser-session output memory changes; the server remains the stream owner.

- [ ] Write failing helper tests for one oversized frame, multiple frames, and an unfinished trailing line.
- [ ] Run `npm test -- --run src/app/consoleBuffer.test.ts` and verify RED.
- [ ] Implement `appendConsoleOutput(current, incoming, 1000)` by trimming before the oldest retained newline boundary.
- [ ] Replace Terminal's `string[]` state with a string and append every decoded frame through the helper.
- [ ] Run helper and App tests and verify GREEN.

### Task 2: Regression and Integration

**Files:**
- Modify: `web/src/app/App.test.tsx` only if the console integration needs an observable assertion.
- Create: `docs/aegis/work/2026-07-17-native-console-line-retention/50-evidence.md`

**Why this task exists:** Prove retention does not regress follow, input, binary frames, or responsive console behavior.

- [ ] Run `npm test -- --run` and `npm run build`.
- [ ] Run `npx playwright test e2e/control-panel.spec.ts` for desktop and mobile.
- [ ] Record RED/GREEN evidence, test counts, build output, and compatibility boundary.
- [ ] Commit, merge to `main`, push, deploy the Panel, and verify health with proxy values empty.

## Repair Track

- Root cause: the frontend counts transport frames instead of logical lines.
- Canonical owner: a pure frontend text-buffer helper.
- Smallest fix: replace the 500-frame slice; retain server behavior.

## Retirement Track

- Retire `[...old, text].slice(-500)` completely.
- Do not retain a second frame-based fallback.
