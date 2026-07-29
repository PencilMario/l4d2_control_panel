# Detailed Background Task Logging Design

## Intent

Make every background task shown in the task center operationally understandable from its persisted log. An administrator must be able to identify the target, relevant non-sensitive inputs, current work, affected files or releases, result, duration, and failure context without reproducing the task.

## Scope

- Cover every operation started through the shared job manager, including manual, scheduled, global, instance-scoped, migration, maintenance, and deferred-restart tasks.
- Preserve the existing job records, event stream, JSONL task log, SSE API, frontend log viewer, redaction, and terminal log-size limit.
- Add shared helpers for consistent phase, duration, byte-size, file-operation, and throttled transfer messages.
- Add task-specific semantic logs at the component that owns each operation.

This includes instance installation, lifecycle actions, game updates and reinstalls, plugin deployment, GitHub source synchronization, backups, restores, deletion, private-file application, shared-game migration, log maintenance, scheduled execution, and shared-VPK restart.

## Logging Contract

Every task records:

1. A start record containing the task type, target identifier or global scope, and relevant non-sensitive parameters.
2. Phase records when meaningful work begins or completes. Completed phase records include elapsed time and useful counts or byte totals when available.
3. Object-level records for materially affected releases, assets, archives, containers, instances, and files.
4. A success summary containing total elapsed time and the main result.
5. A failure summary containing task type, active phase, elapsed time, and the wrapped error context.

Structured operational records use `info`, recoverable or degraded conditions use `warn`, failures use `error`, and unstructured subprocess output remains `output`. Logging failures never fail the managed operation.

Paths in task logs are relative to a controlled content, instance, game-log, backup, or temporary root. Tokens, passwords, authorization headers, authenticated URLs, and sensitive query parameters are never intentionally included and remain protected by the existing log redactor.

## Transfer Progress

HTTP and other measurable file downloads record an unthrottled start message with the source kind and destination filename. During transfer they emit at most one progress record every five seconds and only when the transferred byte count has increased. A progress record contains:

- filename;
- transferred bytes in human-readable and exact-byte form;
- total size and percentage when known;
- recent transfer rate;
- estimated remaining time when total size and a usable rate are known.

Unknown or invalid content lengths omit total size, percentage, and ETA. Completion is unthrottled and records final bytes and elapsed time. Cancellation and copy failures retain the last known byte count in their error context. Progress measurement wraps the stream and does not alter downloaded bytes or task cancellation behavior.

## Task-Specific Detail

### GitHub Sources

GitHub synchronization records the configured source name and ID when available, repository, selected Release name, tag, publication time, matched asset filename, advertised asset size, and final downloaded size. It records whether the concrete package version was downloaded, replaced, or reused. API credentials and signed asset URL parameters are excluded.

### Cleanup And File Maintenance

Cleanup records its retention or size policy before scanning. Each successful deletion or trim records the action, safe relative path, original size, resulting size when applicable, and released bytes. Each failed or skipped material action records the relative path and reason. The final summary retains existing scan, deletion, trim, skip, released-byte, and failure counts.

### Installation, Update, Deployment, And Migration

These tasks identify the instance or shared-game target, selected components, package or release version, relevant preflight decisions, stop/start or rebuild actions, deployment phases, and rollback or recovery actions. External SteamCMD and container output remains available alongside higher-level phase records.

### Lifecycle, Backup, Restore, Delete, And Deferred Work

Lifecycle tasks record the instance, requested transition, current decision where available, container action, and resulting state. Backup and restore tasks record safe archive names, included scope, byte size, and destination or source category. Delete tasks enumerate materially removed managed resources without exposing absolute host paths. Deferred restart tasks record why the restart was queued, readiness checks, restart action, and completion.

### Scheduled Tasks

Scheduled execution records the schedule ID, task type, target, online-player policy, and parsed non-sensitive payload. Waiting, forced execution, or skipping due to players is explicit. The nested operation uses the same task log context so its semantic details appear in the scheduled job.

## Implementation Boundaries

The shared job manager owns task-wide start, success, failure, active-phase, and elapsed-time summaries. Shared logging helpers own deterministic formatting and transfer throttling. Domain components own semantic object-level records because only they know what was selected, changed, removed, or reused.

Existing `jobs.Reporter` calls remain supported. New context-based helpers compose with the reporter already attached by the job manager, avoiding API changes across HTTP and scheduler boundaries. Existing raw output capture remains intact.

The final 10 MiB task-log limit and current terminal compaction behavior remain unchanged. Detailed logs may be truncated under this existing policy; the newest records and truncation marker remain available.

## Error Handling

- A task failure retains the original returned error and status behavior.
- Additional context wraps errors at the owning operation boundary without leaking credentials.
- Per-file cleanup failures follow the current continue-or-abort semantics; logging does not change partial-failure behavior.
- Missing metadata produces a reduced but valid log message rather than failing the task.
- A task-log append failure is reported through the panel process log and never changes the managed task result.

## Verification

- Job-manager tests prove task-wide start, completion, failure, phase, and elapsed-time records while preserving status and event contracts.
- Logging-helper tests prove byte formatting, safe relative labels, credential exclusion, and deterministic messages.
- Transfer tests use a controllable clock to prove the five-second interval, byte-change requirement, known and unknown totals, completion output, rate, and ETA behavior.
- GitHub release tests prove Release name, tag, publication time, asset name and size, and reuse/download decisions are logged.
- Cleanup tests prove every deleted, trimmed, skipped, and failed file is identified safely and the existing summary remains correct.
- Task-entry tests cover each registered task family and prove target/configuration context plus success or failure summaries.
- Related Go integration tests verify that existing job APIs and live log streaming continue to expose the new records without frontend contract changes.

## Compatibility And Non-Goals

No job, event, SSE, or frontend response schema changes are introduced. Existing task status, percentage, scheduling, locking, cancellation, retries, cleanup semantics, and terminal log retention remain unchanged.

This work does not add distributed tracing, configurable log verbosity, a new frontend log format, permanent storage beyond the existing cap, or logging for recurring internal services that are not represented as task-center jobs.

## Design Inputs

### Task Intent Draft

The requested outcome is substantially more detailed logs for every task-center background task. Downloads, GitHub sources, and cleanup are examples and define minimum detail rather than the complete scope. The main risks are excessive log volume, credential leakage, and inconsistent ad hoc messages.

### Baseline Read Set Hint

The design follows the existing contracts in `internal/jobs`, `internal/joblogs`, task entry points in `internal/httpapi`, scheduled dispatch in `internal/automation`, download behavior in `internal/releases` and `internal/docker`, and file maintenance in `internal/gamelogs` and `internal/maintenance`. `CONTEXT.md` defines the project terms used for game instances, plugin sources, deployed packages, and instance plugin updates.

### Impact Statement Draft

The change affects shared task logging plus domain-owned task operations. Compatibility requires preserving job persistence, SSE delivery, redaction, task results, and the frontend contract. UI redesign, task scheduling changes, and logging for non-job background loops are explicit non-goals.
