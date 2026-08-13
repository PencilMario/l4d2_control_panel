# Evidence

## Verified

- `npm test -- --run` -> 21 个测试文件、215 个测试通过。
- `npm run build:web` -> TypeScript 编译和 Vite 生产构建通过。
- `go vet ./...` -> 通过。
- `go test -tags=e2e ./cmd/e2e-fixture -count=1` -> 通过。
- `go test ./internal/config -count=1` -> 通过。
- `go test ./internal/httpapi -run 'TestGameLogsHTTPContract|TestPutGameLogSettingsUpdatesOnlyMaxFileSize' -count=1` -> 通过。
- `go test ./internal/automation -count=1` -> 通过。
- `go test ./internal/gamelogs -count=1` -> 通过。
- `go test ./internal/crashreports -count=1` -> 通过。
- `go test ./internal/store -count=1` -> 通过。
- `go test ./cmd/panel -count=1` -> 通过。
- `git diff --check` -> 通过。

## Residual Risk

- `go test ./... -count=1` 在 Windows 上偶发多个无关包的 `testing.TempDir` 清理阶段 `directory is not empty`，失败位置在 `internal/content`、`internal/httpapi`、`internal/updates`，不是本次改动的清理链路。
- `npm run e2e` 能启动 fixture；A2S 日志和主流程中已有测试存在共享 fixture/端口和文本时序问题，未将这些无关流程改造成新的产品行为。
- E2E 构建生成的 `web/public/vpk-cleaner.wasm` 已恢复，不属于本次改动。

## Scope

验证覆盖共享 retention 数据流、游戏日志设置 HTTP/React 契约、cleanup 计划编辑、crash cleanup 显式参数、Panel/fixture wiring 和构建部署配置。未改变 crash report 接收、分析、下载、artifact 引用保护或旧数据库中遗留的 `game_log_retention_days` 键。
