# Persistent Game Log Viewer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为每个实例持久保存并高亮查看游戏与 SourceMod 日志，并通过可观察的后台任务按全局保留天数清理。

**Architecture:** `internal/gamelogs` 独占日志目录准备、迁移、安全读取和清理；Docker 容器把实例持久目录直接挂载到两个标准日志路径。HTTP 层只暴露认证后的文件树、尾部预览、下载、设置和排队接口，React 页面负责安全的 ANSI/语义高亮；自动与手动清理都进入既有持久任务流水。

**Tech Stack:** Go 1.24、SQLite、chi、现有 jobs/scheduler、React 19、TypeScript、Vitest、Playwright、`ansi_up`。

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-18-persistent-game-log-viewer-design.md`、`CONTEXT.md`、`docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md`、`docs/aegis/specs/2026-07-16-task-live-log-stream-design.md`。

**Compatibility Boundary:** 保持现有 OverlayFS、本体/插件/共享 VPK/私有覆盖所有权、实例任务串行、任务日志 API 和永久删除确认语义；只有两个标准日志目录改由 `instances/<id>/logs/` 持有。不得回退到易丢失的 Overlay 日志写入。

**Verification:** `go test ./...`、`go vet ./...`、`npm test -- --run`、`npm run build`，以及日志查看/重装保留/清理任务 Playwright 主流程。

---

## File ownership map

- `internal/gamelogs/manager.go`: 持久目录、旧日志迁移、安全文件树、尾部预览、下载定位和过期清理的唯一所有者。
- `internal/gamelogs/manager_test.go`: 文件系统与安全边界的单元测试。
- `internal/docker/lifecycle.go`: 仅负责把已准备好的两个持久目录加入容器挂载契约。
- `internal/lifecycle/service.go`: 在创建容器前调用日志目录准备/迁移，不实现日志文件操作。
- `internal/automation/dispatcher.go`: 把 `cleanup_game_logs` 任务接入既有实例 Job 管理器。
- `internal/store/job_history.go`: 普通设置 `game_log_retention_days` 的持久读写。
- `internal/httpapi/game_logs.go`: 日志读取、下载、保留设置和批量排队 HTTP 适配器。
- `internal/httpapi/server.go`: 只注册路由和注入依赖。
- `cmd/panel/main.go`: 组装 gamelogs manager、生命周期、dispatcher、HTTP 与每日调度器。
- `web/src/app/GameLogsPage.tsx`: 实例日志浏览主界面。
- `web/src/app/logHighlight.tsx`: ANSI 与日志语义 tokenizer/安全渲染。
- `web/src/app/App.tsx`: 导航、页面选择和设置表单集成。
- `web/src/styles/app.css`: 查看器与响应式布局。

### Task 1: Persistent directories, migration, and container mounts

**Files:**
- Create: `internal/gamelogs/manager.go`
- Create: `internal/gamelogs/manager_test.go`
- Modify: `internal/docker/lifecycle.go`
- Modify: `internal/docker/lifecycle_test.go`
- Modify: `internal/lifecycle/service.go`
- Modify: `internal/lifecycle/service_test.go`

**Why this task exists:**
- 保证日志在任何容器、游戏、插件或 Overlay 重建后仍位于实例持久目录。
- 保护现有实例升级时的旧日志，避免挂载切换造成不可见或覆盖。

**Impact / Compatibility:**
- `gamelogs.Manager.Prepare` 成为持久日志目录和迁移的唯一所有者。
- 旧 Overlay 标准日志目录在成功迁移后退役；不得保留运行时回退分支。

**Verification:** `go test ./internal/gamelogs ./internal/docker ./internal/lifecycle`

- [ ] **Step 1: Write failing migration and mount tests**

```go
func TestPrepareMigratesLegacyLogsWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "instances", "abc", "overlay", "merged", "left4dead2", "logs")
	requireWrite(t, filepath.Join(legacy, "L001.log"), "legacy")
	requireWrite(t, filepath.Join(root, "instances", "abc", "logs", "game", "L001.log"), "current")
	m := NewManager(root, Options{})
	if err := m.Prepare(context.Background(), "abc"); err != nil { t.Fatal(err) }
	entries, _ := os.ReadDir(filepath.Join(root, "instances", "abc", "logs", "game"))
	if len(entries) != 2 { t.Fatalf("want both logs, got %d", len(entries)) }
}

func TestBuildContainerSpecMountsPersistentLogs(t *testing.T) {
	spec, err := BuildContainerSpecWithGamePath("/data", "/data/instances/abc/overlay/merged", domain.Instance{ID: "abc", RuntimeImage: "runtime"})
	if err != nil { t.Fatal(err) }
	wants := []string{
		"/data/instances/abc/logs/game:/opt/l4d2/game/left4dead2/logs",
		"/data/instances/abc/logs/sourcemod:/opt/l4d2/game/left4dead2/addons/sourcemod/logs",
	}
	for _, want := range wants { if !slices.Contains(spec.Mounts, want) { t.Fatalf("missing %s", want) } }
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/gamelogs ./internal/docker ./internal/lifecycle`
Expected: FAIL because `gamelogs.NewManager`, `Prepare`, and log mounts do not exist.

- [ ] **Step 3: Implement minimal preparation and migration**

```go
type Options struct { Now func() time.Time }
type Manager struct { root string; now func() time.Time }

func (m *Manager) Prepare(ctx context.Context, instanceID string) error {
	base, err := m.instanceRoot(instanceID)
	if err != nil { return err }
	for _, kind := range []string{"game", "sourcemod"} {
		if err := os.MkdirAll(filepath.Join(base, kind), 0o750); err != nil { return err }
	}
	return m.migrateLegacy(ctx, instanceID)
}
```

Add the two bind mounts in `BuildContainerSpecWithGamePath`, inject a `LogPreparer` into lifecycle, and call `Prepare(ctx, id)` before `engine.Create`.

- [ ] **Step 4: Verify GREEN and migration idempotence**

Run: `go test ./internal/gamelogs ./internal/docker ./internal/lifecycle`
Expected: PASS, including a second `Prepare` call producing no new duplicate.

- [ ] **Step 5: Commit**

```powershell
git add internal/gamelogs internal/docker/lifecycle.go internal/docker/lifecycle_test.go internal/lifecycle/service.go internal/lifecycle/service_test.go
git commit -m "feat(logs): 持久挂载并迁移实例游戏日志"
```

### Task 2: Safe tree, tail preview, and download source

**Files:**
- Modify: `internal/gamelogs/manager.go`
- Modify: `internal/gamelogs/manager_test.go`

**Why this task exists:**
- 提供只读诊断能力，同时把访问范围严格限制在两个实例日志根。

**Impact / Compatibility:**
- 不复用 PrivateManager 的写接口；日志 API 永远不能修改文件。
- 预览读取末尾 10 MiB，下载仍读取完整普通文件。

**Verification:** `go test ./internal/gamelogs`

- [ ] **Step 1: Write failing read and safety tests**

```go
func TestTreePreviewAndResolveDownload(t *testing.T) {
	m := preparedManager(t)
	requireWrite(t, m.testPath("abc", "sourcemod", "errors/20260718.log"), "old\nERROR boom\n")
	tree, err := m.Tree(context.Background(), "abc")
	if err != nil || tree[1].Path != "errors/20260718.log" { t.Fatalf("tree=%v err=%v", tree, err) }
	preview, err := m.Preview(context.Background(), "abc", "sourcemod", "errors/20260718.log", 10)
	if err != nil || !preview.Truncated || preview.Text != "ROR boom\n" { t.Fatalf("preview=%+v err=%v", preview, err) }
	path, _, err := m.ResolveDownload("abc", "sourcemod", "errors/20260718.log")
	if err != nil || filepath.Base(path) != "20260718.log" { t.Fatal(err) }
}

func TestReadRejectsTraversalAndSymlinks(t *testing.T) {
	m := preparedManager(t)
	for _, path := range []string{"../secret", "/etc/passwd", "a/../../secret"} {
		if _, err := m.Preview(context.Background(), "abc", "game", path, 10); err == nil { t.Fatalf("accepted %q", path) }
	}
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/gamelogs`
Expected: FAIL because `Tree`, `Preview`, and `ResolveDownload` are undefined.

- [ ] **Step 3: Implement focused read contracts**

```go
type Entry struct { Kind, Path string; Size int64; ModifiedAt time.Time }
type Preview struct { Text string; Truncated bool; Size int64; ModifiedAt time.Time }

func (m *Manager) Preview(ctx context.Context, id, kind, relative string, limit int64) (Preview, error) {
	file, info, err := m.openRegular(id, kind, relative)
	if err != nil { return Preview{}, err }
	defer file.Close()
	start := max(int64(0), info.Size()-limit)
	if _, err := file.Seek(start, io.SeekStart); err != nil { return Preview{}, err }
	raw, err := io.ReadAll(io.LimitReader(file, limit))
	return Preview{Text: strings.ToValidUTF8(string(raw), "�"), Truncated: start > 0, Size: info.Size(), ModifiedAt: info.ModTime().UTC()}, err
}
```

Use `safepath` plus `Lstat`/opened-file `Stat` checks and stable sort by kind, directory, then path.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/gamelogs`
Expected: PASS for nested SourceMod directories, invalid kinds, traversal, symlinks, non-UTF-8 and rotation/not-found cases.

- [ ] **Step 5: Commit**

```powershell
git add internal/gamelogs
git commit -m "feat(logs): 增加安全日志树与尾部预览"
```

### Task 3: Retention setting and cleanup behavior

**Files:**
- Modify: `internal/gamelogs/manager.go`
- Modify: `internal/gamelogs/manager_test.go`
- Modify: `internal/store/job_history.go`
- Modify: `internal/store/store_test.go`

**Why this task exists:**
- 默认保留 14 天且全局可配置；清理必须有稳定、可测试的统计结果。

**Impact / Compatibility:**
- 设置使用现有普通 settings 表，不新增敏感设置或迁移列。
- 清理只删两个根下的过期普通文件，永不删除根目录。

**Verification:** `go test ./internal/gamelogs ./internal/store`

- [ ] **Step 1: Write failing settings and cleanup tests**

```go
func TestGameLogRetentionDaysDefaultsAndValidates(t *testing.T) {
	s := openStore(t)
	if got, _ := s.GameLogRetentionDays(); got != 14 { t.Fatalf("got %d", got) }
	if err := s.SetGameLogRetentionDays(30); err != nil { t.Fatal(err) }
	if err := s.SetGameLogRetentionDays(0); err == nil { t.Fatal("accepted zero") }
}

func TestCleanupDeletesExpiredFilesAndKeepsRoots(t *testing.T) {
	m := preparedManagerAt(t, time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC))
	old := requireDatedLog(t, m, "abc", "sourcemod", "errors/old.log", -15*24*time.Hour)
	requireDatedLog(t, m, "abc", "game", "new.log", -13*24*time.Hour)
	result, err := m.Cleanup(context.Background(), "abc", 14)
	if err != nil || result.Deleted != 1 || result.ReleasedBytes == 0 { t.Fatalf("result=%+v err=%v", result, err) }
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) { t.Fatal("old log remains") }
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/gamelogs ./internal/store`
Expected: FAIL because retention accessors and `Cleanup` do not exist.

- [ ] **Step 3: Implement minimal setting and cleanup result**

```go
const gameLogRetentionDaysKey = "game_log_retention_days"
const DefaultGameLogRetentionDays = 14

type CleanupResult struct { Scanned, Expired, Deleted, Skipped int; ReleasedBytes int64; Failures []string }
```

Implement `GameLogRetentionDays`, `SetGameLogRetentionDays(1..365)`, cutoff comparison using `ModTime().Before(cutoff)`, bottom-up empty-directory removal, and aggregate failures while continuing independent files.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/gamelogs ./internal/store`
Expected: PASS for exact cutoff, partial failure, special-file skip, empty subdirectory cleanup, root preservation, defaults and bounds.

- [ ] **Step 5: Commit**

```powershell
git add internal/gamelogs internal/store/job_history.go internal/store/store_test.go
git commit -m "feat(logs): 增加保留设置与过期清理"
```

### Task 4: Observable cleanup jobs and daily enqueue

**Files:**
- Modify: `internal/automation/dispatcher.go`
- Modify: `internal/automation/dispatcher_test.go`
- Modify: `internal/jobs/manager.go`
- Modify: `internal/jobs/manager_test.go`
- Create: `internal/gamelogs/scheduler.go`
- Create: `internal/gamelogs/scheduler_test.go`
- Modify: `cmd/panel/main.go`
- Modify: `cmd/panel/main_test.go`

**Why this task exists:**
- 所有自动和手动删除都必须在任务流水中可观察，包含统计和失败原因。

**Impact / Compatibility:**
- 使用现有实例 Job 串行与 joblogs，不在启动、计时器或 HTTP handler 中直接删文件。
- 同实例 pending/running 清理任务是唯一去重依据。

**Verification:** `go test ./internal/automation ./internal/jobs ./internal/gamelogs ./cmd/panel`

- [ ] **Step 1: Write failing dispatch, logging, and dedupe tests**

```go
func TestDispatchCleanupGameLogsReportsResult(t *testing.T) {
	cleaner := &fakeCleaner{result: gamelogs.CleanupResult{Scanned: 4, Deleted: 2, ReleasedBytes: 128}}
	d := Dispatcher{Jobs: manager, GameLogs: cleaner, Settings: fakeRetention(14)}
	if err := d.Dispatch(context.Background(), domain.ScheduledTask{Type: "cleanup_game_logs", InstanceID: "abc"}); err != nil { t.Fatal(err) }
	manager.Wait()
	assertJobLogContains(t, logs, "Scanned 4 files", "Deleted 2 files", "Released 128 bytes")
}

func TestEnqueueAllSkipsActiveCleanupForSameInstance(t *testing.T) {
	result := scheduler.EnqueueAll(context.Background())
	if result.Queued != 1 || result.Deduplicated != 1 { t.Fatalf("%+v", result) }
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/automation ./internal/jobs ./internal/gamelogs ./cmd/panel`
Expected: FAIL because the cleanup task and daily enqueuer are not wired.

- [ ] **Step 3: Implement job execution and scheduler**

```go
type EnqueueResult struct { Queued, Deduplicated, Failed int; Errors []string }

func (s *Scheduler) EnqueueAll(ctx context.Context) EnqueueResult {
	instances, err := s.instances.Instances(ctx)
	if err != nil { return EnqueueResult{Failed: 1, Errors: []string{err.Error()}} }
	// inspect active jobs, then submit one cleanup_game_logs job per eligible instance
}
```

Register a daily cron callback in panel startup and stop it during ordered shutdown. The dispatcher calls `Cleanup`, emits reporter progress/log messages, and returns an aggregate error when `Failures` is non-empty.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/automation ./internal/jobs ./internal/gamelogs ./cmd/panel`
Expected: PASS for success, partial failure, per-instance serialization, active-job dedupe, batch partial enqueue failure, daily registration and shutdown.

- [ ] **Step 5: Commit**

```powershell
git add internal/automation internal/jobs internal/gamelogs/scheduler.go internal/gamelogs/scheduler_test.go cmd/panel
git commit -m "feat(logs): 通过任务流水调度日志清理"
```

### Task 5: Authenticated log and retention HTTP APIs

**Files:**
- Create: `internal/httpapi/game_logs.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`
- Modify: `cmd/panel/main.go`

**Why this task exists:**
- 为实例页面提供受控只读访问，并让设置保存与任务排队结果可明确反馈。

**Impact / Compatibility:**
- 所有路由位于现有会话认证组；先验证实例存在，再访问日志。
- 设置提交与清理排队分开；调高保留期不排队，调低才排队。

**Verification:** `go test ./internal/httpapi ./cmd/panel`

- [ ] **Step 1: Write failing route contract tests**

```go
func TestGameLogTreePreviewDownloadAndRetentionSettings(t *testing.T) {
	assertAuthenticatedGET(t, s, "/api/instances/abc/game-logs/tree", http.StatusOK)
	assertAuthenticatedGET(t, s, "/api/instances/abc/game-logs/preview?kind=game&path=L001.log", http.StatusOK)
	assertAuthenticatedGET(t, s, "/api/instances/abc/game-logs/download?kind=game&path=L001.log", http.StatusOK)
	assertJSON(t, s, http.MethodGet, "/api/settings/game-logs", "", http.StatusOK, `"retention_days":14`)
	assertJSON(t, s, http.MethodPut, "/api/settings/game-logs", `{"retention_days":7}`, http.StatusOK, `"queued":1`)
	assertJSON(t, s, http.MethodPost, "/api/settings/game-logs/cleanup", `{}`, http.StatusAccepted, `"deduplicated":1`)
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/httpapi ./cmd/panel`
Expected: FAIL with 404 for the new routes.

- [ ] **Step 3: Implement handlers and dependency option**

```go
type gameLogSettingsResponse struct { RetentionDays int `json:"retention_days"`; Queued, Deduplicated, Failed int `json:"queued,omitempty"` }

func WithGameLogs(manager *gamelogs.Manager, scheduler *gamelogs.Scheduler) Option {
	return func(s *Server) { s.gameLogs, s.gameLogScheduler = manager, scheduler }
}
```

Use strict JSON decoding, 1..365 validation, URL query parsing, `http.ServeContent`/safe attachment headers, 404 for rotated files, 422 for invalid input, and 207-style JSON statistics with an appropriate non-2xx only when every enqueue fails.

- [ ] **Step 4: Verify GREEN**

Run: `go test ./internal/httpapi ./cmd/panel`
Expected: PASS for authentication, missing instance, traversal, invalid kind/path, preview metadata, download, 1/365 bounds, raise/lower behavior and batch results.

- [ ] **Step 5: Commit**

```powershell
git add internal/httpapi cmd/panel/main.go
git commit -m "feat(api): 开放实例日志查看与清理设置接口"
```

### Task 6: Safe syntax-highlighted React log viewer

**Files:**
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Create: `web/src/app/logHighlight.tsx`
- Create: `web/src/app/logHighlight.test.tsx`
- Create: `web/src/app/GameLogsPage.tsx`
- Create: `web/src/app/GameLogsPage.test.tsx`
- Modify: `web/src/api/client.ts`
- Modify: `web/src/styles/app.css`

**Why this task exists:**
- 让管理员快速定位时间、等级、玩家、网络地址和堆栈，同时确保不可信日志不能注入 HTML。

**Impact / Compatibility:**
- 引入 `ansi_up` 仅解析颜色；输出先转成受控 React token，禁止直接使用未净化的 `dangerouslySetInnerHTML`。
- 页面只读，不复用私有文件写操作。

**Verification:** `npm test -- --run src/app/logHighlight.test.tsx src/app/GameLogsPage.test.tsx && npm run build`

- [ ] **Step 1: Install dependency and write failing tokenizer/page tests**

Run: `npm install ansi_up` from `web/`.

```tsx
it("highlights semantic tokens without injecting HTML", () => {
  render(<HighlightedLog text={'\u001b[31mERROR\u001b[0m <img src=x> STEAM_1:0:42 127.0.0.1:27015'} />);
  expect(screen.getByText("ERROR")).toHaveClass("log-token-error");
  expect(screen.getByText("STEAM_1:0:42")).toHaveClass("log-token-steamid");
  expect(document.querySelector("img")).toBeNull();
});

it("loads nested SourceMod logs and reports a truncated tail", async () => {
  render(<GameLogsPage instanceID="abc" />);
  await user.click(await screen.findByRole("button", { name: /errors\/20260718.log/ }));
  expect(await screen.findByText("仅显示文件末尾 10 MiB")).toBeVisible();
  expect(screen.getByRole("link", { name: "下载原文件" })).toHaveAttribute("href", expect.stringContaining("kind=sourcemod"));
});
```

- [ ] **Step 2: Run and verify RED**

Run: `npm test -- --run src/app/logHighlight.test.tsx src/app/GameLogsPage.test.tsx`
Expected: FAIL because the components do not exist.

- [ ] **Step 3: Implement tokenizer and read-only page**

```tsx
export function HighlightedLog({ text }: { text: string }) {
  return <pre className="game-log-content">{tokenizeLog(text).map((token, index) => (
    <span className={`log-token-${token.kind}`} key={index}>{token.text}</span>
  ))}</pre>;
}
```

Implement stable regex precedence (ANSI, timestamp, level, SteamID, address, stack/module, plain), abortable tree/preview loads, nested accessible tree buttons, refresh, download URL, truncation/rotation/error states, and responsive CSS.

- [ ] **Step 4: Verify GREEN and production build**

Run: `npm test -- --run src/app/logHighlight.test.tsx src/app/GameLogsPage.test.tsx; npm run build`
Expected: both commands exit 0; tests cover ANSI reset, timestamps, levels, module/player/IP/stack, HTML text escaping, unknown-line fallback, nested tree and errors.

- [ ] **Step 5: Commit**

```powershell
git add web/package.json web/package-lock.json web/src/app/logHighlight.tsx web/src/app/logHighlight.test.tsx web/src/app/GameLogsPage.tsx web/src/app/GameLogsPage.test.tsx web/src/api/client.ts web/src/styles/app.css
git commit -m "feat(web): 增加高亮实例游戏日志查看器"
```

### Task 7: Navigation, settings UI, E2E, and documentation

**Files:**
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles/app.css`
- Modify: `web/e2e/panel.spec.ts`
- Modify: `cmd/e2e-fixture/main.go`
- Modify: `README.md`
- Create: `docs/aegis/work/2026-07-18-persistent-game-log-viewer/50-evidence.md`

**Why this task exists:**
- 完成管理员从实例导航到查看、重装验证、调整保留期并在任务流水确认清理的主旅程。

**Impact / Compatibility:**
- 保留现有导航和设置表单；新增页面必须在实例切换时清除旧实例选中文件。
- E2E fixture 只添加受控日志样本和时间，不绕过真实 HTTP/React 主路径。

**Verification:** 全量 Go、vet、前端测试、构建和目标 Playwright。

- [ ] **Step 1: Write failing navigation/settings and E2E tests**

```tsx
it("opens logs for the selected instance and queues cleanup", async () => {
  render(<App />);
  await user.click(await screen.findByRole("button", { name: "游戏日志" }));
  expect(await screen.findByRole("heading", { name: "游戏日志" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "系统设置" }));
  await user.clear(screen.getByLabelText("游戏日志保留天数"));
  await user.type(screen.getByLabelText("游戏日志保留天数"), "7");
  await user.click(screen.getByRole("button", { name: "保存日志设置" }));
  expect(await screen.findByText(/已为 .* 个实例排队/)).toBeVisible();
});
```

Playwright flow: open seeded SourceMod nested log, assert colored `ERROR`, download original, invoke fixture rebuild, assert log remains, age one log, click immediate cleanup, open recent tasks and assert `cleanup_game_logs` statistics, then assert old file disappeared and current file remains.

- [ ] **Step 2: Run and verify RED**

Run: `npm test -- --run src/app/App.test.tsx; npm run e2e -- --grep "persistent game logs"`
Expected: FAIL because navigation/settings integration and fixture endpoints are absent.

- [ ] **Step 3: Implement navigation, settings, fixture, and README**

```tsx
type Page = "overview" | "private" | "content" | "gamelogs" | "jobs" | "joblogs" | "schedules" | "settings";

{page === "gamelogs" && selectedInstance ? <GameLogsPage instanceID={selectedInstance.ID} /> : null}
```

Add the log navigation item, SettingsPage retention form and immediate-clean button with disabled/save/error/result states, fixture seed/age/rebuild support, and README persistence/retention documentation.

- [ ] **Step 4: Run target tests and E2E GREEN**

Run: `npm test -- --run src/app/App.test.tsx src/app/GameLogsPage.test.tsx src/app/logHighlight.test.tsx; npm run e2e -- --grep "persistent game logs"`
Expected: exit 0 and main journey verified through UI.

- [ ] **Step 5: Run full verification and record evidence**

Run from repository root:

```powershell
go test ./...
go vet ./...
Push-Location web
npm test -- --run
npm run build
npm run e2e -- --grep "persistent game logs"
Pop-Location
git diff --check
```

Expected: every command exits 0. Record exact commands, counts, residual risks, remove/restore check and confidence in `docs/aegis/work/2026-07-18-persistent-game-log-viewer/50-evidence.md`.

- [ ] **Step 6: Commit**

```powershell
git add web/src/app/App.tsx web/src/app/App.test.tsx web/src/styles/app.css web/e2e/panel.spec.ts cmd/e2e-fixture/main.go README.md docs/aegis/work/2026-07-18-persistent-game-log-viewer/50-evidence.md
git commit -m "feat(logs): 完成持久日志管理主流程"
```

## Repair Track

- **Root cause:** 运行时日志当前由 Overlay 写层隐式持有，重装/重置会让日志消失，且没有受控查看入口。
- **Canonical owner:** `internal/gamelogs` 与 `instances/<id>/logs/` 成为两个标准日志目录的唯一所有者。
- **Smallest repair:** 增加两个持久挂载、一次性幂等迁移、只读服务和任务化保留清理。
- **Compatibility:** 不改变其他游戏文件、私有覆盖、插件包、共享 VPK 或任务日志所有权。
- **Verification:** 重装前后文件哈希一致，过期日志只能通过清理任务或实例永久删除消失。

## Retirement Track

- **Retired owner:** 实例 Overlay upper/merged 中的 `left4dead2/logs` 与 `left4dead2/addons/sourcemod/logs` 不再是运行时日志持久所有者。
- **State after migration:** 成功迁移后容器挂载遮蔽旧路径；不保留复制同步或 Overlay 回退分支。
- **Retention reason:** 迁移代码继续保留，用于尚未升级的实例首次启动。
- **Future removal trigger:** 所有受支持部署版本均已记录完成日志目录迁移，且升级窗口结束后，可在独立迁移退役任务中删除兼容迁移读取。
- **Removal verification:** 在移除迁移代码前，用最老受支持数据布局执行升级测试并确认已有日志已转入持久目录。
