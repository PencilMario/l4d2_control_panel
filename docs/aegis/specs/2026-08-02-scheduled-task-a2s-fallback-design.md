# Scheduled Task A2S Failure Fallback Design

## Intent

Prevent scheduled tasks using the `wait` online policy from blocking an instance job queue indefinitely when player discovery fails.

## Behavior

- `wait`: when the player query succeeds with online players, continue waiting; when it succeeds with no players, execute; when it fails, log the failure and execute immediately.
- `skip`: preserve the existing behavior, including failing the scheduled task when player discovery cannot prove that the instance is empty.
- `force`: preserve the existing behavior and execute without querying players.
- A stopped game instance remains an explicit early bypass because it cannot contain online players.

"Execute immediately" only bypasses the player-wait gate. The scheduled operation still uses the existing job lock, dispatcher, progress, error, and terminal-status behavior.

## Ownership And Flow

`internal/automation/dispatcher.go::waitForPlayers` remains the single owner of scheduled-task player policy. No retry worker, fallback owner, API, schema, or configuration is added.

For `wait`, each player check has three outcomes:

1. Online players found: log and retry after one minute.
2. No players found: log and return success from the gate.
3. Query error: log a warning that execution is being forced, then return success from the gate.

## Compatibility

Task types, schedules, payloads, APIs, persistence, instance locking, and update behavior remain unchanged. The only retired behavior is unbounded waiting after an A2S/player-query failure under the `wait` policy.

## Verification

- Add a regression test proving `wait` plus a player-query error returns immediately.
- Preserve or add coverage proving `wait` plus online players still waits.
- Preserve `skip` and `force` behavior.
- Run the automation package tests and the full Go test suite.
- After deployment, verify the panel is healthy, the deployed binary contains the new log marker, and no jobs remain unexpectedly pending or running.

## Working Drafts

- **TaskIntentDraft:** Apply the A2S-failure force-run rule to every instance-scoped scheduled task that uses `wait`.
- **BaselineReadSetHint:** `CONTEXT.md`, `internal/automation/dispatcher.go`, `internal/automation/dispatcher_test.go`, and the current stopped-instance hotfix diff.
- **ImpactStatementDraft:** The automation player-policy gate changes; job contracts and task implementations do not. The primary risk is executing while players are online during an A2S outage, which is explicitly accepted by this policy.
