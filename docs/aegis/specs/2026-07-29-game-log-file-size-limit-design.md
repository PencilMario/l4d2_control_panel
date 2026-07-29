# Game Log File Size Limit Design

## Intent

Add a configurable maximum size for each persistent game log file. The default is 10 MiB. Periodic and manual game-log maintenance retain the newest bytes of oversized files while preserving the existing age-retention behavior.

## Scope

- Extend game-log settings with `max_file_size_mb`, defaulting to 10 and accepting integer values from 1 through 1024.
- Apply the limit to regular files below each instance's `logs/game` and `logs/sourcemod` roots.
- Reuse the existing daily 03:00 cleanup job and the existing immediate-cleanup endpoint.
- Queue immediate maintenance when retention days or maximum file size becomes more restrictive.
- Report trimmed-file count and released bytes in maintenance summaries.

## Behavior

Maintenance first deletes files older than `retention_days`, then inspects the remaining regular files. A file whose size is greater than the configured byte limit has its newest `max_file_size_mb * 1024 * 1024` bytes copied to the beginning of the same open file before it is truncated. Keeping the same file identity allows a game process with an existing file handle to continue writing to the maintained log. A file exactly at the limit is unchanged.

Directory validation remains anchored to the controlled instance root. Symbolic links and special files are skipped. Before trimming, maintenance verifies that the pathname and open handle still identify the inspected file; rotations or replacements become skips or reported failures rather than overwriting an unrelated file. Writes from the external game process cannot share the panel's lock, so the size is an eventual maintenance limit rather than a hard real-time cap.

## Contracts

`GET /api/settings/game-logs` returns both `retention_days` and `max_file_size_mb`. `PUT /api/settings/game-logs` requires and persists both fields and returns both fields plus the existing enqueue statistics. Existing game-log tree, preview, and download contracts remain unchanged.

The settings UI presents a numeric “单个日志文件最大大小（MB）” control beside retention days. Saving, validation, pending-state behavior, and failure rollback follow the existing game-log settings interaction.

## Verification

- Store tests prove the 10 MiB default, persistence, and range validation.
- Manager tests prove tail retention, exact-boundary behavior, age deletion plus trimming, and filesystem safety.
- Scheduler/API tests prove configuration propagation and immediate enqueue rules.
- React tests prove loading, validation, and save payload behavior.
- Full Go tests and frontend test/build commands remain green.

## Compatibility And Non-Goals

Age retention remains enabled and unchanged. The panel does not intercept game-process writes, add log rotation, trim task/job logs, or guarantee a hard real-time disk cap between maintenance runs.
