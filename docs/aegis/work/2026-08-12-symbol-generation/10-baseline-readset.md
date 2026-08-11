# Baseline Read Set

- `docs/aegis/specs/2026-08-10-accelerator-management-analysis-design.md`
- `docs/aegis/plans/2026-08-10-accelerator-management-analysis.md`
- `CONTEXT.md`
- `Dockerfile`, `docker-compose.yml`, `.env.example`
- `internal/crashreports/{manager,artifacts,builtin_symbols}.go`
- `internal/crashanalysis/stackwalk.go`
- `internal/config/config.go`
- `cmd/panel/main.go`

## Compatibility boundary

现有 `/submit`、`/symbols/submit`、`/binary/submit` 的 token、loopback、managed-instance 认证和旧 symbol 文本兼容保持不变。自动生成只走本地 Manager 入口，不增加公开上传权限。
