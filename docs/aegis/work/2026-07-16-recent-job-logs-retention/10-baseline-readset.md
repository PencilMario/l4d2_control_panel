# 最近任务日志与成功记录保留：基线读取集

## 权威输入

- docs/aegis/specs/2026-07-16-recent-job-logs-retention-design.md：已批准的功能、数据、交互和兼容边界。
- CONTEXT.md：游戏实例和状态术语；本任务不改变实例生命周期定义。
- internal/jobs/manager.go：当前任务串行化、Reporter 和持久任务所有权。
- internal/store/store.go 与 internal/store/migrations.go：SQLite、WAL、迁移和任务记录所有权。
- internal/httpapi/server.go：认证路由、任务摘要、详情和 SSE 契约。
- web/src/app/App.tsx 与 web/src/styles/app.css：最近任务和系统设置现有界面。
- web/e2e/control-panel.spec.ts 与 cmd/e2e-fixture/main.go：控制面板主流程和确定性任务夹具。

## 已知事实

- jobs 只保存最后一次阶段、消息和错误，没有事件历史或真实开始/结束时间。
- GET /api/jobs 与 SSE 返回 SQLite 摘要；GET /api/jobs/{id} 由 Manager 返回快照。
- Reporter.Progress 当前覆盖快照，持久化错误不改变任务业务函数结果。
- Store 使用单 SQLite 连接、WAL、外键和版本 1 至 5 的迁移。
- 系统设置只有加密凭据，没有普通数值设置表。
- React JobsPage 和 SettingsPage 当前位于大型 App.tsx 中。

## 兼容边界

- 保留任务 ID、Status、Stage、Percent、Message、Error、CreatedAt 和 UpdatedAt。
- 保留 GET /api/jobs/{id} 顶层字段、jobs SSE 事件名和每秒摘要流。
- 保留按 InstanceID 串行执行、Panel 重启恢复和现有任务启动接口。
- 事件日志只追加 Panel 已知的结构化消息，不采集外部进程原始输出。

## 基线证据

- go test ./...：全部 Go 包通过。
- cd web && npm test -- --run：5 个文件、83 个测试通过。
- 依赖安装：go mod download 与 npm install --no-package-lock 成功。

## 未知与处理

- 旧任务没有真实开始时间：迁移使用 CreatedAt 近似，并生成明确的历史快照事件。
- Playwright 完整运行时间较长：作为最终主流程验证执行，不用单元测试替代。
