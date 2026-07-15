# Recent Job Logs and Retention Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** 为最近任务增加可展开的结构化日志、准确的执行时间，以及可在系统设置调整的全局成功任务保留上限。

**Architecture:** SQLite 使用独立 job_events 追加表，并在 jobs 保存 StartedAt/FinishedAt 快照；Store 通过事务同步写任务和事件，Manager 负责生成生命周期事件。摘要和 SSE 不携带事件，React 只在展开行时按需读取详情。

**Tech Stack:** Go 1.24、modernc SQLite、chi HTTP、React、TypeScript、Vitest、Testing Library、Playwright。

**Baseline / Authority Refs:** docs/aegis/specs/2026-07-16-recent-job-logs-retention-design.md、CONTEXT.md、docs/aegis/work/2026-07-16-recent-job-logs-retention/10-baseline-readset.md。

**Compatibility Boundary:** 保留现有任务 JSON 顶层字段、GET /api/jobs/{id} 轮询、jobs SSE 事件名、每实例串行执行和重启恢复；新增字段均为加法式扩展，清理只删除超额成功任务。

**Verification:** go test ./...、go vet ./...、cd web && npm test -- --run、cd web && npm run build、cd web && npm run e2e。

---

### Task 1: SQLite 任务事件、时间与保留设置

**Files:**
- Modify: internal/domain/models.go
- Modify: internal/store/store.go
- Create: internal/store/job_history.go
- Modify: internal/store/store_test.go

**Why this task exists:**
- 为任务日志、执行时间和成功任务清理建立唯一持久化所有者。
- 保护旧数据库升级、事件顺序和级联删除。

**Impact / Compatibility:**
- jobs 原字段和 SaveJob 保留，现有夹具仍可写入最终快照。
- 新迁移版本为 6；版本 1 至 5 的迁移顺序不变。

**Repair Track:**
- 根因是 jobs 单行覆盖最后进度，无法还原任务历史；Store 继续作为规范持久化所有者。
- 最小修复是时间列、job_events、普通设置和事务写入，不改变任务调度。

**Retirement Track:**
- SaveJob 为旧夹具和迁移兼容保留，但生产 Manager 不再用它记录生命周期。
- “最后 Message 等同完整日志”的旧假设在 Manager 全部改用 SaveJobWithEvent 后退休。

**Verification:**
- go test ./internal/store -run "Test(JobHistoryMigration|SaveJobWithEvent|SuccessfulJobLimit|RecoverJobs)" -count=1

- [ ] **Step 1: 写失败的领域模型和迁移测试**

在 internal/store/store_test.go 增加旧 jobs 表数据库夹具，并断言 Open 后存在 StartedAt、FinishedAt 和一条 synthetic 事件：

~~~go
func TestJobHistoryMigrationBackfillsLegacySnapshot(t *testing.T) {
    path := filepath.Join(t.TempDir(), "panel.db")
    createLegacyJobsDatabase(t, path, domain.JobRecord{
        ID: "legacy-failed", Status: "failed", Error: "legacy failure",
        CreatedAt: time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC),
        UpdatedAt: time.Date(2026, 7, 16, 1, 2, 0, 0, time.UTC),
    })
    db, err := Open(path)
    if err != nil { t.Fatal(err) }
    defer db.Close()
    job, found, err := db.LoadJob("legacy-failed")
    if err != nil || !found || job.StartedAt == nil || job.FinishedAt == nil {
        t.Fatalf("job=%#v found=%v err=%v", job, found, err)
    }
    events, err := db.JobEvents("legacy-failed")
    if err != nil || len(events) != 1 || events[0].Kind != "snapshot" {
        t.Fatalf("events=%#v err=%v", events, err)
    }
}

func createLegacyJobsDatabase(t *testing.T, path string, record domain.JobRecord) {
    t.Helper()
    db, err := sql.Open("sqlite", path)
    if err != nil { t.Fatal(err) }
    defer db.Close()
    _, err = db.Exec("CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL); CREATE TABLE jobs (id TEXT PRIMARY KEY, instance_id TEXT NOT NULL, type TEXT NOT NULL, status TEXT NOT NULL, stage TEXT NOT NULL DEFAULT '', percent INTEGER NOT NULL DEFAULT 0, message TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)")
    if err != nil { t.Fatal(err) }
    _, err = db.Exec("INSERT INTO jobs(id,instance_id,type,status,stage,percent,message,error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)", record.ID, record.InstanceID, record.Type, record.Status, record.Stage, record.Percent, record.Message, record.Error, record.CreatedAt.Format(time.RFC3339Nano), record.UpdatedAt.Format(time.RFC3339Nano))
    if err != nil { t.Fatal(err) }
}
~~~

- [ ] **Step 2: 运行迁移测试并确认红灯**

Run: go test ./internal/store -run TestJobHistoryMigrationBackfillsLegacySnapshot -count=1

Expected: FAIL，因为 JobRecord 尚无 StartedAt/FinishedAt，Store 尚无 JobEvents。

- [ ] **Step 3: 增加模型、版本 6 迁移与事件事务**

在 internal/domain/models.go 定义：

~~~go
type JobRecord struct {
    ID, InstanceID, Type, Stage, Message, Status, Error string
    Percent int
    CreatedAt, UpdatedAt time.Time
    StartedAt, FinishedAt *time.Time
}

type JobEvent struct {
    ID int64
    JobID, Kind, Stage, Message string
    Percent int
    CreatedAt time.Time
}
~~~

在 internal/store/job_history.go 实现 migrateJobHistory、SaveJobWithEvent、JobEvents、SuccessfulJobLimit、SetSuccessfulJobLimit 和 PruneSuccessfulJobs。设置常量固定为：

~~~go
const (
    successfulJobLimitKey = "successful_job_limit"
    DefaultSuccessfulJobLimit = 25
    MinSuccessfulJobLimit = 1
    MaxSuccessfulJobLimit = 500
)
~~~

SaveJobWithEvent 在单事务内 upsert jobs 并 insert job_events。SetSuccessfulJobLimit 在单事务内 upsert system_settings 并按 FinishedAt/UpdatedAt、ID 稳定排序删除超额 succeeded 任务。

- [ ] **Step 4: 增加设置、事务、顺序、清理与恢复失败测试**

覆盖默认 25、边界 1/500、越界拒绝、保留最新成功任务、失败/运行/中断不删除、事件级联删除、相同时间稳定排序，以及 RecoverJobs 写 FinishedAt 和 interrupted 事件。

~~~go
func TestSuccessfulJobLimitPrunesOnlyOldestSucceeded(t *testing.T) {
    db, err := Open(filepath.Join(t.TempDir(), "panel.db"))
    if err != nil { t.Fatal(err) }
    defer db.Close()
    seedJobStatuses(t, db, []string{"succeeded", "succeeded", "failed", "interrupted"})
    if err := db.SetSuccessfulJobLimit(1); err != nil { t.Fatal(err) }
    assertJobExists(t, db, "newest-succeeded", true)
    assertJobExists(t, db, "oldest-succeeded", false)
    assertJobExists(t, db, "failed", true)
    assertJobExists(t, db, "interrupted", true)
}

func assertJobExists(t *testing.T, db *Store, id string, want bool) {
    t.Helper()
    _, found, err := db.LoadJob(id)
    if err != nil || found != want {
        t.Fatalf("job %s found=%v want=%v err=%v", id, found, want, err)
    }
}
~~~

seedJobStatuses 使用 SaveJob 写入 ID 为 newest-succeeded、oldest-succeeded、failed 和 interrupted 的四个确定性记录，并为两个成功记录设置相隔一分钟的 FinishedAt。

- [ ] **Step 5: 运行 Store 目标测试并确认绿灯**

Run: go test ./internal/store -run "Test(JobHistoryMigration|SaveJobWithEvent|SuccessfulJobLimit|RecoverJobs)" -count=1

Expected: PASS。

- [ ] **Step 6: 提交持久化切片**

~~~powershell
git add internal/domain/models.go internal/store/store.go internal/store/job_history.go internal/store/store_test.go
git commit -m "feat(store): 持久化任务事件与成功记录上限"
~~~

### Task 2: Job Manager 生命周期事件

**Files:**
- Modify: internal/jobs/manager.go
- Modify: internal/jobs/manager_test.go

**Why this task exists:**
- 把任务排队、开始、进度和终态转换为可展开的结构化日志。
- 准确区分排队时间与实际执行时间。

**Impact / Compatibility:**
- Reporter.Progress 签名和每实例锁保持不变。
- Get 继续返回 Job 快照；新增 Details 提供事件。

**Verification:**
- go test ./internal/jobs -run "Test(PersistentManagerRecordsLifecycleEvents|PersistentManagerReloadsCompletedJob|PersistentManagerMarksStaleRunningJobInterrupted|StartReturnsInitialPersistenceFailure)" -count=1

- [ ] **Step 1: 写失败的生命周期事件测试**

~~~go
func TestPersistentManagerRecordsLifecycleEvents(t *testing.T) {
    db, _ := store.Open(filepath.Join(t.TempDir(), "panel.db"))
    defer db.Close()
    manager := NewPersistentManager(db)
    created, err := manager.Start(context.Background(), "a", "install", func(_ context.Context, r Reporter) error {
        r.Progress("download", 40, "downloading")
        return errors.New("download interrupted")
    })
    if err != nil { t.Fatal(err) }
    wait(t, manager, created.ID)
    job, events, ok := manager.Details(created.ID)
    if !ok || job.StartedAt == nil || job.FinishedAt == nil {
        t.Fatalf("job=%#v ok=%v", job, ok)
    }
    assertEventKinds(t, events, "queued", "started", "progress", "failed")
}

func assertEventKinds(t *testing.T, events []domain.JobEvent, wants ...string) {
    t.Helper()
    if len(events) != len(wants) { t.Fatalf("events=%#v", events) }
    for index, want := range wants {
        if events[index].Kind != want {
            t.Fatalf("event %d kind=%q want=%q", index, events[index].Kind, want)
        }
    }
}
~~~

- [ ] **Step 2: 运行测试并确认红灯**

Run: go test ./internal/jobs -run TestPersistentManagerRecordsLifecycleEvents -count=1

Expected: FAIL，因为 Details、生命周期事件和开始/结束时间尚不存在。

- [ ] **Step 3: 最小实现事件生成和详情读取**

Manager 增加内存 events map；持久 Repository 使用以下契约：

~~~go
type Job struct {
    ID, InstanceID, Type, Stage, Message string
    Status Status
    Percent int
    Error string
    CreatedAt, UpdatedAt time.Time
    StartedAt, FinishedAt *time.Time
}

type Repository interface {
    SaveJobWithEvent(domain.JobRecord, domain.JobEvent) error
    LoadJob(string) (domain.JobRecord, bool, error)
    JobEvents(string) ([]domain.JobEvent, error)
}
~~~

Start 写 queued；获得锁后写 started 和 StartedAt；Progress 写 progress；终态写 FinishedAt 和 succeeded/failed。Details 优先返回内存事件，重启后从 Repository 加载。

- [ ] **Step 4: 接入成功后清理并更新失败 Repository 测试桩**

通过可选接口调用 Store.PruneSuccessfulJobs；清理错误不覆盖真实任务结果。初始 queued 事务失败继续阻止业务函数运行。

~~~go
type successfulJobPruner interface {
    PruneSuccessfulJobs() error
}

if status == Succeeded {
    if pruner, ok := m.repo.(successfulJobPruner); ok {
        if err := pruner.PruneSuccessfulJobs(); err != nil {
            log.Printf("prune successful jobs: %v", err)
        }
    }
}
~~~

- [ ] **Step 5: 运行 Manager 包测试**

Run: go test ./internal/jobs -count=1

Expected: PASS。

- [ ] **Step 6: 提交 Manager 切片**

~~~powershell
git add internal/jobs/manager.go internal/jobs/manager_test.go
git commit -m "feat(jobs): 记录任务结构化生命周期日志"
~~~

### Task 3: 任务详情与保留设置 HTTP 契约

**Files:**
- Modify: internal/httpapi/server.go
- Modify: internal/httpapi/server_test.go

**Why this task exists:**
- 让 UI 按需读取日志并持久调整成功任务保留数量。
- 保持摘要和 SSE 轻量。

**Impact / Compatibility:**
- GET /api/jobs/{id} 仍返回原顶层字段，只新增 Events。
- GET /api/jobs 和 jobs SSE 不返回 Events。

**Repair Track:**
- 详情接口当前只重复返回摘要快照；getJob 仍是规范路由，但改为从 Manager.Details 组合事件。
- 设置由 Store 直接拥有，HTTP 只做认证、严格解码、范围校验和错误映射。

**Retirement Track:**
- 不新增第二个日志路由；旧 GET /api/jobs/{id} 继续存在并以加法字段收敛。
- 摘要和 SSE 携带完整日志的候选方案明确退休，防止每秒响应体增长。

**Verification:**
- go test ./internal/httpapi -run "Test(JobDetailIncludesEvents|JobSummariesExcludeEvents|JobSettings)" -count=1

- [ ] **Step 1: 写失败的详情和设置路由测试**

~~~go
func TestJobDetailIncludesEventsAndSummaryDoesNot(t *testing.T) {
    server, db := testServer(t)
    defer db.Close()
    cookie := loginCookie(t, server)
    now := time.Now().UTC()
    err := db.SaveJobWithEvent(
        domain.JobRecord{ID: "job-1", Status: "failed", Error: "boom", CreatedAt: now, UpdatedAt: now},
        domain.JobEvent{JobID: "job-1", Kind: "failed", Message: "boom", CreatedAt: now},
    )
    if err != nil { t.Fatal(err) }
    detailRequest := httptest.NewRequest(http.MethodGet, "/api/jobs/job-1", nil)
    detailRequest.AddCookie(cookie)
    detail := httptest.NewRecorder()
    server.Handler().ServeHTTP(detail, detailRequest)
    if !strings.Contains(detail.Body.String(), "\"Events\"") { t.Fatal(detail.Body.String()) }
    summaryRequest := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
    summaryRequest.AddCookie(cookie)
    summary := httptest.NewRecorder()
    server.Handler().ServeHTTP(summary, summaryRequest)
    if strings.Contains(summary.Body.String(), "\"Events\"") { t.Fatal(summary.Body.String()) }
}
~~~

设置测试覆盖 GET 默认 25、PUT 1/500 成功、0/501/非整数返回 422，以及调低后立即只保留指定数量的成功任务。

- [ ] **Step 2: 运行 HTTP 目标测试并确认红灯**

Run: go test ./internal/httpapi -run "Test(JobDetailIncludesEvents|JobSettings)" -count=1

Expected: FAIL，因为详情无 Events，设置路由返回 404。

- [ ] **Step 3: 实现加法式详情响应和设置路由**

~~~go
type jobDetail struct {
    jobs.Job
    Events []domain.JobEvent
}

type jobSettings struct {
    SuccessfulJobLimit int `json:"successful_job_limit"`
}
~~~

注册 GET/PUT /api/settings/jobs。PUT 严格解码整数，校验 1 至 500，调用 Store.SetSuccessfulJobLimit，并返回服务端确认值。

~~~go
func (s *Server) setJobSettings(w http.ResponseWriter, r *http.Request) {
    var input jobSettings
    if decodeJSON(w, r, &input) != nil { return }
    if input.SuccessfulJobLimit < store.MinSuccessfulJobLimit ||
        input.SuccessfulJobLimit > store.MaxSuccessfulJobLimit {
        writeError(w, http.StatusUnprocessableEntity, "invalid_job_limit", "successful_job_limit must be between 1 and 500")
        return
    }
    if err := s.store.SetSuccessfulJobLimit(input.SuccessfulJobLimit); err != nil {
        writeError(w, http.StatusInternalServerError, "settings_error", err.Error())
        return
    }
    writeJSON(w, http.StatusOK, input)
}
~~~

- [ ] **Step 4: 运行 HTTP 包测试**

Run: go test ./internal/httpapi -count=1

Expected: PASS。

- [ ] **Step 5: 提交 HTTP 切片**

~~~powershell
git add internal/httpapi/server.go internal/httpapi/server_test.go
git commit -m "feat(api): 提供任务日志详情与保留设置"
~~~

### Task 4: 最近任务行内日志界面

**Files:**
- Modify: web/src/api/client.ts
- Create: web/src/app/JobsPage.tsx
- Create: web/src/app/JobsPage.test.tsx
- Modify: web/src/app/App.tsx
- Modify: web/src/styles/app.css

**Why this task exists:**
- 管理员无需离开任务列表即可查看失败原因和完整结构化阶段。
- 摘要提供可扫描的发起时间和执行用时。

**Impact / Compatibility:**
- App 主导航和 JobsPage 入口保持不变。
- 同时只展开一个任务；终态详情缓存，运行态按 UpdatedAt 更新。

**Repair Track:**
- 当前行只展示最后 Error/Message，无法表达事件历史；独立 JobsPage 负责详情状态、缓存和无障碍展开。
- App 只保留页面路由，不继续拥有第二份任务列表实现。

**Retirement Track:**
- 删除 App.tsx 内旧 JobsPage 和本地 JobRecord 类型，避免两个展示所有者并存。
- 现有 job-row 视觉语言保留，由新组件和扩展样式继续使用。

**Verification:**
- cd web && npm test -- --run src/app/JobsPage.test.tsx src/app/App.test.tsx

- [ ] **Step 1: 写失败的展开、失败原因和时长测试**

~~~tsx
it("expands one job and renders structured failure events", async () => {
  render(<JobsPage />);
  await userEvent.click(await screen.findByRole("button", { name: /查看 game_update 任务日志/ }));
  expect(await screen.findByText("download interrupted")).toBeVisible();
  expect(screen.getByText("执行 2分18秒")).toBeVisible();
  expect(fetch).toHaveBeenCalledWith("/api/jobs/job-1", expect.anything());
});
~~~

另测再次点击收起、切换任务关闭前一项、终态缓存、运行态 UpdatedAt 变化重新读取、详情错误重试和历史快照空状态。

- [ ] **Step 2: 运行组件测试并确认红灯**

Run: npm test -- --run src/app/JobsPage.test.tsx

Expected: FAIL，因为 JobsPage 独立组件和详情交互尚不存在。

- [ ] **Step 3: 扩展 Job 类型并实现独立 JobsPage**

~~~ts
export type JobEvent = {
  ID: number;
  JobID: string;
  Kind: string;
  Stage: string;
  Percent: number;
  Message: string;
  CreatedAt: string;
};

export type Job = {
  ID: string;
  InstanceID: string;
  Type: string;
  Status: string;
  Stage: string;
  Percent: number;
  Message: string;
  Error: string;
  CreatedAt: string;
  UpdatedAt: string;
  StartedAt?: string | null;
  FinishedAt?: string | null;
  Events?: JobEvent[];
};
~~~

JobsPage 使用 button、aria-expanded 和 role=region。formatTimestamp 使用浏览器本地时区；formatDuration 处理排队中、运行中、终态和旧任务近似值。

- [ ] **Step 4: 实现稳定的桌面与移动端样式**

任务摘要增加时间列和 ChevronDown 图标；展开区使用紧凑事件时间线、错误摘要、加载/空/重试状态。800px 以下换行，长消息 overflow-wrap:anywhere，不产生横向页面滚动。

~~~css
.job-row-toggle {
  width: 100%;
  display: grid;
  grid-template-columns: minmax(130px, .8fr) minmax(220px, 1.5fr) minmax(150px, .9fr) minmax(100px, .7fr) auto;
  align-items: center;
}
.job-log-event {
  display: grid;
  grid-template-columns: 88px minmax(0, 1fr);
}
.job-log-event p {
  overflow-wrap: anywhere;
}
@media (max-width: 800px) {
  .job-row-toggle { grid-template-columns: 1fr auto; }
  .job-log-event { grid-template-columns: 72px minmax(0, 1fr); }
}
~~~

- [ ] **Step 5: 运行前端目标测试**

Run: npm test -- --run src/app/JobsPage.test.tsx src/app/App.test.tsx

Expected: PASS。

- [ ] **Step 6: 提交任务界面切片**

~~~powershell
git add web/src/api/client.ts web/src/app/JobsPage.tsx web/src/app/JobsPage.test.tsx web/src/app/App.tsx web/src/styles/app.css
git commit -m "feat(web): 支持展开任务日志与执行时间"
~~~

### Task 5: 系统设置成功任务上限

**Files:**
- Modify: web/src/app/App.tsx
- Modify: web/src/app/App.test.tsx

**Why this task exists:**
- 管理员可以持久调整全局成功任务保留数量。
- 明确该设置不影响失败和中断记录。

**Impact / Compatibility:**
- Steam 和 GitHub 凭据表单保持原行为。
- 设置加载失败使用现有 settingsError，不伪造保存成功。

**Verification:**
- cd web && npm test -- --run src/app/App.test.tsx

- [ ] **Step 1: 写失败的设置读取、保存和错误测试**

~~~tsx
it("updates the successful job retention limit", async () => {
  render(<App initialInstances={[instance]} />);
  await userEvent.click(screen.getByRole("button", { name: "系统设置" }));
  const input = await screen.findByRole("spinbutton", { name: "成功任务保留数量" });
  expect(input).toHaveValue(25);
  await userEvent.clear(input);
  await userEvent.type(input, "40");
  await userEvent.click(screen.getByRole("button", { name: "保存任务记录设置" }));
  expect(fetch).toHaveBeenCalledWith("/api/settings/jobs", expect.objectContaining({
    method: "PUT",
    body: JSON.stringify({ successful_job_limit: 40 }),
  }));
});
~~~

另测 1/500 输入边界、保存期间禁用、服务端失败保留原确认值，以及说明文本包含失败和中断不会自动删除。

- [ ] **Step 2: 运行设置测试并确认红灯**

Run: npm test -- --run src/app/App.test.tsx -t "successful job retention"

Expected: FAIL，因为系统设置尚无任务记录表单。

- [ ] **Step 3: 实现任务记录设置表单**

SettingsPage 加载 GET /api/settings/jobs，维护 confirmedLimit、draftLimit 和 savingJobs。提交 PUT 后仅用服务端返回值更新 confirmedLimit；失败恢复草稿到确认值并显示 errorMessage。

~~~tsx
<form className="control-form" onSubmit={saveJobSettings}>
  <p className="eyebrow">JOB RECORDS</p>
  <h2>任务记录</h2>
  <label>
    成功任务保留数量
    <input
      aria-label="成功任务保留数量"
      type="number"
      min={1}
      max={500}
      value={draftLimit}
      onChange={(event) => setDraftLimit(Number(event.target.value))}
    />
  </label>
  <p>仅限制成功任务；失败和中断任务不会自动删除。</p>
  <button disabled={savingJobs} aria-label="保存任务记录设置">保存</button>
</form>
~~~

- [ ] **Step 4: 运行 App 回归测试**

Run: npm test -- --run src/app/App.test.tsx

Expected: PASS。

- [ ] **Step 5: 提交设置界面切片**

~~~powershell
git add web/src/app/App.tsx web/src/app/App.test.tsx
git commit -m "feat(settings): 可调整成功任务保留数量"
~~~

### Task 6: E2E 主流程与完成证据

**Files:**
- Modify: cmd/e2e-fixture/main.go
- Modify: web/e2e/control-panel.spec.ts
- Create: docs/aegis/work/2026-07-16-recent-job-logs-retention/50-evidence.md

**Why this task exists:**
- 验证真实浏览器从任务列表展开失败日志、读取时长并保存设置。
- 用完整回归证明跨层契约没有破坏现有控制面板流程。

**Impact / Compatibility:**
- E2E 夹具继续提供原有成功、失败、运行和中断任务。
- 只增加确定性事件和时间，不改变其他页面测试数据。

**Verification:**
- 全部项目验证命令和桌面/移动端 Playwright 主流程。

- [ ] **Step 1: 扩展确定性任务夹具和 Playwright 断言**

fixture_failure 增加 StartedAt、FinishedAt 和 failed 事件“deterministic fixture failure”。Playwright 点击其 aria-expanded 任务按钮，断言失败摘要、事件时间线和执行用时；进入系统设置把成功上限改为 25 并断言保存确认。使用 testInfo.outputPath("recent-job-logs.png") 保存桌面与移动项目各自的完整页面截图。

~~~ts
const failedJob = page.getByRole("button", { name: /查看 fixture_failure 任务日志/ });
await failedJob.click();
await expect(page.getByRole("region", { name: "fixture_failure 任务日志" }))
  .toContainText("deterministic fixture failure");
await expect(page.getByText(/执行用时/)).toBeVisible();
await page.screenshot({
  path: testInfo.outputPath("recent-job-logs.png"),
  fullPage: true,
});
~~~

- [ ] **Step 2: 运行目标 E2E**

Run: npm run e2e -- --grep "authenticated control panel"

Expected: PASS，桌面和移动项目均完成任务展开与设置保存。

- [ ] **Step 3: 运行完整后端验证**

Run: go test ./...

Expected: PASS，所有包零失败。

Run: go vet ./...

Expected: exit 0。

- [ ] **Step 4: 运行完整前端验证**

Run: npm test -- --run

Expected: 所有 Vitest 文件和测试通过。

Run: npm run build

Expected: TypeScript 与 Vite 构建 exit 0。

Run: npm run e2e

Expected: 所有 Playwright 项目通过。

- [ ] **Step 5: 记录证据并检查副作用**

在 50-evidence.md 写入命令、退出状态、关键计数、主流程截图路径、未验证项和置信等级。运行 git diff --check 与 git status --short，确认没有 node_modules、构建产物、数据库或 package-lock 进入版本控制。

- [ ] **Step 6: 提交 E2E 与证据**

~~~powershell
git add cmd/e2e-fixture/main.go web/e2e/control-panel.spec.ts docs/aegis/work/2026-07-16-recent-job-logs-retention/50-evidence.md
git commit -m "test(jobs): 覆盖任务日志与保留设置主流程"
~~~

## 风险与回滚面

- 迁移版本 6 是前向数据库变更，回滚应用版本前必须备份 SQLite；旧二进制无法读取新增事件，但 jobs 原字段仍保留。
- 成功任务删除不可恢复，因此设置更新与清理必须原子提交，默认 25 的首次清理只在新任务成功或管理员保存设置时触发。
- UI 回滚只需恢复 JobsPage 和 SettingsPage；新增 API 字段对旧客户端为可忽略字段。
- 如果完整 E2E 环境不可启动，只能把完成结论降为部分验证，并在 50-evidence.md 记录手工复现步骤，不得用单元测试代替主流程。
