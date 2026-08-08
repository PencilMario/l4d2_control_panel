# 后台任务强制停止实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为执行中的后台任务增加确认式强制停止，并在取消后可靠记录为 `interrupted`。

**Architecture:** `jobs.Manager` 保存活动任务的取消函数与取消请求标记，统一决定任务终态；HTTP 新增认证取消路由；React 任务页只展示运行中停止按钮并复用现有操作列 CSS。取消请求不会提前伪造终态，任务函数返回后才释放实例锁并持久化中断事件。

**Tech Stack:** Go、SQLite 持久化接口、Chi HTTP 路由、React、TypeScript、Vitest、Testing Library、Vite。

**Baseline / Authority Refs:** `CONTEXT.md`; `README.md`; `docs/aegis/specs/2026-08-08-job-force-stop-design.md`; `internal/jobs/manager.go`; `internal/httpapi/server.go`; `web/src/app/JobsPage.tsx`; `web/src/styles/app.css`。

**Compatibility Boundary:** 保持既有任务启动、超时、实例串行锁、查询/SSE/日志接口和数据库 schema；只新增 `POST /api/jobs/{id}/cancel` 以及运行中任务的 UI 操作。

**Verification:** 每个任务先写失败测试并确认 RED；再运行目标 Go/Vitest 测试，随后运行 `go test ./...`、`go vet ./...`、`npm run build:web`、`git diff --check`，最后进行两台远端健康和功能检查。

---

### Task 1: Manager runtime cancellation

**Files:**
- Modify: `internal/jobs/manager.go`
- Test: `internal/jobs/manager_test.go`

**Why this task exists:** 只有 Manager 能同时看到任务上下文、实例锁和任务终态，取消逻辑必须由它拥有，避免 HTTP 层直接修改状态造成锁与持久化不一致。

**Impact / Compatibility:** 新增 `Manager.Cancel(id)` 和取消错误；既有成功、失败、超时和重启恢复路径保持原行为。取消期间仍保持 `running`，任务函数返回后才记录 `interrupted`。

**Repair Track:** 修复运行时没有取消 owner 的缺口；最小改动是保存 `context.CancelFunc`、识别 Manager 发起的取消并复用现有 `setStatus`。

**Retirement Track:** 重启恢复产生 `interrupted` 的 Store 逻辑继续保留，作为进程重启兜底；运行时不新增第二套状态写入路径。

**Verification:** `go test ./internal/jobs -run 'TestManager(Cancel|ForceStop)' -count=1`。

- [ ] **Step 1: Write the failing tests**

Add tests that start a blocking task, call `Cancel`, release it through `ctx.Done()`, and assert `Status == Interrupted`, the administrator stop message, and event kinds `queued, started, interrupted`. Add a second test where the task returns `nil` after cancellation and assert it still becomes `Interrupted`; add a test that `Cancel` rejects a pending task with `ErrJobNotRunning`.

- [ ] **Step 2: Run the target tests to verify RED**

Run `go test ./internal/jobs -run 'TestManager(Cancel|ForceStop)' -count=1`.

Expected: compile failure because `Manager.Cancel` and the cancellation errors do not exist.

- [ ] **Step 3: Implement the minimal Manager cancellation owner**

Add `cancels map[string]context.CancelFunc` and `cancelRequested map[string]bool` to `Manager`, initialize them in `NewManager`, expose `ErrJobNotFound` and `ErrJobNotRunning`, and implement:

```go
func (m *Manager) Cancel(id string) (Job, error) {
    m.mu.Lock()
    job, ok := m.jobs[id]
    if !ok {
        m.mu.Unlock()
        return Job{}, ErrJobNotFound
    }
    if job.Status != Running {
        m.mu.Unlock()
        return job, ErrJobNotRunning
    }
    cancel := m.cancels[id]
    m.cancelRequested[id] = true
    m.mu.Unlock()
    if cancel != nil {
        cancel()
    }
    return job, nil
}
```

Create a child cancel context at task start, register its cancel function before the goroutine begins, and pass that context through preflight and operation execution. Centralize the finish decision so `cancelRequested` selects `Interrupted` even if the task returns `nil`; use the existing `setStatus` to persist the terminal event and clean the control maps in a deferred goroutine cleanup. Preserve the existing timeout context and non-cancelled error paths.

- [ ] **Step 4: Run the target tests to verify GREEN**

Run `go test ./internal/jobs -run 'TestManager(Cancel|ForceStop)' -count=1`.

Expected: all new cancellation tests pass with no warnings.

- [ ] **Step 5: Run related Manager regression tests**

Run `go test ./internal/jobs -count=1`.

Expected: the complete existing and new Manager suite passes.

### Task 2: Authenticated HTTP cancel contract

**Files:**
- Modify: `internal/httpapi/server.go`
- Test: `internal/httpapi/server_test.go`

**Why this task exists:** The browser needs an authenticated command that asks the canonical Manager to cancel a task and reports races explicitly.

**Impact / Compatibility:** Add only `POST /api/jobs/{id}/cancel`; existing GET job APIs, auth, audit middleware, and response shapes remain unchanged.

**Repair Track:** Add the missing API owner while keeping status transitions in Manager.

**Retirement Track:** No old cancel endpoint exists; no fallback status mutation is added.

**Verification:** `go test ./internal/httpapi -run 'TestJobCancellation' -count=1`.

- [ ] **Step 1: Write the failing HTTP tests**

Add an authenticated integration test with a real `jobs.Manager` task blocked on its context. POST `/api/jobs/{id}/cancel`, assert HTTP `202`, then wait for the task and assert the persistent detail contains `interrupted`. Add not-found and non-running assertions for `404 job_not_found` and `409 job_not_running`.

- [ ] **Step 2: Run the target tests to verify RED**

Run `go test ./internal/httpapi -run 'TestJobCancellation' -count=1`.

Expected: route requests return `404` because no POST cancel route exists.

- [ ] **Step 3: Implement the route and handler**

Register `r.Post("/api/jobs/{id}/cancel", s.cancelJob)` in the authenticated group. Implement `cancelJob` by calling `s.jobs.Cancel(chi.URLParam(r, "id"))`, mapping `jobs.ErrJobNotFound` to `404 job_not_found`, `jobs.ErrJobNotRunning` to `409 job_not_running`, other errors to `500 jobs_error`, and returning the job with status `202` on success. Do not decode a request body.

- [ ] **Step 4: Run the target tests to verify GREEN**

Run `go test ./internal/httpapi -run 'TestJobCancellation' -count=1`.

Expected: authenticated success and error contract tests pass.

- [ ] **Step 5: Run related HTTP regression tests**

Run `go test ./internal/httpapi -count=1`.

Expected: all HTTP API tests pass.

### Task 3: Jobs page control and preserved styling

**Files:**
- Modify: `web/src/app/JobsPage.tsx`
- Test: `web/src/app/JobsPage.test.tsx`
- Modify: `web/src/styles/app.css`

**Why this task exists:** The operator needs a visible, guarded action in the existing task table without changing the established dense layout or responsive behavior.

**Impact / Compatibility:** Only running rows gain the button. Existing event and full-log controls, SSE refresh, filters, and mobile grid continue to work.

**Verification:** `npm test -- --run src/app/JobsPage.test.tsx` and `npm run build:web` from `web`.

- [ ] **Step 1: Write the failing component tests**

Add tests that assert: a running task renders an accessible “强制停止” button; a pending and a terminal task do not; `window.confirm` returning `false` makes no POST; returning `true` posts to `/api/jobs/{id}/cancel` with `POST` and disables the button while the request is pending. Keep existing event/log button assertions.

- [ ] **Step 2: Run the target tests to verify RED**

Run `npm test -- --run src/app/JobsPage.test.tsx`.

Expected: the new running-task button query fails because the component has no cancel control.

- [ ] **Step 3: Implement the minimal UI behavior**

Add `cancelingID` state and a `cancelJob` callback using `api<Job>(
`/api/jobs/${item.ID}/cancel`, { method: "POST" })`. Render the button only for `item.Status === "running"`, use a Lucide stop icon, `aria-busy`, and a Chinese confirmation message. On success retain the current item snapshot until SSE or detail refresh supplies the terminal state; on failure set the existing `jobsError` and clear the busy state. Do not change table columns or existing operation button classes.

- [ ] **Step 4: Add narrow CSS without changing the base layout**

Add `.job-force-stop` and its hover/disabled rules adjacent to `.job-operation` styles, using the existing danger palette and button dimensions. Do not change `.job-row` grid tracks or the mobile `.job-operation` placement. Ensure the icon has the same 12px sizing as sibling operation icons.

- [ ] **Step 5: Run the target tests to verify GREEN**

Run `npm test -- --run src/app/JobsPage.test.tsx`.

Expected: all JobsPage tests, including new cancel interaction tests, pass.

- [ ] **Step 6: Build the frontend**

Run `npm run build:web`.

Expected: TypeScript and Vite both exit with status 0.

### Task 4: Full verification and two-node deployment

**Files:**
- Modify: `docs/aegis/work/2026-08-08-job-force-stop/50-evidence.md`

**Why this task exists:** The user requested the change be live on both named SSH hosts, so local checks and remote health checks must be recorded together.

**Impact / Compatibility:** Deployment uses the repository's `deploy.sh` fast-forward/update-and-compose workflow on SSH aliases `琥珀` and `安可服`; no persistent proxy configuration is added.

**Verification:** Go full suite, vet, frontend target/build, diff check, then remote deployment and `/api/health`/Compose checks on both hosts.

- [ ] **Step 1: Run local full verification**

Run from the worktree root:

```powershell
go test ./...
go vet ./...
git diff --check
```

Run from `web`:

```powershell
npm test -- --run
npm run build:web
```

- [ ] **Step 2: Commit the implementation branch**

Run `git status --short`, review the diff, then commit the implementation with a Conventional Commit message such as `feat(jobs): add force stop for running tasks`.

- [ ] **Step 3: Push the branch and update both remotes**

Push the verified branch to `origin`, then use the repository deployment workflow through SSH aliases. The remote command must run `sudo bash /opt/l4d2-control-panel/deploy.sh` (or the existing installed path discovered by a read-only check) and must not set a persistent proxy.

- [ ] **Step 4: Verify both production nodes**

For each host, run read-only checks equivalent to:

```sh
cd /opt/l4d2-control-panel
sudo docker compose --env-file .env ps
curl --fail http://127.0.0.1:${L4D2_PANEL_HTTP_PORT:-18081}/api/health
```

Record the deployed revision, health response, Compose service state, and any unverified browser-level detail in `50-evidence.md`.
