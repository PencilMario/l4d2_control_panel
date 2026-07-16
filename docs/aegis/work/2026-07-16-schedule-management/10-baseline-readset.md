# BaselineReadSetHint

- `docs/aegis/specs/2026-07-16-schedule-management-design.md`: approved behavior and compatibility boundary.
- `CONTEXT.md`: project language for game instances and state.
- `internal/domain/models.go`: `ScheduledTask` JSON contract.
- `internal/scheduler/service.go`: canonical save, reschedule, disable, delete, and run owner.
- `internal/automation/dispatcher.go`: exact task-type dispatch, payload, and player-policy semantics.
- `internal/maintenance/manager.go`: backup and cleanup effects.
- `internal/updates/game.go`, `internal/updates/coordinator.go`, `internal/updates/pipeline.go`: update interruption and rollback behavior.
- `internal/httpapi/server.go`, `internal/httpapi/server_test.go`: schedule HTTP routes and contract tests.
- `web/src/app/App.tsx`, `web/src/app/App.test.tsx`, `web/src/styles/app.css`: current schedule UI and shared dialog/action patterns.
- `web/e2e/control-panel.spec.ts`: real HTTP desktop/mobile administration journey.

Baseline evidence:

- `npm test -- --run`: 6 files, 96 tests passed.
- `go test -p 1 ./internal/scheduler ./internal/httpapi -count=1`: both packages passed.
