# Schedule Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let administrators safely edit and delete existing schedules, create every supported schedule with the required payload, and inspect detailed descriptions of all eight task types.

**Architecture:** Extract the schedule page from `App.tsx` into a focused React component. A single typed task catalog owns labels, applicability rules, payload summaries, and detailed help content. The component continues to use the existing schedule POST/DELETE API and submits a full preserved task record for edits; backend production owners remain unchanged and receive regression coverage only.

**Tech Stack:** React 19, TypeScript, Vitest/Testing Library, lucide-react, Go HTTP/scheduler tests, Playwright.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-16-schedule-management-design.md`, `CONTEXT.md`, and the files listed in `10-baseline-readset.md`.

**Compatibility Boundary:** Preserve `ScheduledTask` JSON/SQLite fields, existing schedule routes, task type strings, dispatcher payloads, Job history, and running-Job behavior. Editing must not change task ID, type, target, timezone, payload, or last-run time.

**Verification:** Focused Vitest RED/GREEN, full Vitest, scheduler/httpapi Go tests, full serial Go suite, TypeScript/Vite build, Playwright desktop/mobile main journey, layout assertions, `git diff --check`.

---

### Task 1: Schedule Page Contract And Main Interactions

**Files:**
- Create: `web/src/app/SchedulesPage.test.tsx`
- Create: `web/src/app/SchedulesPage.tsx`

**Why this task exists:** The current UI discards fields required for a safe update and exposes no edit, delete, or help commands. The main journey must prove a full preserved update, confirmed deletion, and complete task documentation.

**Impact / Compatibility:** This component owns presentation and request construction only. It must use `/api/schedules` and `/api/schedules/{id}` without changing their contracts.

**Repair Track:** Root cause is the incomplete inline React model and actionless list. The canonical fix is a full `ScheduledTask` model and commands in the schedule page.

**Retirement Track:** The old inline `SchedulesPage`, `ScheduledTask`, and `normalizeScheduledTask` in `App.tsx` will be removed in Task 2; no fallback remains.

**Verification:** `npm test -- --run src/app/SchedulesPage.test.tsx`.

- [ ] **Step 1: Write failing tests for help, edit, delete, and task payloads**

Create real component tests with fetch responses containing a complete schedule. Assert:

```tsx
await user.click(screen.getByRole("button", { name: "任务说明" }));
expect(screen.getByRole("dialog", { name: "计划任务类型说明" })).toHaveTextContent("游戏更新");
expect(screen.getByRole("dialog", { name: "计划任务类型说明" })).toHaveTextContent("清理");

await user.click(screen.getByRole("button", { name: "编辑 游戏更新" }));
await user.clear(screen.getByLabelText("Cron"));
await user.type(screen.getByLabelText("Cron"), "30 5 * * *");
await user.selectOptions(screen.getByLabelText("在线玩家策略"), "wait");
await user.click(screen.getByLabelText("启用计划"));
await user.click(screen.getByRole("button", { name: "保存修改" }));
expect(submitted).toMatchObject({
  id: "schedule-1",
  instance_id: "instance-1",
  type: "game_update",
  timezone: "Asia/Hong_Kong",
  payload: "{}",
  last_run: "2026-07-15T20:00:00Z",
  cron: "30 5 * * *",
  online_policy: "wait",
  enabled: false,
});
```

Also assert delete does not request before confirmation, then sends DELETE; `package_hot` submits `package_id`; `cleanup` submits `retention_days`.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `npm test -- --run src/app/SchedulesPage.test.tsx`

Expected: FAIL because `SchedulesPage.tsx` and the requested commands do not exist.

- [ ] **Step 3: Implement the typed task catalog and normalization**

Define:

```ts
export type ScheduledTask = {
  id: string;
  instance_id: string;
  type: ScheduleTaskType;
  cron: string;
  timezone: string;
  online_policy: OnlinePolicy;
  payload: string;
  enabled: boolean;
  last_run: string;
  next_run: string;
};

const TASK_TYPES: Record<ScheduleTaskType, TaskTypeDefinition> = {
  game_update: { label: "游戏更新", needsInstance: true, usesPlayerPolicy: true, ... },
  package_hot: { label: "插件热更新", needsInstance: true, usesPlayerPolicy: true, ... },
  package_full: { label: "插件完整更新", needsInstance: true, usesPlayerPolicy: true, ... },
  release_check: { label: "仅同步 GitHub 源", needsInstance: false, usesPlayerPolicy: false, ... },
  release_hot: { label: "GitHub Release 热更新", needsInstance: true, usesPlayerPolicy: true, ... },
  release_full: { label: "GitHub Release 完整更新", needsInstance: true, usesPlayerPolicy: true, ... },
  backup: { label: "备份", needsInstance: true, usesPlayerPolicy: true, ... },
  cleanup: { label: "清理", needsInstance: false, usesPlayerPolicy: false, ... },
};
```

Each definition contains the approved target, steps, interruption, parameters, and caution text from the design spec.

- [ ] **Step 4: Implement create/edit/delete state and request construction**

Use controlled fields. For edit, construct the body from the selected task and conditionally preserve `last_run`:

```ts
const body: Record<string, unknown> = {
  id: editing.id,
  instance_id: editing.instance_id,
  type: editing.type,
  cron,
  timezone: editing.timezone,
  online_policy: TASK_TYPES[editing.type].usesPlayerPolicy ? policy : "force",
  payload: editing.payload,
  enabled,
};
if (editing.last_run) body.last_run = editing.last_run;
```

Create payloads with `{ package_id }`, `{ source_id }`, or `{ retention_days }` according to type. Keep edit-only parameters read-only. Confirm deletion before DELETE and reload after success.

- [ ] **Step 5: Implement accessible help and delete dialogs**

Use lucide icons, icon tooltips, `role="dialog"`, `aria-modal`, Escape close, focus containment, and a scrollable description body. Deletion text must state that queued/running Jobs are not canceled.

- [ ] **Step 6: Run focused tests and verify GREEN**

Run: `npm test -- --run src/app/SchedulesPage.test.tsx`

Expected: all new component tests pass.

- [ ] **Step 7: Commit Task 1**

```bash
git add web/src/app/SchedulesPage.tsx web/src/app/SchedulesPage.test.tsx
git commit -m "feat(schedules): 增加计划编辑删除与任务说明"
```

### Task 2: App Integration And Responsive Styling

**Files:**
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles/app.css`

**Why this task exists:** The new component must replace the incomplete inline owner and match the existing quiet operational UI across desktop and mobile.

**Impact / Compatibility:** Navigation and existing App tests remain stable. `App` passes current instances and packages; source loading stays inside the schedule component.

**Repair Track:** Remove the field-dropping inline implementation and its actionless `Row` usage.

**Retirement Track:** Delete the old inline schedule types/functions completely after the imported component is wired. Keep `Row` only if another page still uses it; otherwise remove it.

**Verification:** Focused App tests, full Vitest, production build.

- [ ] **Step 1: Write failing App integration assertions**

Update schedule tests to expect Chinese task labels, the help command, edit/delete buttons, and `packages` flowing into creation behavior.

- [ ] **Step 2: Run relevant App tests and verify RED**

Run: `npm test -- --run src/app/App.test.tsx -t "Cron|scheduled|计划"`

Expected: FAIL until `App` uses the extracted component.

- [ ] **Step 3: Replace the inline component**

Import `SchedulesPage` and render:

```tsx
{page === "schedules" && (
  <SchedulesPage instances={instances} packages={packages} />
)}
```

Remove the old schedule type, normalizer, component, and unused icon imports.

- [ ] **Step 4: Add restrained operational styles**

Add stable two-column form/list sizing, full-width schedule rows, compact status indicators, icon-only edit/delete controls, read-only field summaries, and a scrollable help dialog. At `max-width: 800px`, collapse to one column and keep all controls within the viewport. Do not introduce nested decorative cards, gradients, or layout-shifting controls.

- [ ] **Step 5: Run focused and full frontend verification**

Run:

```bash
npm test -- --run src/app/SchedulesPage.test.tsx src/app/App.test.tsx
npm test -- --run
npm run build
```

Expected: all tests pass; build exits 0 with only the existing chunk-size advisory if still present.

- [ ] **Step 6: Commit Task 2**

```bash
git add web/src/app/App.tsx web/src/app/App.test.tsx web/src/styles/app.css
git commit -m "refactor(schedules): 接入独立计划任务管理页面"
```

### Task 3: Backend Contract Regression And Browser Journey

**Files:**
- Modify: `internal/httpapi/server_test.go`
- Modify: `internal/scheduler/service_test.go`
- Modify: `web/e2e/control-panel.spec.ts`
- Modify: `docs/aegis/work/2026-07-16-schedule-management/20-checkpoint.md`
- Modify: `docs/aegis/work/2026-07-16-schedule-management/40-atomic-tasks.md`
- Modify: `docs/aegis/work/2026-07-16-schedule-management/50-evidence.md`

**Why this task exists:** Frontend safety depends on the existing upsert/delete owner preserving a single record and rescheduling correctly. The real browser journey must prove persistence and responsive operation.

**Impact / Compatibility:** Backend production code changes only if regression tests reveal a contract defect. Existing routes and Job behavior remain unchanged.

**Repair Track:** Lock the already-present backend behavior that the repaired UI now relies on.

**Retirement Track:** No backend owner retires; the tests prevent future introduction of a parallel edit path.

**Verification:** Focused Go, full Go, Playwright desktop/mobile, source diff checks.

- [ ] **Step 1: Add backend regression tests**

HTTP test flow: create schedule, POST the same ID with changed Cron/policy/enabled, GET and assert one record with preserved type/payload/last run, DELETE, GET and assert empty. Scheduler service test saves an enabled task, updates it disabled, asserts one persisted record and a recomputed `NextRun`, then deletes it.

- [ ] **Step 2: Run backend tests**

Run: `go test -p 1 ./internal/scheduler ./internal/httpapi -count=1`

Expected: pass without backend production changes. If a behavior assertion fails, stop and repair only the canonical scheduler/API owner.

- [ ] **Step 3: Extend the real E2E schedule journey**

After creating a schedule:

```ts
await page.getByRole("button", { name: "任务说明" }).click();
await expect(page.getByRole("dialog", { name: "计划任务类型说明" })).toContainText("GitHub Release 完整更新");
await page.getByRole("button", { name: "关闭任务说明" }).click();
await page.getByRole("button", { name: "编辑 游戏更新" }).click();
await page.getByLabel("Cron").fill(mobile ? "45 4 * * *" : "30 4 * * *");
await page.getByLabel("启用计划").uncheck();
await page.getByRole("button", { name: "保存修改" }).click();
await page.reload();
await page.getByRole("button", { name: "计划任务" }).click();
await expect(page.getByText("停用", { exact: true })).toBeVisible();
await page.getByRole("button", { name: "删除 游戏更新" }).click();
await page.getByRole("button", { name: "确认删除计划" }).click();
await expect(page.getByText("暂无计划任务")).toBeVisible();
```

Add layout checks that the schedule stage and help dialog have no horizontal overflow and all buttons remain within the viewport.

- [ ] **Step 4: Run full verification**

Run:

```bash
go test -p 1 ./... -count=1
npm test -- --run
npm run build
npm run e2e -- --project=desktop
npm run e2e -- --project=mobile
git diff --check
```

- [ ] **Step 5: Record evidence and commit**

Write exact commands, exit codes, test counts, E2E results, layout findings, residual risks, and cleanup state to `50-evidence.md`; update task/checkpoint statuses.

```bash
git add internal/httpapi/server_test.go internal/scheduler/service_test.go web/e2e/control-panel.spec.ts docs/aegis/work/2026-07-16-schedule-management
git commit -m "test(schedules): 覆盖计划更新删除与浏览器主流程"
```
