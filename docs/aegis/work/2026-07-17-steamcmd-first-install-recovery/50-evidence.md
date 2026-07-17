# SteamCMD 首装恢复证据

## 本地基线

- 干净 worktree 建立于 `96417d0`，分支为 `fix/steamcmd-first-install-recovery`。
- `go test -count=1 ./internal/docker` 在改动前后均通过。
- 初次全仓测试的两个失败均为 Windows `testing.TempDir` 清理竞态：`TestScheduleUpdateAndDeletePreserveSingleRecord`、`TestPackageUpdateRebasesDeletedPrivatePath`；两个测试单独重跑均通过。

## Task 1

- RED：新增 `TestRunMaintenanceReturnsExitCodeAndRecentOutput` 后，编译失败：`runMaintenance returns 1 value`。
- GREEN：维护结果现在包含 `StatusCode` 和 64 KiB 尾部输出；维护容器绑定 `<data-root>/panel/steamcmd/Steam:/home/steam/Steam`。
- `go test -count=1 ./internal/docker`：PASS。
- Commit：`08e34f3 refactor(steamcmd): expose bounded maintenance result`。

## Task 2

- RED：`TestAnonymousInstallRetriesMissingConfigurationThenSucceeds` 和三次耗尽测试在首次退出码 8 时失败。
- GREEN：匿名首装对精确 `Failed to install app '222860' (Missing configuration)` 最多三次独立维护容器；磁盘错误、Docker API 错误只执行一次。
- `go test -count=1 ./internal/docker -run 'TestAnonymousInstall|TestCredentialedGameInstall|TestGameUpdate'`：PASS。
- `go test -count=1 ./internal/docker`：PASS。
- Commit：`d4f6f21 fix(steamcmd): retry anonymous missing configuration`。

## 发布验证

- `go test -count=1 ./internal/provisioning ./internal/lifecycle`：PASS；首次并行批次中的清理竞态测试单独重跑 PASS。
- `go test -count=1 ./...`：生产断言和测试均通过；出现的 `TestReconcileSeparatesMaintenanceWriterFromGameContainer` 与 `TestJobsIncludesExecutionTimes` 仅为 Windows TempDir 清理竞态，分别单独重跑 PASS。
- `go vet ./...`：PASS。
- `git diff --check`：PASS。

## 远端验收待填

- 安可 Panel 镜像、容器 ID 和 `/api/health`：待部署。
- 第一个实例容器 ID/状态和 manifest 校验：待部署前后记录。
- 实例 `7c991725-7345-42ad-a030-61c8e59a7746` 的 Job 尝试次数、最终状态、manifest 和目录大小：待重试。
- 未验证项：Valve 仍可能在三次尝试中持续返回配置缺失；该情况下保留原始日志并报告外部条件，不扩大重试范围。
