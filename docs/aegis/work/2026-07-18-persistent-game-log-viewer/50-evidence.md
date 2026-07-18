# 持久游戏日志查看器证据

- 日期：2026-07-18
- 验证分支：`feature/persistent-game-logs`
- 最终功能提交：`e658a5f` 及其祖先
- Linux 验证主机：SSH `sirphomesv`
- Linux 验证方式：Docker，不使用 WSL

## 验证结果

### Linux Go

在 `sirphomesv` 的独立 Git bundle clone 中使用 `golang:1.25`：

```text
go test -count=1 ./...
```

结果：通过。包括 `internal/gamelogs`、`internal/httpapi`、`internal/lifecycle`、`internal/store`、`internal/content` 与 `internal/safepath`。

```text
go vet ./...
```

结果：通过，无输出，退出码 0。

远端首次运行发现并修复两个既有 portable path owner：Linux 对 Windows 盘符/反斜杠的解释与 Windows 不同。最终 `safepath` 和私有 manifest 均使用跨平台语义，远端全量回归通过。

### Frontend

在 `sirphomesv` 的 `node:22-bookworm` 容器中：

```text
npm ci
npm test -- --run
npm run build
```

结果：12 个测试文件、149 项测试通过；TypeScript 与 Vite 生产构建通过。

构建仍报告既有单 chunk 超过 500 kB 警告；本功能没有保留新增的 ANSI 依赖，日志查看器使用受控 tokenizer。该警告不阻止构建。

### Playwright

在 `sirphomesv` 使用 `mcr.microsoft.com/playwright:v1.61.1-noble`，fixture 由 Go 1.25 Linux 二进制提供。desktop 与 mobile 分别使用全新 fixture，避免跨项目状态污染：

```text
npx playwright test --config=playwright.remote.config.ts --project=desktop --grep "real HTTP administration journey"
npx playwright test --config=playwright.remote.config.ts --project=mobile --grep "real HTTP administration journey"
```

结果：desktop 1/1 通过，mobile 1/1 通过。

主旅程覆盖：浏览 SourceMod 嵌套日志、语义高亮、下载、插件重装后同一日志仍存在、按 14 天清理 aged 日志、保留 recent 日志、打开 `cleanup_game_logs` 完整任务日志并验证 `Scanned`、`Deleted=1` 与 `ReleasedBytes`。

## Remove / Restore

- 旧 Overlay 日志路径只作为首次幂等迁移来源，不作为运行时回退 owner。
- 日志持久目录由 `internal/gamelogs` 唯一准备，lifecycle 不再重复创建。
- E2E fixture 只显式初始化一次日志；Start、Rebuild 和后续 update 不会自动补种，避免掩盖持久性回归。
- 远端测试未改生产服务；只创建独立 bundle clone、测试 cache、fixture 进程和测试镜像。

## Residual Risk

- 单 Panel 进程内清理任务去重有互斥和持久 active 查询；多个 Panel 进程共享同一数据根不属于支持的部署模型，查询与提交不是跨进程原子 claim。
- 跨平台标准 Go API 无按 `FileInfo` 原子 unlink；清理在删除前使用 `Lstat`/`SameFile` 复核，仍保留极窄的路径替换窗口。
- 前端生产 bundle 的既有大小警告仍在，本次未做无关拆包。

## Confidence

等级：A。核心功能有 Linux 全量 Go 回归、前端全量测试、生产构建及 desktop/mobile 真实浏览器主旅程直接证据。该证据是验证结果，不替代仓库所有者的合并授权。
