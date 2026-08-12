# Todo Checkpoint

## Current state

- Active slice: 安可部署与运行时验证。
- Completed: 基线读取、worktree 隔离、TDD red/green、生成器、Manager 持久化、Panel/Compose/配置接入、文档更新、远端镜像构建与 Panel 重建。
- Remaining: 本地 `main` 尚未合并符号生成分支；保留用户在 `deployment_test.go` 和 `internal/config/config.go` 的未提交修改。
- Blocked on: 安可数据目录当前没有可用于现场复现的完整新游戏模块流程；已有运行时扫描证据已确认符号生成链路工作。

## Drift check

- Scope: remains within local symbol generation and crash analysis storage.
- Compatibility: legacy Accelerator symbol uploads remain accepted, including MODULE records without debug file.
- Retirement: no Throttle path or new public symbol source was added.
- Decision: continue only with controlled branch integration after reviewing the preserved local changes.

## Deployment evidence

- 安可源码 revision：`5fe9de4`。
- Panel 镜像：`sha256:e5a919a24e274326b1f350bca9877995993f34e91afc808c505bd6e85d8b4262`。
- 容器状态：`l4d2-control-panel-panel-1` 为 `healthy`；`/api/health` 返回 `status=ok`、`database=ok`。
- 容器内工具：`/usr/local/bin/dump_syms` 与 `/usr/local/bin/minidump_stackwalk` 均存在，`dump_syms --help` 正常输出。
- 启动扫描：`scanned=105543 candidates=396 generated=248 skipped=0 duplicates=142 failed=6`；6 个失败项均为超大 `libcef.so`/`steamclient.so`，原因是符号输出超过大小上限。
- Playwright：带缓存破坏参数的桌面页面加载成功，JS/CSS 均 200，控制台错误 0；崩溃报告页面的列表、详情和 stackwalk 下载接口均 200。移动视口未形成有效证据，浏览器桥接实际保持约 1028px 宽度。
