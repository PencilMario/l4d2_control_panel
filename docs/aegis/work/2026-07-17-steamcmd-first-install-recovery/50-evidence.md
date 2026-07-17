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

## 安可远端验收

- 远端 `internal/docker/client.go` 部署前 SHA-256 与本地 `main` 完全一致：`6e9cab0f70218ff5fd30c297d6e78d3b1332e666a8d763c27febfb5d38eb0bb5`，排除了覆盖服务器侧未同步代码的风险。
- 只同步 `internal/docker/client.go`，`docker compose build panel` 生成镜像 `sha256:7b05c18e3e1f444d827c05723e6a54ac7393336cbe8f6a23a75b8b535d96072b`，随后只执行 `up -d --no-deps --force-recreate panel`。
- Panel 容器从 `e48997047cb3` 变为 `618240e1f36c`；`/api/health` 返回数据库和 Docker 均为 `ok`。
- 第一个实例容器部署前后均为 `46eb431c3950`，其 `appmanifest_222860.acf` SHA-256 部署前后均为 `d45c952bb9704c8f888cd870f462338697f0d26a1ec8eb242739f2bb8ae0d41c`。
- 现有失败实例 `7c991725-7345-42ad-a030-61c8e59a7746` 在部署前为 `faulted`、无容器、目录 108 KiB。通过一次 `start` Action 创建 Job `42fc0eba-fc94-4d95-be8e-3f117bd6f623`，没有重复提交操作。
- 第一次维护容器 `c70ad23d3a5c` 在 Windows 引导阶段再次记录 `ERROR! Failed to install app '222860' (Missing configuration)`；紧接着 Job 记录 `SteamCMD configuration missing during first install; retrying attempt 2 of 3`。
- 第二次维护容器 `5163c2fabf38` 使用保留的全局 Steam 元数据，Windows depot 完成 9,710,287,462 字节安装并报告成功；切换 Linux 后下载 202,762,857 字节并再次报告成功。
- Job 于 `2026-07-17T07:15:01Z` 以 `succeeded`、100% 完成。第二实例创建运行容器 `080bb0ace812`，实例目录为 9.5 GiB，独立 manifest SHA-256 为 `481eb76dc394b4023b08a5f928fd3bdff13772b3fbacea4e3009380a045c6bcb`。
- 实例 Overview 返回 `actual_state=running`、`container_running=true`、地图 `c2m1_highway`、A2S 延迟约 9 ms。全局 Steam 元数据目录为 8.4 MiB，最终没有残留 maintenance 容器。
- 未验证项：连续三次都由 Valve 返回配置缺失的真实远端分支只由自动化测试覆盖；本次真实恢复在第二次尝试成功。

## 证据判断

本地精确分类、重试上限、非目标错误和 Docker 错误边界具有自动化回归覆盖。安可真实故障在第一次尝试复现，并在自动第二次尝试完成安装、插件部署、游戏容器启动和 A2S 检查。对本次用户目标的信心为 A；连续三次外部配置缺失仍是有界失败而非可保证成功的情况。
