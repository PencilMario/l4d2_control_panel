# Baseline Read Set

## Authority and terminology

- `CONTEXT.md`：游戏实例、控制台和缓存相关项目术语。
- 已获用户批准的设计：全局持久化设置，默认 8192，范围 1 至 1,000,000，保存后立即生效。

## Existing owners

- `internal/store/job_history.go`：现有 `system_settings` 表及整数设置读写模式。
- `internal/store/migrations.go`、`internal/store/store.go`：SQLite 初始化和迁移顺序；表在现有迁移中已创建。
- `internal/httpapi/server.go`：认证设置路由、JSON 解码与错误响应模式。
- `internal/httpapi/console.go`：控制台会话历史、1 MiB 字节限制和静态 8192 行默认值。
- `internal/httpapi/console_test.go`、`internal/httpapi/server_test.go`：后端缓存和 WebSocket 集成测试。
- `web/src/app/consoleBuffer.ts`：前端 8192 行默认裁剪逻辑。
- `web/src/app/App.tsx`：`Terminal` 控制台和 `SettingsPage` 系统设置表单。
- `web/src/app/App.test.tsx`、`web/src/app/consoleBuffer.test.ts`：前端控制台与设置交互测试。

## Compatibility boundary

设置接口保持已认证的系统设置路由风格；旧数据库缺失设置值时返回 8192。已有导出按钮、WebSocket 输入输出和后端字节上限保持不变。
