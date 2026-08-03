# Job 超时与共享 VPK 重启实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为所有后台 Job 增加分钟级超时契约，并让新 VPK 发布立即创建可见、等待玩家后最多 24 小时强制重启的 `shared_vpk_restart` Job。

**Architecture:** Job 管理器统一归一化并持久化 `timeout_minutes`，普通 Job 在取得实例锁后以 deadline context 执行；VPK Job 使用锁前 preflight 等待玩家，再在实例锁内复核并重启。计划任务把分钟数持久化并传递给触发的 Job，旧记录默认 1440 分钟。

**Tech Stack:** Go、SQLite、React/TypeScript、Vitest、现有 Job Manager、VPK Restart Coordinator、SSE 任务列表。

**Baseline / Authority Refs:** [Job 超时与共享 VPK 重启设计](../specs/2026-08-03-job-timeouts-and-vpk-restart-design.md)、[共享 VPK 空服延迟重启设计](../specs/2026-07-17-shared-vpk-deferred-restart-design.md)、`internal/jobs/manager.go`、`internal/store/store.go`、`internal/automation/dispatcher.go`、`web/src/app/SchedulesPage.tsx`、`web/src/app/JobsPage.tsx`。

**Compatibility Boundary:** 现有 `Start` 调用默认 1440 分钟；上传原子发布、管理员/公开入口、实例串行操作、停止实例不自动启动、在线策略名称和共享 VPK 挂载保持不变。未完成事项不能重复创建 Job 或重复重启。

**Verification:** 每个任务先添加失败测试再实现；最终运行 `go test ./... -count=1`、相关 Vitest 测试、`npm run build`，并通过 HTTP/任务列表集成测试验证公开与管理员上传主流程。

---

### Task 1: Job 超时数据契约与迁移

**Files:**
- Modify: `internal/domain/models.go` (`JobRecord`、`ScheduledTask`)
- Modify: `internal/store/migrations.go`、`internal/store/job_history.go`
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Why this task exists:** 持久化超时是所有 Job、计划任务和 Panel 重启恢复的共同基础。

**Impact / Compatibility:** 旧 SQLite 数据和旧 Go 调用必须读取为 1440；新字段必须出现在 Job/计划任务 API 使用的 domain record 中。

**Verification:** `go test ./internal/store -run 'Test.*(Job|Schedule|VPKRestart)' -count=1`。

- [ ] **Step 1: Write the failing tests**：断言新库默认值为 1440、旧 jobs/scheduled_tasks 缺失或零值归一化为 1440、显式分钟值可保存并读取。
- [ ] **Step 2: Run tests and verify failure**：`go test ./internal/store -run 'Test.*Timeout' -count=1`；预期因字段不存在或返回零值失败。
- [ ] **Step 3: Add migration and normalization**：给 `jobs`、`scheduled_tasks` 增加 `timeout_minutes`，提供旧库迁移；在读取和写入边界调用同一个 `NormalizeTimeoutMinutes`，范围限定 1 至 10080，零值默认为 1440。
- [ ] **Step 4: Run tests and verify pass**：重复运行目标测试，确认旧库兼容与显式值通过。
- [ ] **Step 5: Commit**：`git add internal/domain internal/store && git commit -m "feat(jobs): 持久化 Job 超时配置"`。

**Repair Track:** 修复当前 Job/计划任务没有统一超时字段的问题，所有持久化与归一化由 store/domain 边界负责。

**Retirement Track:** 退役各模块隐含的无限等待/零值语义；保留旧数据库迁移兼容，直到所有旧记录完成读取验证。

### Task 2: Job Manager 统一超时与锁前阶段

**Files:**
- Modify: `internal/jobs/manager.go`
- Test: `internal/jobs/manager_test.go`
- Modify: `internal/jobs/manager_test.go` fixtures and repository assertions

**Why this task exists:** 普通 Job 需要统一 deadline；VPK 等待必须立即可见但不能占用实例锁。

**Impact / Compatibility:** 保留 `Start(ctx, instanceID, kind, fn)`，新增带 options/preflight/timeout 的入口；现有任务行为默认不变。

**Verification:** `go test ./internal/jobs -count=1`。

- [ ] **Step 1: Write failing tests**：测试默认 1440、显式分钟值、deadline 取消后失败、preflight 在锁外运行且同实例普通 Job 可并行进入、任务详情持久化 timeout。
- [ ] **Step 2: Run red tests**：`go test ./internal/jobs -run 'Test.*Timeout|Test.*Preflight' -count=1`；预期新入口和字段缺失。
- [ ] **Step 3: Implement minimal manager API**：新增 `StartWithOptions` 和 options `{TimeoutMinutes int; Preflight func(context.Context) error}`；Job 创建即持久化 pending，preflight 在实例锁外执行，完成后取得锁；普通 fn 使用 `context.WithTimeout`，deadline 返回后由 manager 标记 failed。
- [ ] **Step 4: Run green tests**：目标测试和完整 `internal/jobs` 包通过，确认函数响应 context 后不会继续修改 Job 状态。
- [ ] **Step 5: Commit**：`git add internal/jobs && git commit -m "feat(jobs): 增加统一超时与锁前阶段"`。

**Repair Track:** 修复 Job 只有创建/运行状态而无 deadline、长等待占用实例锁的问题。

**Retirement Track:** 退役业务模块自行维护普通 timeout 的做法；保留 `Start` 兼容包装作为默认入口。

### Task 3: 计划任务分钟配置

**Files:**
- Modify: `internal/httpapi/server.go` schedule request/response validation
- Modify: `internal/automation/dispatcher.go`
- Modify: `web/src/app/SchedulesPage.tsx`
- Test: `internal/httpapi/server_test.go`, `internal/automation/dispatcher_test.go`, `web/src/app/SchedulesPage.test.tsx`

**Why this task exists:** 用户需要在创建/编辑计划任务时按分钟选择超时，已有计划任务必须自动使用 1440。

**Impact / Compatibility:** 旧 JSON 缺字段仍可创建；非法值返回 422；手动运行和 Cron 触发都传递同一 timeout。

**Verification:** `go test ./internal/httpapi ./internal/automation -run 'Test.*(Schedule|Timeout)' -count=1`; `cd web && npm test -- --run src/app/SchedulesPage.test.tsx`。

- [ ] **Step 1: Add failing API/UI tests**：创建、编辑、列表断言 `timeout_minutes`；缺失为 1440，0/负数/超过 10080 返回 422；前端输入分钟并保留编辑值。
- [ ] **Step 2: Run red tests**：运行上述 Go/Vitest 命令，确认字段未被保存或提交。
- [ ] **Step 3: Implement API and dispatcher propagation**：扩展 request/domain mapping、校验归一化；`dispatcher` 调用 `StartWithOptions` 传入计划值。
- [ ] **Step 4: Implement UI control**：在计划创建/编辑表单加入整数分钟 input，默认 1440，显示单位“分钟”，提交和 normalize 保留字段。
- [ ] **Step 5: Run green tests and commit**：目标测试通过后提交 `git add internal/httpapi internal/automation web/src/app/SchedulesPage* && git commit -m "feat(schedules): 支持按分钟配置 Job 超时"`。

### Task 4: VPK 立即创建真实 Job

**Files:**
- Modify: `internal/domain/models.go`
- Modify: `internal/vpkrestart/coordinator.go`
- Modify: `internal/store/store.go`
- Modify: `internal/store/migrations.go`
- Test: `internal/vpkrestart/coordinator_test.go`, `internal/store/store_test.go`, `internal/httpapi/server_test.go`

**Why this task exists:** 上传完成后用户必须立即在任务列表看到每实例真实 `shared_vpk_restart` Job，而不是等待事项合成投影。

**Impact / Compatibility:** 管理员和公开完成接口继续调用同一 registrar；重复 Hash/失败发布不登记；同实例已有未完成 Job 时合并 publication，不重复创建。

**Verification:** `go test ./internal/vpkrestart ./internal/store ./internal/httpapi -run 'Test.*(VPK|Vpk|Restart)' -count=1`。

- [ ] **Step 1: Add failing tests**：新发布立即得到真实 Job ID；任务列表包含该 Job；重复发布只更新 publication；停止/无容器实例不创建。
- [ ] **Step 2: Run red tests**：预期当前只生成 pending projection 或等待后才创建 Job。
- [ ] **Step 3: Implement real Job ownership**：在 registrar 中通过 Job Manager `StartWithOptions` 创建/恢复真实 Job，将 Job ID 写入 `shared_vpk_restarts`；删除 `Store.Jobs` 的重复合成投影路径，仅保留迁移恢复兼容。
- [ ] **Step 4: Run green tests**：确认管理员和公开完成 HTTP 响应的 `restart_instances` 不变，任务列表不重复。
- [ ] **Step 5: Commit**：`git add internal/domain internal/vpkrestart internal/store internal/httpapi && git commit -m "feat(vpk): 上传后立即创建重启任务"`。

**Repair Track:** 修复等待事项不是真实 Job、用户看不到任务的问题。

**Retirement Track:** 退役 `vpk-restart:<instance>` 合成任务投影和“空服后才创建 Job”路径；保留旧事项迁移读取直到恢复测试覆盖完成。

### Task 5: VPK 玩家等待与 24 小时强制重启

**Files:**
- Modify: `internal/vpkrestart/coordinator.go`
- Test: `internal/vpkrestart/coordinator_test.go`

**Why this task exists:** VPK Job 要使用玩家等待规则，查询失败不提前重启，24 小时后强制重启。

**Impact / Compatibility:** 等待在 preflight 执行；重启前再次核对容器代次和期望状态；停止实例取消，不自动启动。

**Verification:** `go test ./internal/vpkrestart -count=1`。

- [ ] **Step 1: Add failing tests**：有玩家持续等待；空服立即进入 restart；查询失败持续等待；deadline 到达调用 Restart；容器变更不二次重启；停止实例取消。
- [ ] **Step 2: Run red tests**：确认旧三次失败阈值和无限等待测试失败或需要改写。
- [ ] **Step 3: Implement preflight state machine**：每 30 秒查询玩家，向 reporter 更新 `waiting_players` 和剩余时间；用 Job timeout deadline 判断 `timeout_force`，返回 preflight success 后在锁内 Restart。
- [ ] **Step 4: Run green tests**：VPK 包全测通过，确认等待不持有实例锁。
- [ ] **Step 5: Commit**：`git add internal/vpkrestart && git commit -m "feat(vpk): 等待玩家后在 24 小时内强制重启"`。

### Task 6: Job API、任务列表与恢复

**Files:**
- Modify: `internal/httpapi/server.go`
- Modify: `web/src/app/JobsPage.tsx`
- Modify: `internal/store/store.go`, `internal/jobs/manager.go`
- Test: `internal/httpapi/server_test.go`, `web/src/app/JobsPage.test.tsx`, `internal/store/store_test.go`

**Why this task exists:** 超时时间、VPK 等待状态和恢复结果必须在 API、任务详情、事件和日志中可见。

**Impact / Compatibility:** 现有 Job ID、详情和 SSE 路由保持不变；新字段可选读取，旧客户端忽略。

**Verification:** `go test ./internal/httpapi ./internal/store -run 'Test.*Job' -count=1`; `cd web && npm test -- --run src/app/JobsPage.test.tsx`。

- [ ] **Step 1: Add failing API/UI tests**：Job 列表/详情返回 timeout；VPK pending 显示目标、玩家等待、剩余时间；普通超时显示失败原因；SSE 更新真实 Job。
- [ ] **Step 2: Run red tests**：确认 API 缺字段或 UI 不显示。
- [ ] **Step 3: Implement mapping and presentation**：扩展 domain-to-JSON、任务列/详情文本和阶段标签；恢复逻辑按原始创建时间计算剩余期限。
- [ ] **Step 4: Run green tests**：目标包和前端任务页测试通过。
- [ ] **Step 5: Commit**：`git add internal/httpapi internal/store internal/jobs web/src/app/JobsPage* && git commit -m "feat(jobs): 展示超时与 VPK 等待状态"`。

### Task 7: 全链路回归与拖放修复提交

**Files:**
- Test/verify: `internal/httpapi/server_test.go`, `internal/vpkrestart/*_test.go`, `web/src/app/SelfServiceVPKPage.test.tsx`
- Commit pending existing changes: `web/src/app/SelfServiceVPKPage.tsx`, `web/src/app/SelfServiceVPKPage.test.tsx`, `web/src/styles/app.css`

**Why this task exists:** 验证公开/管理员上传主流程、任务可见性、恢复、超时和刚修复的公开拖放入口没有回归。

**Impact / Compatibility:** 不修改功能契约，只收集新鲜证据；`web/public/vpk-cleaner.wasm` 为构建生成物，不纳入提交。

**Verification:**

- [ ] **Step 1: Run Go regression**：`go test ./... -count=1`，预期全部通过。
- [ ] **Step 2: Run frontend regression**：`cd web && npm test -- --run`，预期全部通过。
- [ ] **Step 3: Build frontend**：`npm run build`，确认 TypeScript/Vite 构建成功；检查并移除构建生成的未提交 wasm 变更。
- [ ] **Step 4: Run diff checks**：`git diff --check; git status --short`，确认只包含计划内代码和测试。
- [ ] **Step 5: Commit drag-and-drop repair**：`git add web/src/app/SelfServiceVPKPage.tsx web/src/app/SelfServiceVPKPage.test.tsx web/src/styles/app.css && git commit -m "fix(web): 支持公开 VPK 拖放上传"`。
- [ ] **Step 6: Record evidence**：在 `docs/aegis/work/2026-08-03-job-timeouts-and-vpk-restart/50-evidence.md` 记录命令、结果、部署前后残余风险；不声称远端部署完成，除非另行执行 SSH 验证。

**Repair Track:** 覆盖公开上传拖放缺少事件绑定，以及 VPK 任务不可见的用户症状。

**Retirement Track:** 删除构建产物变更和旧合成任务展示；保留测试作为未来收敛验证。
