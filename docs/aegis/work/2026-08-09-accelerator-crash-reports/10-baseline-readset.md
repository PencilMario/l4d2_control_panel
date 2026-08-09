# Baseline Read-Set

## Authority

- `CONTEXT.md`: 项目术语和实例数据边界。
- `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`: 单机 Docker、SQLite、认证、数据持久化和安全约束。
- `docs/aegis/specs/2026-07-18-persistent-game-log-viewer-design.md`: 游戏日志与 crash dump 解耦的持久化惯例。
- `internal/httpapi/server.go`: Chi 路由、认证分组、响应和输入限制惯例。
- `internal/config/config.go`: 数据根目录与环境配置惯例。
- `cmd/panel/main.go`: 服务组装、生命周期和外层 `/api` mux。
- `docker-compose.yml`: Panel 运行用户、数据卷和网络边界。

## External protocol evidence

- `asherkin/accelerator/extension/extension.cpp`: `/submit` 预提交响应 `Y|<module decisions>|<token>`；multipart 字段 `upload_file_minidump`、`upload_file_metadata`、`GameDirectory`、`ExtensionVersion`、`ServerID`、`PresubmitToken`；符号字段 `symbol_file`；二进制字段 `code_file`。

## Baseline verification

- `go test ./internal/httpapi ./internal/store ./cmd/panel ./internal/gamelogs`: passed.
- `go test ./...`: first 120-second attempt timed out while compiling; final verification must rerun with a longer timeout.
