# 后台任务强制停止设计

## 目标

在后台任务列表中为正在执行的任务提供“强制停止”入口。管理员确认后，Panel 取消该任务的执行上下文；任务完成收尾后以 `interrupted` 终态持久化，并通过现有 SSE 任务列表同步到页面。

## 已确认范围

- 仅 `running` 状态的后台任务显示强制停止按钮。
- 点击按钮先经过确认；取消确认不发送请求。
- 后端提供 `POST /api/jobs/{id}/cancel`，只接受正在执行的任务。
- 取消请求返回 `202` 和当前任务快照；真正的终态由任务执行函数返回后写入 `interrupted`。
- 取消原因写入任务错误字段和结构化事件，任务完整日志记录中断结果。
- 保留现有任务表布局、操作按钮类名和响应式网格；新增样式只作用于强制停止按钮及“停止中”状态。

## 非目标

- 本次不取消 `pending` 排队任务。
- 本次不增加任务删除、重试或批量停止功能。
- 本次不改变任务超时策略、实例锁顺序或任务持久化表结构。
- 本次不承诺终止不响应 `context.Context` 的任意第三方代码；现有 Docker、网络和命令执行边界通过上下文取消完成实际中断。

## 现状与约束

- `internal/jobs.Manager` 已有 `interrupted` 状态，但目前仅用于 Panel 重启恢复，没有运行时取消入口。
- `Manager.StartWithOptions` 为任务创建带超时的执行上下文，并把上下文传入任务函数；HTTP 入口使用 `context.WithoutCancel` 保证请求结束不会中断后台任务。
- `internal/httpapi.Server` 已统一认证、审计和任务查询路由；取消路由应加入同一认证组。
- `web/src/app/JobsPage.tsx` 已有运行状态、任务事件和完整日志操作；不重构任务页表格。
- `web/src/styles/app.css` 的 `.job-operation` 使用固定操作列和移动端响应式规则；强制停止按钮必须服从现有按钮尺寸、间距和换行约束。

## 设计

### 任务管理器

Manager 新增每个活动任务的取消函数和取消请求标记。任务协程在进入可执行生命周期前登记取消函数；`Cancel` 在互斥锁内确认任务仍为 `running`，标记取消请求，然后在锁外调用取消函数，避免回调阻塞任务状态锁。

任务结束时统一判断是否由 Manager 发起取消：

- 是：记录 `interrupted`，错误信息为“任务已由管理员强制停止”，并记录中断日志。
- 否且函数返回错误：保持现有 `failed` 行为。
- 否且函数成功返回：保持现有 `succeeded` 行为。

取消句柄在任务协程退出时清理。取消请求与任务返回之间保持 `running` 快照，避免任务尚未释放实例锁时被误认为已结束；页面显示“停止中”并禁用重复操作，收到终态 SSE 后恢复普通状态。

### HTTP API

新增 `POST /api/jobs/{id}/cancel`。成功时返回 `202` 和任务快照；任务不存在返回 `404 job_not_found`；任务不是 `running` 返回 `409 job_not_running`。接口不接受请求体，沿用现有认证和审计中间件。

### 前端

`JobsPage` 只为 `running` 行渲染停止按钮。按钮使用现有 `job-operation` 容器和 Lucide 图标，点击后调用 `window.confirm`；确认后请求取消 API，将当前行置为停止中，失败时复用任务页错误提示。SSE 继续作为最终事实来源，终态任务不再显示该按钮。

### 错误与竞态

- 重复点击由前端禁用和后端幂等取消标记共同防护。
- 任务在点击前已进入终态时，后端返回 `409`，前端显示可读错误并恢复按钮状态；由于终态行不显示按钮，主要用于竞态保护。
- 取消时任务函数返回 `nil` 也必须落为 `interrupted`，不能被误记为成功。
- 取消请求本身不直接释放实例锁；任务函数收到取消后完成已有清理，再由 Manager 释放锁。

## 验证标准

- Manager 单元测试证明活动任务收到取消、取消后的 `nil` 返回仍为 `interrupted`、取消非运行任务被拒绝，并且事件链只产生正确终态。
- HTTP 集成测试证明认证取消路由返回 `202`，任务最终持久化为 `interrupted`，不存在任务和非运行任务的错误码稳定。
- React 测试证明运行中任务显示按钮、确认取消后发出正确请求、拒绝确认不发请求，排队/终态任务不显示按钮。
- `go test ./...`、`go vet ./...`、前端目标测试、`npm run build:web` 和 `git diff --check` 通过。
- 部署到琥珀和安可后分别检查服务健康、Compose 状态，并通过 API/页面确认新版本可用。

## TaskIntentDraft

- 请求结果：为执行中的后台任务增加可验证的强制停止工作流，并更新两个生产节点。
- 变更类型：任务运行时控制、HTTP API、React 交互、CSS 状态、部署验证。
- 风险提示：取消时序、实例锁释放、任务持久化终态、远端部署期间服务重建。

## BaselineReadSetHint

- `internal/jobs/manager.go` 与 `internal/jobs/manager_test.go`：任务生命周期、上下文和事件持久化。
- `internal/httpapi/server.go` 与 `internal/httpapi/server_test.go`：认证路由、错误响应和任务查询接口。
- `web/src/app/JobsPage.tsx` 与 `JobsPage.test.tsx`：任务页操作与 SSE 更新。
- `web/src/styles/app.css`：操作列和移动端响应式样式。
- `CONTEXT.md`、`README.md`、`docs/aegis/INDEX.md`：项目术语、验证和部署边界。

## ImpactStatementDraft

- 受影响 owner：`jobs.Manager` 是取消生命周期的唯一 owner；HTTP 只负责授权和转发；React 只负责确认、反馈和展示。
- 不变量：任务仍按实例串行执行；超时、成功、失败行为不变；持久化任务在重启恢复逻辑不变。
- 兼容边界：既有 GET `/api/jobs`、GET `/api/jobs/{id}`、SSE 和任务日志接口保持不变；数据库无需迁移。
- 退休路径：旧的“只有重启才能产生 interrupted”行为保留为恢复兜底；运行时取消由 Manager 新 owner 统一产生。
