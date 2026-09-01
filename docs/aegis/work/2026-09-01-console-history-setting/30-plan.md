# 游戏实例控制台缓存设置实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 增加默认 8192、范围 1～1,000,000 且保存后立即生效的控制台缓存行数设置，并让后端与前端共享该设置。

**Architecture:** 复用 SQLite `system_settings` 键值表，新增 `/api/settings/console` 认证接口。`consoleHub` 持有动态后端上限并在更新时裁剪现有会话；React App 统一加载并保存共享前端上限，SettingsPage 通过回调更新 App，保存后通过 prop 更新 Terminal，避免 WebSocket 重连和旧读取覆盖新值。

**Tech Stack:** Go `database/sql`/`testing`、Chi HTTP、React/TypeScript、Vitest、Vite。

**Baseline / Authority Refs:** `CONTEXT.md`；`docs/aegis/work/2026-09-01-console-history-setting/20-spec.md`；`internal/store/job_history.go`；`internal/httpapi/server.go`；`internal/httpapi/console.go`；`web/src/app/App.tsx`；现有控制台导出实现。

**Compatibility Boundary:** 保持 `/api/instances/{id}/console` WebSocket 地址和文本/二进制输入、实时输出、实例隔离、已有 1 MiB 后端历史字节上限及 TXT 导出行为；旧数据库缺失设置值时回落 8192。

**Verification:** 每个生产切片先写失败测试并确认 RED，再写最小实现确认 GREEN；最终运行 Store/HTTP/Console Go 测试、前端控制台与 App 测试、Go 全量、前端全量和生产构建，并检查 diff 边界。

---

### Task 1: 持久化控制台缓存设置

**Files:**
- Create: `internal/store/console_settings.go`
- Test: `internal/store/console_settings_test.go`

**Why this task exists:** 设置必须在 Panel 重启后保留，并对旧数据库安全提供 8192 默认值。

**Impact / Compatibility:** 复用现有 `system_settings` 表；不改变既有设置键或迁移顺序。

**Verification:** 先运行新增 Store 测试确认缺少方法而 RED；实现后确认默认值、边界值、重开持久化和非法值不改变已保存值。

- [ ] **Step 1: 写失败测试**

```go
func TestConsoleHistoryLinesDefaultsPersistsAndRejectsInvalidValues(t *testing.T) {
  path := filepath.Join(t.TempDir(), "panel.db")
  s, err := Open(path)
  if err != nil { t.Fatal(err) }
  got, err := s.ConsoleHistoryLines()
  if err != nil || got != DefaultConsoleHistoryLines { t.Fatalf("default=%d err=%v", got, err) }
  if err := s.SetConsoleHistoryLines(MinConsoleHistoryLines); err != nil { t.Fatal(err) }
  if got, err = s.ConsoleHistoryLines(); err != nil || got != MinConsoleHistoryLines { t.Fatalf("min=%d err=%v", got, err) }
  if err := s.SetConsoleHistoryLines(MaxConsoleHistoryLines); err != nil { t.Fatal(err) }
  if got, err = s.ConsoleHistoryLines(); err != nil || got != MaxConsoleHistoryLines { t.Fatalf("max=%d err=%v", got, err) }
  for _, invalid := range []int{0, -1, MaxConsoleHistoryLines + 1} {
    if err := s.SetConsoleHistoryLines(invalid); err == nil { t.Fatalf("expected %d rejected", invalid) }
    if got, err := s.ConsoleHistoryLines(); err != nil || got != MaxConsoleHistoryLines { t.Fatalf("invalid %d changed value=%d err=%v", invalid, got, err) }
  }
  if err := s.Close(); err != nil { t.Fatal(err) }
  s, err = Open(path)
  if err != nil { t.Fatal(err) }
  defer s.Close()
  if got, err := s.ConsoleHistoryLines(); err != nil || got != MaxConsoleHistoryLines { t.Fatalf("reopen=%d err=%v", got, err) }
}
```

- [ ] **Step 2: 运行 RED**

Run: `go test ./internal/store -run TestConsoleHistoryLinesDefaultsPersistsAndRejectsInvalidValues -count=1`

Expected: FAIL because the new Store constants/methods do not exist.

- [ ] **Step 3: 写最小实现**

Add `DefaultConsoleHistoryLines = 8192`, `MinConsoleHistoryLines = 1`, `MaxConsoleHistoryLines = 1000000`, key `console_history_lines`, and Store methods that read an integer from `system_settings`, return the default on `sql.ErrNoRows`, validate bounds, and upsert the decimal value with the existing timestamp format.

- [ ] **Step 4: 运行 GREEN**

Run: `gofmt -w internal/store/console_settings.go internal/store/console_settings_test.go; go test ./internal/store -run TestConsoleHistoryLinesDefaultsPersistsAndRejectsInvalidValues -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the slice**

```sh
git add internal/store/console_settings.go internal/store/console_settings_test.go
git commit -m "feat: persist console history limit"
```

### Task 2: 后端动态缓存上限和设置 API

**Files:**
- Modify: `internal/httpapi/console.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/console_test.go`
- Modify: `internal/httpapi/server_test.go`

**Why this task exists:** 后端必须按管理员选择保留历史，并在保存后立即裁剪当前会话，供新订阅恢复正确历史。

**Impact / Compatibility:** 默认行为继续是 8192；后端 1 MiB 字节上限、WebSocket 路径和订阅流程不变。

**Repair Track:** 静态行数常量是当前后端历史的唯一 owner；改为 Hub 中受锁保护的动态行数，并让设置接口成为更新入口。

**Retirement Track:** 旧的静态 `maxConsoleHistoryLines` 行为退出；保留默认常量作为新 Hub 初始值，不保留旧值 fallback。

- [ ] **Step 1: 写失败测试**

在 Console 测试中增加自定义上限和 Hub 更新后裁剪测试；在 Server 测试中增加 GET 默认、PUT 合法值和非法值不覆盖原值测试。核心断言形态：

```go
hub := newConsoleHub(nil)
session := &consoleSession{instanceID: "instance", subscribers: map[*consoleSubscriber]struct{}{}}
hub.sessions[session.instanceID] = session
hub.publish(session, []byte("one\ntwo\nthree\n"))
hub.setHistoryLines(2)
if got := string(session.history); got != "two\nthree\n" { t.Fatalf("history=%q", got) }
```

- [ ] **Step 2: 运行 RED**

Run: `go test ./internal/httpapi -run 'TestConsoleHistory|TestConsoleSettings' -count=1`

Expected: FAIL because dynamic Hub setter/API route are absent.

- [ ] **Step 3: 写最小实现**

Use a `historyLines` field initialized to `store.DefaultConsoleHistoryLines`; extract the existing append logic into a helper receiving a line limit; make `publish` use the current limit and add a setter that trims every session under `h.mu`. Load the Store value during `New`, add authenticated GET/PUT routes, decode an exact `history_lines` object, validate 1～1,000,000, save it, update the Hub, and return JSON.

- [ ] **Step 4: 运行 GREEN**

Run: `gofmt -w internal/httpapi/console.go internal/httpapi/server.go internal/httpapi/console_test.go internal/httpapi/server_test.go; go test ./internal/httpapi -run 'TestConsoleHistory|TestConsoleSettings' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the slice**

```sh
git add internal/httpapi/console.go internal/httpapi/server.go internal/httpapi/console_test.go internal/httpapi/server_test.go
git commit -m "feat: expose console history settings"
```

### Task 3: 前端共享设置、即时裁剪和系统设置表单

**Files:**
- Modify: `web/src/app/consoleBuffer.ts`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/consoleBuffer.test.ts`
- Modify: `web/src/app/App.test.tsx`

**Why this task exists:** 当前页面的显示缓存也必须遵守后端选择，并且保存设置时立即更新打开的控制台。

**Impact / Compatibility:** 不重连 WebSocket；已有导出按钮继续导出当前裁剪后的文本，默认无配置时仍为 8192。

- [ ] **Step 1: 写失败测试**

增加 `trimConsoleOutput` 的边界测试，以及设置页加载/保存测试：输入标记为 `控制台缓存行数`，属性 `min="1"`、`max="1000000"`、默认值 8192，保存请求 body 为 `{history_lines: 1000000}`。在控制台集成测试中打开 Terminal、写入多行、通过设置页保存 `1`，断言当前 `<pre>` 立即只剩最后一行。

- [ ] **Step 2: 运行 RED**

Run: `npm test -- --run src/app/consoleBuffer.test.ts src/app/App.test.tsx` (workdir `web`)

Expected: FAIL because no trim helper, shared prop, input, or API call exists.

- [ ] **Step 3: 写最小实现**

Export frontend min/max/default constants and a trim helper; App loads `/api/settings/console` after authentication, gates Terminal until the response is valid, falls back to the default with an error notice on failure, and owns the save request sequence. App passes the current limit and save callback to SettingsPage and Terminal. Terminal stores the latest limit in a ref, trims output when the prop changes, and applies the ref to incoming frames. SettingsPage renders the numeric setting card, validates and saves through the App callback, and restores the confirmed value on error.

- [ ] **Step 4: 运行 GREEN**

Run: `npm test -- --run src/app/consoleBuffer.test.ts src/app/App.test.tsx` (workdir `web`)

Expected: PASS.

- [ ] **Step 5: Commit the slice**

```sh
git add web/src/app/consoleBuffer.ts web/src/app/App.tsx web/src/app/consoleBuffer.test.ts web/src/app/App.test.tsx
git commit -m "feat: configure console history in settings"
```

### Task 4: 全量验证、文档证据和部署

**Files:**
- Modify: `docs/aegis/work/2026-09-01-console-history-setting/50-evidence.md`

**Why this task exists:** 跨 Go/React/API 的用户可见改动需要完整回归，并按用户先前授权更新安可部署实例。

**Impact / Compatibility:** 验证不修改根工作区原有加速器改动和日志目录；部署只更新已验证的本分支代码对应部署副本。

- [ ] **Step 1: 运行控制台/设置相关 Go 测试**

Run: `go test ./internal/store ./internal/httpapi -count=1`

Expected: exit code 0.

- [ ] **Step 2: 运行 Go 全量测试**

Run: `go test ./... -count=1`

Expected: exit code 0.

- [ ] **Step 3: 运行前端全量测试**

Run: `npm test -- --run` (workdir `web`)

Expected: exit code 0.

- [ ] **Step 4: 运行前端生产构建**

Run: `npm run build` (workdir `web`)

Expected: exit code 0.

- [ ] **Step 5: 审查差异边界**

Run: `git diff --check; git status --short --branch; git diff --stat HEAD~3..HEAD`

Expected: no whitespace errors; only task commits plus task-scoped docs are changed in the isolated worktree.

- [ ] **Step 6: 通过 SSH 更新安可部署实例并检查健康状态**

Use the repository's deployment procedure and the configured SSH target from the SSH skill/environment. Before mutating the remote, record remote commit/service state; update only the deployment checkout, rebuild/restart through its documented script, then verify `/api/health` and container status. If target or credentials are unavailable, report the exact blocker and do not guess a host.

- [ ] **Step 7: 写证据并提交**

Record exact commands, exit statuses, deployment target, remote health output, unchanged local-root user files, residual risks, and Repair/Retirement tracks in `50-evidence.md`, then commit the evidence file.
