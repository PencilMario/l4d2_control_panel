# 持久游戏日志查看器实施检查点

## TaskIntentDraft

为每个实例持久保存并安全查看游戏与 SourceMod 日志；日志跨普通重装保留，默认 14 天，并且所有过期删除进入可观察的实例任务流水。

## BaselineReadSetHint

- `docs/aegis/specs/2026-07-18-persistent-game-log-viewer-design.md`
- `docs/aegis/plans/2026-07-18-persistent-game-log-viewer.md`
- `CONTEXT.md`
- `internal/docker/lifecycle.go`
- `internal/lifecycle/service.go`
- 现有 jobs、joblogs、scheduler、settings 与 React 私有文件页面契约

## ImpactStatementDraft

影响容器挂载、实例生命周期、持久文件所有权、任务调度、HTTP 和 React。两个标准日志目录改由 `instances/<id>/logs/` 唯一持有；不得改变其他 Overlay、私有文件、插件包、共享 VPK 或任务串行行为。

## TodoCheckpointDraft

- [x] Task 1：持久目录、迁移与容器挂载
- [x] Task 2：安全文件树、尾部预览与下载源
- [ ] Task 3：保留设置与清理行为（active）
- [ ] Task 4：清理任务与每日排队
- [ ] Task 5：认证 HTTP API
- [ ] Task 6：安全高亮 React 查看器
- [ ] Task 7：导航、设置、E2E 与文档

当前证据：

- `go test ./...`：通过。
- `web/npm test -- --run`：10 个测试文件、119 项测试通过。
- Task 1 commits：`7beb166`、`c91a319`、`91bc29b`、`5cf52f7`。
- `go test -count=1 ./internal/gamelogs ./internal/docker ./internal/lifecycle`：通过。
- Task 1 规格审查：通过；代码质量审查：Approved。
- Task 2 commits：`078659d`、`8d76d55`、`2f77e42`。
- `go test -count=1 ./internal/gamelogs ./internal/docker ./internal/lifecycle`：通过。
- `go vet ./...`：通过；Task 2 规格审查与代码质量审查均通过。

阻塞项：无。

下一步：Task 3 实现代理按 TDD 增加默认 14 天设置和过期普通文件清理统计。

## ResumeStateHint

工作分支为 `feature/persistent-game-logs`，工作区为 `.worktrees/persistent-game-logs`。恢复时先核对本文件、分支状态和最新提交；禁止在主工作区实现代码。

## DriftCheckDraft

- 仍服务原始意图：是。
- 兼容边界：尚未改变。
- 新 owner/fallback：`internal/gamelogs` 已成为持久日志目录与迁移唯一 owner，未增加 Overlay 回退。
- 退役轨迹：旧 Overlay 日志路径在幂等迁移后退役。
- 决策：continue。

残余风险：基于路径的文件 API 无法抵御拥有宿主 DataRoot 写权限的特权 actor 在校验后替换目录；该 actor 已可直接篡改数据库与实例数据，不属于现有 Panel safepath 威胁模型。
