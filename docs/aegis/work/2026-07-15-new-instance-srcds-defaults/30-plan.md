# New Instance SRCDS Defaults Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make newly created instances default to versus mode, 32 players, and the approved extra SRCDS arguments without changing existing instances.

**Architecture:** Keep `createDefaults` in the React instance configuration modal as the sole owner of browser-created instance defaults. Preserve the existing managed command builder and backend/runtime contracts; only the initial form values change, and the existing submission path persists them.

**Tech Stack:** React, TypeScript, Vitest, Testing Library, Vite

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-15-new-instance-srcds-defaults-design.md`, `docs/aegis/specs/2026-07-15-instance-startup-package-design.md`, `web/src/app/InstanceConfigModal.tsx`, `web/src/app/InstanceConfigModal.test.tsx`

**Compatibility Boundary:** Existing instances must continue to use their persisted values. API, database, Docker environment variables, Supervisor defaults, managed argument ordering, `-console`, and reserved-option validation remain unchanged.

**Verification:** Run the focused Vitest file, the complete frontend Vitest suite, and the production frontend build.

---

### Task 1: Change New Instance Defaults

**Files:**
- Modify: `web/src/app/InstanceConfigModal.test.tsx`
- Modify: `web/src/app/InstanceConfigModal.tsx`

**Why this task exists:**
- New instances need to start from the approved versus/32-player configuration and prefilled extra arguments.
- The create modal preview and submitted values must agree before an administrator creates the instance.

**Impact / Compatibility:**
- Only the initial state used when `mode="create"` has no `instance` changes.
- The edit branch continues copying every saved value from `instance`, so existing instances are not migrated.

**Repair Track:**
- Root cause: `createDefaults` still contains the old `coop`, `8`, and empty-extra defaults.
- Canonical owner: `createDefaults` in `InstanceConfigModal.tsx`.
- Smallest change: replace those three initial values and protect the complete default preview with a component test.
- Compatibility: do not alter `buildLaunchPreview`, API payload types, or runtime command generation.

**Retirement Track:**
- Old owner: none; there is a single default object.
- Old values `coop`, `8`, and empty `extra_args` retire only from new-instance initialization.
- Existing persisted values remain active and are verified through the edit-form regression test.

**Verification:**
- `npm test -- --run src/app/InstanceConfigModal.test.tsx`
- `npm test -- --run`
- `npm run build`

- [ ] **Step 1: Write the failing default behavior test**

Add this test to `web/src/app/InstanceConfigModal.test.tsx`:

```tsx
it("loads the approved SRCDS defaults for new instances", () => {
  render(
    <InstanceConfigModal
      mode="create"
      packages={[packageA]}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />,
  );

  expect(screen.getByLabelText("模式")).toHaveValue("versus");
  expect(screen.getByLabelText("Tickrate")).toHaveValue(100);
  expect(screen.getByLabelText("最大玩家")).toHaveValue(32);
  expect(screen.getByLabelText("额外 SRCDS 启动项")).toHaveValue(
    "-sv_lan 0 -ip 0.0.0.0 +sv_clockcorrection_msecs 25 -timeout 10 +sv_setmax 32 +servercfgfile server.cfg",
  );
  expect(screen.getByLabelText("启动指令预览")).toHaveTextContent(
    "./srcds_run -game left4dead2 -console -port 27015 -tickrate 100 +map c2m1_highway +mp_gamemode versus -maxplayers 32 -sv_lan 0 -ip 0.0.0.0 +sv_clockcorrection_msecs 25 -timeout 10 +sv_setmax 32 +servercfgfile server.cfg",
  );
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run from `web`:

```powershell
npm test -- --run src/app/InstanceConfigModal.test.tsx
```

Expected: the new test fails because the mode is `coop`, maximum players is `8`, and extra arguments are empty.

- [ ] **Step 3: Implement the minimal new-instance defaults**

Change the relevant fields in `createDefaults` in `web/src/app/InstanceConfigModal.tsx` to:

```tsx
game_mode: "versus",
tickrate: 100,
max_players: 32,
extra_args:
  "-sv_lan 0 -ip 0.0.0.0 +sv_clockcorrection_msecs 25 -timeout 10 +sv_setmax 32 +servercfgfile server.cfg",
```

- [ ] **Step 4: Run focused and regression verification**

Run from `web`:

```powershell
npm test -- --run src/app/InstanceConfigModal.test.tsx
npm test -- --run
npm run build
```

Expected: all commands exit successfully with no TypeScript, Vitest, or Vite errors.

- [ ] **Step 5: Record evidence and commit**

Update `docs/aegis/work/2026-07-15-new-instance-srcds-defaults/40-atomic-tasks.md` with completed checkboxes and create `50-evidence.md` containing the RED and GREEN command results.

```powershell
git add web/src/app/InstanceConfigModal.tsx web/src/app/InstanceConfigModal.test.tsx docs/aegis/work/2026-07-15-new-instance-srcds-defaults
git commit -m "feat(srcds): 更新新建实例默认启动项"
```
