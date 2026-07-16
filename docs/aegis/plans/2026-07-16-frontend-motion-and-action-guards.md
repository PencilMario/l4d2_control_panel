# Frontend Motion and Action Guards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add restrained, layered motion across the control panel and prevent duplicate submissions for every asynchronous write operation.

**Architecture:** Shared CSS variables and state selectors own visual motion, while each React feature keeps ownership of its asynchronous business state. Existing local `busy`, `submitting`, `deleting`, and saving flags remain canonical; missing guards use resource-scoped keys or dialog-local state so unrelated instances remain operable.

**Tech Stack:** React, TypeScript, CSS, Lucide React, Vitest, Testing Library, Playwright. No new runtime dependency is required.

**Baseline / Authority Refs:** `CONTEXT.md`, `docs/aegis/specs/2026-07-16-frontend-motion-and-action-guards-design.md`, existing component tests and `web/e2e/control-panel.spec.ts`.

**Compatibility Boundary:** Keep HTTP APIs, payloads, navigation, confirmation steps, polling, error messages, layout, branding, and per-instance independence stable. Read-only requests and navigation must not acquire write locks.

**Verification:** Targeted Vitest tests for each slice, `npm test -- --run`, `npm run build`, Playwright desktop/mobile checks, and CSS inspection for reduced-motion coverage.

---

### Task 1: Shared layered motion system

**Files:**
- Modify: `web/src/styles/app.css`
- Create: `web/src/styles/app.test.ts`

**Why this task exists:** Give controls, cards, rows, dialogs, and content changes consistent feedback without changing layout dimensions or delaying operation.

**Impact / Compatibility:** CSS only. Existing class names remain valid; reduced-motion users must receive an effectively static interface.

**Verification:** `npm test -- --run src/styles/app.test.ts`

- [ ] **Step 1: Write the failing CSS contract test**

```ts
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const css = readFileSync(new URL("./app.css", import.meta.url), "utf8");

describe("shared interaction motion", () => {
  it("defines layered hover, busy and reduced-motion states", () => {
    expect(css).toContain("--motion-fast:");
    expect(css).toMatch(/\.instance-card:hover/);
    expect(css).toMatch(/\[aria-busy=["']true["']\]/);
    expect(css).toContain("@media (prefers-reduced-motion: reduce)");
  });
});
```

- [ ] **Step 2: Run the test and verify RED**

Run: `npm test -- --run src/styles/app.test.ts`
Expected: FAIL because the motion variables, hover contract, busy selector, and reduced-motion block are absent.

- [ ] **Step 3: Add the shared CSS implementation**

Define motion variables near `:root`, add 160–240 ms transitions to buttons, links, cards, rows, navigation and dialogs, add stable busy-icon rotation, and finish with:

```css
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    scroll-behavior: auto !important;
    transition-duration: 0.01ms !important;
  }
}
```

- [ ] **Step 4: Run the test and verify GREEN**

Run: `npm test -- --run src/styles/app.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add web/src/styles/app.css web/src/styles/app.test.ts
git commit -m "feat(frontend): 增加层次化组件动效"
```

### Task 2: Guard login and per-instance lifecycle submissions

**Files:**
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`

**Why this task exists:** Login and game instance lifecycle actions are the highest-risk duplicate submission paths and must visibly lock immediately.

**Impact / Compatibility:** Preserve current API calls and confirmation flow. Locks use `${instanceID}:${action}` keys; one instance cannot block another.

**Verification:** `npm test -- --run src/app/App.test.tsx`

- [ ] **Step 1: Write failing login and instance-action tests**

Add tests using deferred fetch responses which click twice before resolution and assert one POST, `disabled`, and `aria-busy="true"`. Add a two-instance test proving instance B remains enabled while instance A is pending. Add a rejection test proving the originating button is enabled again.

```ts
fireEvent.click(loginButton);
fireEvent.click(loginButton);
expect(loginPosts).toBe(1);
expect(loginButton).toBeDisabled();
expect(loginButton).toHaveAttribute("aria-busy", "true");
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `npm test -- --run src/app/App.test.tsx -t "prevents duplicate"`
Expected: FAIL because login and lifecycle buttons remain enabled and issue duplicate calls.

- [ ] **Step 3: Implement minimal local guards**

Add `submitting` to `Login`, and a `Set<string>`-shaped state plus synchronous ref guard in `App` for lifecycle/task creation. Wrap every guarded async handler in `try/finally`; pass pending keys into `Overview`; render busy labels and a spinning `RefreshCw` without changing button dimensions.

```tsx
if (actionLocks.current.has(key)) return;
actionLocks.current.add(key);
setPendingActions((current) => new Set(current).add(key));
try {
  await action(id, kind);
} finally {
  actionLocks.current.delete(key);
  setPendingActions((current) => {
    const next = new Set(current);
    next.delete(key);
    return next;
  });
}
```

- [ ] **Step 4: Run the tests and verify GREEN**

Run: `npm test -- --run src/app/App.test.tsx`
Expected: PASS with one request per pending operation and independent instance buttons.

- [ ] **Step 5: Commit**

```powershell
git add web/src/app/App.tsx web/src/app/App.test.tsx
git commit -m "fix(frontend): 阻止登录与实例操作重复提交"
```

### Task 3: Complete write-operation guards across feature pages

**Files:**
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/app/PrivateFilesPage.tsx`
- Modify: `web/src/app/PrivateFilesPage.test.tsx`
- Modify: `web/src/app/SchedulesPage.tsx`
- Modify: `web/src/app/SchedulesPage.test.tsx`
- Modify: `web/src/app/InstanceConfigModal.tsx`
- Modify: `web/src/app/InstanceConfigModal.test.tsx`

**Why this task exists:** Saving, deleting, uploading, applying changes, settings updates, and player moderation must follow the same immediate lock and failure recovery behavior.

**Impact / Compatibility:** Reuse existing local owners. `PrivateFilesPage`, schedules, and instance configuration already have state and need synchronous re-entry guards plus `aria-busy`; content/settings/player operations need scoped pending keys. Read-only refresh and navigation remain available.

**Verification:** Targeted page tests followed by the full frontend suite.

- [ ] **Step 1: Add failing representative tests**

Add deferred-response tests for private-file apply, schedule save/delete, instance save, content source save/delete, settings save, and player confirmation. Assert duplicate clicks make one mutation request, the initiating control becomes disabled/busy, and rejection restores it.

- [ ] **Step 2: Run targeted tests and verify RED**

Run: `npm test -- --run src/app/PrivateFilesPage.test.tsx src/app/SchedulesPage.test.tsx src/app/InstanceConfigModal.test.tsx src/app/App.test.tsx -t "pending|duplicate|busy"`
Expected: At least the newly added assertions fail on missing synchronous guards or accessibility state.

- [ ] **Step 3: Implement guards in the canonical owners**

Use refs alongside existing boolean state where two events can occur before React rerenders:

```ts
if (busyRef.current) return;
busyRef.current = true;
setBusy(true);
try {
  await operation();
} finally {
  busyRef.current = false;
  setBusy(false);
}
```

For multiple independent App-level operations, use scoped string keys rather than a global boolean. Add `disabled`, `aria-busy`, and stable busy text/icon to each initiating control. Confirmation dialogs accept an async callback and keep their confirm button mounted and disabled until it settles.

- [ ] **Step 4: Run targeted tests and verify GREEN**

Run: `npm test -- --run src/app/PrivateFilesPage.test.tsx src/app/SchedulesPage.test.tsx src/app/InstanceConfigModal.test.tsx src/app/App.test.tsx`
Expected: PASS.

- [ ] **Step 5: Run the full unit suite**

Run: `npm test -- --run`
Expected: PASS with no warnings or unhandled promise rejections.

- [ ] **Step 6: Commit**

```powershell
git add web/src/app/App.tsx web/src/app/App.test.tsx web/src/app/PrivateFilesPage.tsx web/src/app/PrivateFilesPage.test.tsx web/src/app/SchedulesPage.tsx web/src/app/SchedulesPage.test.tsx web/src/app/InstanceConfigModal.tsx web/src/app/InstanceConfigModal.test.tsx
git commit -m "fix(frontend): 统一异步写操作忙碌状态"
```

### Task 4: Build and browser verification

**Files:**
- Modify: `web/e2e/control-panel.spec.ts`
- Create: `docs/aegis/work/2026-07-16-frontend-motion-and-action-guards/50-evidence.md`

**Why this task exists:** Unit tests cannot prove layout stability, motion restraint, mobile fit, or visible busy feedback in the real application.

**Impact / Compatibility:** E2E coverage extends the existing fixture only. No production behavior is added in this slice.

**Verification:** Build, Playwright, screenshots at desktop/mobile, and browser console review.

- [ ] **Step 1: Extend the main E2E journey**

Add assertions that a representative mutation button disables immediately, keeps its dimensions while pending, and does not produce a second network request. Check focus-visible navigation and ensure the mobile viewport has no horizontal overflow.

- [ ] **Step 2: Run build and E2E**

Run: `npm run build`
Expected: TypeScript and Vite build succeed.

Run: `npm run e2e`
Expected: All Playwright tests pass.

- [ ] **Step 3: Inspect desktop and mobile screenshots**

Use 1440x900 and 390x844 viewports. Verify cards do not overlap, button labels fit, busy icons rotate without resizing, dialogs remain visible, and reduced-motion emulation removes nonessential transitions.

- [ ] **Step 4: Record evidence**

Document exact command results, screenshot paths, any residual risks, and a drift check confirming API and layout boundaries remained intact in `docs/aegis/work/2026-07-16-frontend-motion-and-action-guards/50-evidence.md`.

- [ ] **Step 5: Commit**

```powershell
git add web/e2e/control-panel.spec.ts docs/aegis/work/2026-07-16-frontend-motion-and-action-guards/50-evidence.md
git commit -m "test(frontend): 验证动效与操作防重复主流程"
```

## Repair Track

- Root cause: several async handlers rely only on React state updates, so a second click can enter before rerender; other handlers have no pending state at all.
- Canonical owners: the feature component that initiates each mutation and the shared CSS state layer.
- Smallest repair: synchronous ref/key guards plus visible disabled/busy state; no API or backend changes.
- Verification: deferred promise tests demonstrate one mutation per pending resource and recovery after rejection.

## Retirement Track

- Existing local flags remain active and gain synchronous guards and accessible presentation.
- Ad hoc unguarded mutation handlers retire as they are converted to the same pattern.
- No global lock, duplicate request wrapper, compatibility adapter, or fallback branch is introduced.
