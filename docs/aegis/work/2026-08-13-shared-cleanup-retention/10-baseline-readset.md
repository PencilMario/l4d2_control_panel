# Baseline Read Set

- `CONTEXT.md`: 项目术语和边界。
- `docs/aegis/specs/2026-07-16-schedule-management-design.md`: cleanup payload 和计划任务编辑契约。
- `internal/automation/dispatcher.go`: 当前 clear 执行器与默认 30 天。
- `internal/maintenance/manager.go`: 维护目录清理范围。
- `internal/gamelogs/scheduler.go`, `internal/gamelogs/manager.go`: 当前游戏日志独立 Cron/Job 和清理实现。
- `internal/crashreports/manager.go`, `internal/crashreports/config.go`: 当前崩溃保留期限和独立清理循环。
- `internal/store/job_history.go`, `internal/httpapi/game_logs.go`: 游戏日志系统设置/API。
- `cmd/panel/main.go`, `cmd/e2e-fixture/main.go`: 运行时 wiring。
- `web/src/app/App.tsx`, `web/src/app/SchedulesPage.tsx`: 设置和计划任务界面。
