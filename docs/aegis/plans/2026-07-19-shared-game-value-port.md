# Shared Game Value Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Selectively port the still-valuable shared-game work onto current `main`: real shared-game status, correct Overlay storage accounting, and schedules that follow each instance's configured package.

**Architecture:** Keep the current shared-game coordinator, migration API, persistent game logs, and React navigation as canonical. Extend `/api/game` with derived display metadata, correct storage at the metrics owner, and move scheduled package/release selection from stale task payloads to the target instance configuration. Port behavior through focused tests instead of merging the obsolete branch wholesale.

**Tech Stack:** Go 1.24, Chi HTTP API, React 19, TypeScript, Vitest.

**Baseline / Authority Refs:** `README.md`; `CONTEXT.md`; `docs/aegis/specs/2026-07-17-shared-game-overlay-design.md`; approved user direction on 2026-07-19 that schedules must use the instance's current package.

**Compatibility Boundary:** Preserve `/api/game/update`, shared-game migration and maintenance gates, existing schedule types and stored records, persistent game-log behavior, and instance content precedence. `release_check` remains tied to an explicit GitHub source. Package and release update schedules intentionally stop honoring stored `package_id`/`source_id` payloads.

**Verification:** Targeted Go and Vitest tests per task, followed by `go test -p 1 ./...`, `go vet ./...`, `npm test -- --run`, `npm run build`, and `git diff --check`.

---

### Task 1: Shared-game status and UI

**Files:**
- Create: `internal/httpapi/game_version.go`
- Create: `internal/httpapi/game_version_test.go`
- Modify: `internal/domain/models.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`
- Modify: `cmd/panel/main.go`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles/app.css`

**Why this task exists:** Administrators currently have a working global update API but cannot see the installed shared-game version or state in the UI.

**Impact / Compatibility:** `/api/game` gains derived `version` and `path` response fields without changing persisted columns. Missing or unreadable `steam.inf` renders “版本未知”; it does not fail the status API or update path.

**Repair Track:** The canonical display source is `/api/game`, with `PatchVersion` derived from the configured shared-game current path. The smallest repair adds response-only fields and UI loading.

**Retirement Track:** The control-channel placeholder metric retires. Release UUIDs remain internal identifiers and are not presented as game versions.

**Verification:** `go test ./internal/httpapi ./cmd/panel`; `npm test -- --run src/app/App.test.tsx`.

- [ ] **Step 1: Write failing backend tests**

```go
func TestReadSharedGameVersion(t *testing.T) {
    current := t.TempDir()
    requireWrite(t, filepath.Join(current, "left4dead2", "steam.inf"), "PatchVersion=2.2.4.3\n")
    got, err := readSharedGameVersion(current)
    if err != nil || got != "2.2.4.3" { t.Fatalf("version=%q err=%v", got, err) }
}
```

Extend `TestGlobalGameStatusAndUpdate` to pass `WithSharedGamePath(current)` and require `"version":"2.2.4.3"` plus `"path":"/data/game/current"`.

- [ ] **Step 2: Verify backend RED**

Run: `go test ./internal/httpapi ./cmd/panel`
Expected: FAIL because `readSharedGameVersion` and `WithSharedGamePath` do not exist.

- [ ] **Step 3: Implement derived status fields**

```go
type SharedGameState struct {
    // persisted fields stay unchanged
    Version string `json:"version,omitempty"`
    Path    string `json:"path,omitempty"`
}

func WithSharedGamePath(path string) Option {
    return func(s *Server) { s.sharedGamePath = path }
}
```

Parse `left4dead2/steam.inf` case-insensitively for non-empty `PatchVersion`; derive fields only in `gameStatus` and wire `cfg.GameCurrentPath` from `cmd/panel/main.go`.

- [ ] **Step 4: Verify backend GREEN**

Run: `go test ./internal/httpapi ./cmd/panel`
Expected: PASS.

- [ ] **Step 5: Write failing frontend tests**

```tsx
expect(sharedGameVersionLabel({ version: "2.2.4.3", active_release_id: "uuid" })).toBe("2.2.4.3");
expect(sharedGameVersionLabel({ active_release_id: "uuid" })).toBe("版本未知");
expect(await screen.findByText("保存位置")).toBeVisible();
expect(screen.getByRole("button", { name: "更新共享游戏本体" })).toHaveClass("shared-game-update");
```

- [ ] **Step 6: Verify frontend RED**

Run: `npm test -- --run src/app/App.test.tsx`
Expected: FAIL because shared-game state is not loaded or displayed.

- [ ] **Step 7: Implement the minimal shared-game UI**

Load `/api/game` on login and after completed jobs. Show version, `/data/game/current`, and migration state in Content Repository; refresh after update. Add responsive `.shared-game-details`, `.shared-game-policy`, and `.shared-game-update` styles. Do not add the unpopulated `size_bytes` tile.

- [ ] **Step 8: Verify frontend GREEN and commit**

Run: `npm test -- --run src/app/App.test.tsx`
Expected: PASS.

Commit: `feat(game): 展示共享游戏本体版本与状态`

### Task 2: Overlay storage accounting and memoization

**Files:**
- Modify: `internal/metrics/storage.go`
- Create: `internal/metrics/storage_test.go`
- Modify: `web/src/app/PerformancePanel.tsx`
- Modify: `web/src/app/PerformancePanel.test.tsx`

**Why this task exists:** After Overlay migration, `instances/<id>/game` is a symlink to the merged view. Counting that path returns zero or risks counting lower shared content instead of per-instance writable storage; storage-only snapshot changes are also hidden by React memoization.

**Impact / Compatibility:** API field names and total formula remain unchanged. Only the source for per-instance `game_size_bytes` changes to `overlay/upper` for the known managed symlink.

**Repair Track:** `DirectoryStorage` owns filesystem accounting; `PerformancePanel` owns render memo keys. Tests cover both canonical owners.

**Retirement Track:** The generic `game` path remains the fallback for legacy layouts. Only the recognized `overlay/merged` link redirects to `overlay/upper`.

**Verification:** `go test ./internal/metrics`; `npm test -- --run src/app/PerformancePanel.test.tsx`.

- [ ] **Step 1: Write failing storage test**

```go
func TestDirectoryStorageCountsOverlayUpperInsteadOfMerged(t *testing.T) {
    // game -> overlay/merged; upper contains "upper"; merged also contains lower data
    usage, err := (DirectoryStorage{Root: root}).InstanceStorage(context.Background(), "instance-1")
    if err != nil || usage.Game != uint64(len("upper")) { t.Fatalf("usage=%+v err=%v", usage, err) }
}
```

- [ ] **Step 2: Verify storage RED**

Run: `go test ./internal/metrics -run TestDirectoryStorageCountsOverlayUpperInsteadOfMerged`
Expected: FAIL with game usage zero or the wrong size.

- [ ] **Step 3: Implement recognized-link resolution**

Add `instanceGameStoragePath(base string)` using `Lstat`, `Readlink`, absolute normalization, and exact comparison with `<base>/overlay/merged`; return `<base>/overlay/upper` only for that managed layout.

- [ ] **Step 4: Verify storage GREEN**

Run: `go test ./internal/metrics`
Expected: PASS.

- [ ] **Step 5: Write failing memo test**

```tsx
expect(performancePanelPropsEqual(
  { snapshot, history, loading: false },
  { snapshot: { ...snapshot, game_size_bytes: 99 }, history, loading: false },
)).toBe(false);
```

- [ ] **Step 6: Verify memo RED, implement keys, and verify GREEN**

Run: `npm test -- --run src/app/PerformancePanel.test.tsx`
Expected RED: comparison incorrectly returns true.

Add `game_size_bytes`, `private_size_bytes`, `backups_size_bytes`, and `console_size_bytes` to `SNAPSHOT_KEYS`, then rerun the command and expect PASS.

- [ ] **Step 7: Commit**

Commit: `fix(metrics): 按 Overlay 写层统计实例存储`

### Task 3: Instance-owned scheduled package and release updates

**Files:**
- Modify: `internal/automation/dispatcher.go`
- Modify: `internal/automation/dispatcher_test.go`
- Modify: `cmd/panel/main.go`
- Modify: `web/src/app/SchedulesPage.tsx`
- Modify: `web/src/app/SchedulesPage.test.tsx`
- Modify: `web/src/styles/app.css`

**Why this task exists:** A schedule currently captures a package or source payload that can drift from the instance's configured package. The user explicitly requires execution to follow the target instance's current configuration.

**Impact / Compatibility:** Existing task types and rows remain readable. For `package_hot`, `package_full`, `release_hot`, and `release_full`, old payload package/source IDs become ignored compatibility data. `release_check` continues using its explicit source.

**Repair Track:** The dispatcher resolves the target instance, then its `SelectedPackageID`, then the package metadata. The form no longer presents a competing owner.

**Retirement Track:** Package/source selectors and payload ownership retire only for instance update tasks. Stored old payloads are retained for record compatibility but no longer drive execution.

**Verification:** `go test ./internal/automation ./cmd/panel`; `npm test -- --run src/app/SchedulesPage.test.tsx`.

- [ ] **Step 1: Write failing dispatcher tests**

```go
func TestScheduledPackageUpdateUsesInstanceSelectedPackage(t *testing.T) {
    d := Dispatcher{Instances: fakeInstanceRepo{instance: domain.Instance{SelectedPackageID: selected.ID}}, Packages: manager, PackagesUpdate: updater}
    err := d.run(context.Background(), domain.ScheduledTask{InstanceID: "instance", Type: "package_hot", Payload: `{"package_id":"wrong"}`})
    if err != nil || updater.packageID != selected.ID { t.Fatalf("package=%q err=%v", updater.packageID, err) }
}
```

Add a release-update test requiring repository and escaped filename pattern from the selected package while `release_check` still resolves `source_id`.

- [ ] **Step 2: Verify dispatcher RED**

Run: `go test ./internal/automation`
Expected: FAIL because the payload still owns selection.

- [ ] **Step 3: Implement instance-owned resolution**

Add `Instances.Instance(ctx, id)` to `Dispatcher`, wire the store from `cmd/panel/main.go`, and add `selectedPackage(ctx, task)`. Use it for package/release update tasks; restrict the existing GitHub source lookup to `release_check`.

- [ ] **Step 4: Verify dispatcher GREEN**

Run: `go test ./internal/automation ./cmd/panel`
Expected: PASS.

- [ ] **Step 5: Write failing schedule-form tests**

```tsx
await user.selectOptions(await screen.findByLabelText("任务"), "package_hot");
expect(screen.queryByLabelText("插件包")).not.toBeInTheDocument();
expect(screen.getByText(/使用目标实例当前配置的插件包/)).toBeVisible();
```

Require submitted payload `{}` for package and release update tasks; keep the source selector for `release_check`.

- [ ] **Step 6: Verify UI RED, implement form ownership, and verify GREEN**

Run: `npm test -- --run src/app/SchedulesPage.test.tsx`
Expected RED: the old selector is still visible.

Remove package/source selection for instance update tasks, show the ownership note, summarize saved rows using instance configuration, and preserve explicit source selection for `release_check`. Rerun and expect PASS.

- [ ] **Step 7: Commit**

Commit: `feat(schedules): 按实例当前插件配置执行更新`

### Task 4: Integrated verification and merge readiness

**Files:**
- Create: `docs/aegis/work/2026-07-19-shared-game-value-port/50-evidence.md`

**Why this task exists:** Selective porting crosses API, metrics, automation, and UI boundaries and must prove the current mainline behavior remains intact.

**Impact / Compatibility:** No deployment or remote push occurs in this task. The dirty GOPROXY change in the older persistent-log worktree remains untouched.

**Verification:** Full repository matrix.

- [ ] **Step 1: Run full verification**

```powershell
$env:GOTMPDIR = Join-Path $env:TEMP 'l4d2-panel-gotmp'
go test -p 1 ./...
go vet ./...
Push-Location web
npm test -- --run
npm run build
Pop-Location
git diff --check
```

If Windows denies execution of only a random Go test binary, compile that package with `go test -c` to a fixed path and run it directly; record both outputs without claiming the original command exited zero.

- [ ] **Step 2: Record evidence and drift check**

Record exact commands, pass counts, environmental failures, compatibility conclusions, and remaining risks in the evidence file. Confirm no game-log files or old worktree changes were modified.

- [ ] **Step 3: Commit evidence**

Commit: `docs(aegis): 记录共享本体有效改动验证`
