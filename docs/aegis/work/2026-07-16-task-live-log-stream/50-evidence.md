# 任务实时日志流验证证据

- 日期：2026-07-16
- 分支：`feature/task-live-log-stream`
- 部署目标：安可服 `steam@100.73.249.118`

## 自动化验证

- `go test ./internal/joblogs -count=10`：通过。
- `go vet ./internal/joblogs`：通过。
- `go test ./internal/jobs ./internal/store ./cmd/panel -count=3`：通过。
- `go test ./internal/jobs ./internal/socketproxy ./cmd/socket-proxy ./internal/docker -count=1`：通过。
- `go test ./internal/httpapi ./cmd/panel -count=1`：通过。
- `go vet ./...`：通过。
- `npm test -- --run`：9 个测试文件、112 项测试通过。
- `npm run build`：通过；Vite 保留既有单 bundle 大于 500 KiB 警告。
- `npm run e2e -- --project=desktop`：1 项真实 HTTP 管理流程通过。
- `npm run e2e -- --project=mobile`：1 项真实 HTTP 管理流程通过。

## 平台差异

Windows 完整 Go 套件多次只在临时目录清理或临时文件占用处失败；相关目标连续复跑通过。Linux `golang:1.25-alpine` 完整套件暴露既有跨平台测试假设：Windows 绝对路径测试在 Linux `filepath` 语义下失败，且纯 Go 镜像没有 Python，无法运行 runtime supervisor 自测。

Linux 上与本功能相关的完整包集合通过：

```text
go test ./internal/joblogs ./internal/jobs ./internal/docker ./internal/socketproxy ./cmd/socket-proxy ./internal/httpapi ./cmd/panel -count=1
```

## 浏览器验证

- 使用 Playwright 登录已部署面板，打开最新任务详情并进入独立日志页面。
- 页面在 1440x1000 视口正确显示任务头、搜索、来源/级别筛选、复制、下载和实时日志表格。
- 页面尺寸为 1101x1198，无控件重叠、空白画布或横向布局破坏。
- 实时页面成功显示 300 条 SteamCMD 日志。

## 真实 SteamCMD 验证

- 任务：`17fc31ad-8496-4c49-81c1-47d81886a924`。
- SteamCMD maintenance 容器成功使用绝对入口启动，并实时采集 stdout/stderr。
- 日志记录了约 9.71 GB 更新过程，从 0% 持续到 98.34%。
- 最终诊断从原先只有退出码提升为：`Error! App '222860' state is 0x402 after update job.`，随后记录 `steamcmd exited with code 8`。
- maintenance 容器删除后，JSONL 文件仍存在，大小 153227 字节。
- 历史 API 跨 Panel 重启仍可读取；下载接口返回 HTTP 200 和相同 153227 字节。
- 本任务未达到 10 MiB，因此 `truncated=false`；终态截断由 joblogs 单测覆盖。

## 远端安全与回归

- `/api/health` 返回 `status=ok`。
- Panel 容器健康状态为 `healthy`。
- Panel 可访问受限 `/run/l4d2-panel/proxy.sock`，不可见 `/var/run/docker.sock`。
- socket proxy 唯一增加能力为 `CAP_NET_RAW`。
- 日志代理只允许带 `managed=true`、`role=maintenance` 标签的容器；策略与 handler 测试通过。
- MariaDB 与 TeamSpeak 在部署后保持 `running`。
- 任务日志文件位于 `/home/steam/l4d2-panel-data/panel/job-logs`，跟随持久数据目录。

## 残余风险

- App 222860 本次仍在 98.34% 后以状态 `0x402` 失败；完整日志功能已经将根因证据暴露，但修复 Steam 内容下载本身不属于本规格。
- 前端 bundle 大小警告为既有状态，本次增加约 6 KiB 压缩前代码，未实施路由级拆包。
- Windows 文件系统临时锁仍可能使完整 Go 套件偶发失败；本次相关包、目标复跑和 Linux 相关包验证均通过。
