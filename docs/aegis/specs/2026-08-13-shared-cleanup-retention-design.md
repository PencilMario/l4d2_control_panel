# Shared Cleanup Retention Design

- 状态：已批准
- 日期：2026-08-13
- 范围：清理计划任务、游戏日志清理、崩溃转储清理和系统设置

## 1. 目标

将 `clear` 计划任务的 `payload.retention_days` 作为 Panel 文件清理的唯一保留期限。一次 `clear` 执行同时清理实例备份、上传残留、历史插件包、游戏日志和崩溃转储。

系统设置中的游戏日志策略只保留单个日志文件最大大小；游戏日志和崩溃转储的保留天数不再单独配置。

## 2. 执行语义

- `retention_days` 缺失或小于 1 时沿用执行器默认值 30 天。
- 同一 `clear` 执行把相同天数传给维护目录、游戏日志和崩溃转储清理器。
- pending crash submission token 继续固定保留 24 小时，不受计划天数影响。
- 清理器继续保护正在使用的文件、被报告引用的派生 artifact 和内置符号。
- 三类清理都尝试执行；任一阶段失败都会记录任务日志并使计划任务失败。

## 3. 退役行为

- 移除 `L4D2_PANEL_CRASH_RETENTION_DAYS` 运行配置及部署注入。
- 移除崩溃转储启动时清理和每 24 小时独立清理。
- 移除游戏日志独立每日 Cron、按实例 cleanup Job 和手动立即清理 API。
- 移除系统设置 API/UI 中的游戏日志保留天数。

没有启用 `clear` 计划任务时，游戏日志和崩溃转储不会由这些组件自动过期删除。

## 4. 接口与兼容性

`GET /api/settings/game-logs` 和 `PUT /api/settings/game-logs` 只返回或接受 `max_file_size_mb`。严格 JSON 解码会拒绝已经退役的 `retention_days` 字段，避免客户端继续以为该策略仍由系统设置拥有。

计划任务的数据库结构、Cron API、任务类型和 `payload.retention_days` 字段保持不变。旧的 cleanup 计划缺少该字段时仍按 30 天执行。

## 5. 测试验收

- Go 测试验证 `clear` 将同一个天数传给维护、游戏日志和崩溃转储清理，并验证阶段错误合并。
- CrashReports 测试验证显式天数、范围校验和独立清理入口已删除。
- GameLogs/Store/HTTP 测试验证仅保留文件大小设置和清理器的显式天数。
- React 测试验证游戏日志设置不再显示保留天数或立即清理按钮，cleanup 计划编辑可以更新保留天数。
- 运行全部 Go 测试、`go vet`、前端单测和生产构建。

## 6. 设计输入与影响

### TaskIntentDraft

让游戏日志和崩溃转储跟随同一个清理计划保留期限，避免系统设置、独立定时器和计划任务之间出现不同步的清理策略。

### BaselineReadSetHint

参考 `internal/automation/dispatcher.go`、`internal/maintenance/manager.go`、`internal/gamelogs/scheduler.go`、`internal/crashreports/manager.go`、`internal/store/job_history.go`、`internal/httpapi/game_logs.go`、`cmd/panel/main.go`、`web/src/app/App.tsx` 和现有计划任务/清理测试。

### ImpactStatementDraft

影响自动化执行器、游戏日志清理服务、崩溃报告管理器、普通系统设置存储/API、Panel 启动 wiring、计划任务编辑页和相关测试。保留现有文件安全检查、日志大小限制、清理任务执行模型和 crash report 接收/分析/下载行为；不改变备份、插件包或报告存储格式。
