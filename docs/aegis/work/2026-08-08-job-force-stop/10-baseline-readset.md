# Baseline Read Set: 后台任务强制停止

## Authority

- `CONTEXT.md`: 项目术语和状态边界。
- `README.md`: 任务、测试和部署命令。
- `docs/aegis/specs/2026-08-08-job-force-stop-design.md`: 本任务已确认的设计边界。

## Code

- `internal/jobs/manager.go`: 当前状态机只记录 `pending/running/succeeded/failed/interrupted`，没有运行时取消句柄。
- `internal/jobs/manager_test.go`: 现有生命周期、上下文超时、事件链测试。
- `internal/httpapi/server.go`: 认证任务路由、`startJob`、`getJob`、SSE 任务列表。
- `internal/httpapi/server_test.go`: 已有任务查询和认证 HTTP 测试辅助。
- `web/src/app/JobsPage.tsx`: 任务表、事件展开、SSE 摘要刷新。
- `web/src/app/JobsPage.test.tsx`: 任务页组件测试。
- `web/src/styles/app.css`: `.job-operation` 与移动端任务行布局。

## Baseline evidence

- `go test ./internal/jobs ./internal/httpapi`: PASS。
- `web/node_modules`: 隔离 worktree 中不存在；前端测试待安装依赖后执行。
- 工作区创建前 `main`：clean，HEAD `b727eb7`。
