# Detailed Background Task Logging Evidence

## Outcome

All task-center jobs now receive common start, success, and failure summaries containing task type, target, phase, duration, and error context. Domain owners add task-specific records for downloads, GitHub releases, cleanup, backup, lifecycle operations, package/game updates, shared-game migration, schedules, player actions, private-file application, and deferred VPK restarts.

## Registration Audit

Production job registrations were enumerated with:

```text
rg -n "\.Start\(|startJob\(" internal cmd -g '*.go'
```

Coverage:

- `internal/httpapi/server.go`: reconfigure, delete, instance package update, global game update, shared-game migration, raw GitHub fetch, configured GitHub source check, private apply, player kick/ban, lifecycle start/stop/restart, and startup migration.
- `internal/automation/dispatcher.go`: all scheduled game/package/release/backup/cleanup tasks, including task ID, target, policy, safe payload values, source resolution, and player wait/skip decisions.
- `internal/gamelogs/scheduler.go`: policy, per-file deletion/trim/failure detail, and aggregate result.
- `internal/vpkrestart/coordinator.go`: registration reason, readiness checks, query retries, restart, and completion.
- `internal/jobs/manager.go`: common summaries cover every registration, including paths with sparse domain logging.

## Transfer And File Audit

Direct `io.Copy`, `os.Remove`, and `FetchLatest` sites were enumerated. GitHub Release asset download is the task-owned measurable network transfer and now uses five-second throttled progress. SteamCMD owns game downloads externally; its raw output remains captured and the update coordinators now add semantic phase records. Other copies/removals belong to synchronous uploads, preview/export, atomic replacement, rollback, or internal cleanup and are not separate task-center downloads.

Game-log and retained-backup cleanup log safe relative filenames and sizes at their canonical action points. Private-file application logs every affected logical path. Absolute managed roots, GitHub tokens, authorization headers, and signed asset URL query values are excluded.

## Verification

Targeted RED/GREEN evidence:

- `go test ./internal/jobs -count=1`
- `go test ./internal/jobs ./internal/releases -count=1`
- `go test ./internal/gamelogs ./internal/maintenance -count=1`
- `go test ./internal/lifecycle ./internal/updates ./internal/migration -count=1`
- `go test ./internal/httpapi ./internal/automation ./internal/content ./internal/players ./internal/vpkrestart -count=1`

Final verification on 2026-07-30:

```text
go test -p 1 ./...    PASS
go vet -p 1 ./...     PASS
git diff --check      PASS
```

An initial parallel `go test ./...` completed all assertions but Windows failed cleanup of one random `t.TempDir`. The affected test changed between runs. Each affected target passed five consecutive runs, `internal/content` passed three full runs, and `internal/httpapi` passed two full runs. Serial package execution passed the complete suite, so no product workaround was added for the environment-specific directory cleanup race.

## Compatibility

- Job records, job events, status, percent, SSE, and frontend schemas are unchanged.
- Existing subprocess `output` records and redaction remain active.
- Operation locking, cancellation, update rollback, cleanup partial-failure handling, and private-file journaling remain unchanged.
- Existing 10 MiB finalized task-log compaction remains the volume boundary.

## Residual Risk

SteamCMD does not expose a stable byte-level stream to the panel, so game downloads rely on its detailed output rather than calculated speed and ETA. GitHub downloads without a valid content length intentionally omit percentage and ETA while retaining filename, transferred bytes, rate, and completion size.
