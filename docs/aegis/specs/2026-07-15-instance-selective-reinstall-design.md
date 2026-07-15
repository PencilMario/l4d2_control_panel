# Instance Selective Reinstall Design

## Intent

Each game instance's manual update action must let the administrator choose whether to reinstall the L4D2 game files, the instance's selected plugin package, or both. Both choices are forced reinstall operations rather than update checks.

## User Experience

The update action on an instance opens a confirmation dialog containing two checkboxes:

- **Reinstall game files**: run the existing SteamCMD `app_update 222860 validate` workflow.
- **Reinstall instance plugin package**: fully deploy the package currently selected by the instance, even when that exact package is already applied.

Both options are selected by default. The confirmation action is disabled when neither option is selected. The dialog describes that the instance will be stopped while the selected components are reinstalled and then returned to its prior desired state.

The content repository's independent package hot-update and full-update controls remain unchanged.

## API Contract

The instance game-update endpoint becomes a selective reinstall endpoint while retaining its existing route:

```json
{
  "confirm": true,
  "reinstall_game": true,
  "reinstall_package": true
}
```

`confirm` remains required. At least one reinstall field must be true. For compatibility, a request that omits both new fields retains the old behavior and reinstalls only the game files. A request that explicitly supplies both fields as false is rejected with a validation error.

The endpoint creates one serialized instance Job. The Job type remains compatible with existing job displays and event consumers, while its stages and messages identify the selected reinstall work.

## Execution Model

A composite coordinator owns the manual reinstall workflow. It reads the latest instance state, records whether the desired state is running, and stops an active instance at most once.

Selected work runs in this order:

1. Reinstall and validate the game files when `reinstall_game` is selected.
2. Fully deploy the instance's current `SelectedPackageID` when `reinstall_package` is selected.
3. Replay the instance private layer after all selected lower-layer writers complete.
4. Restore the instance to its latest desired state.

The plugin package is never replaced with a newer package ID during this flow. No GitHub Release lookup or version comparison occurs. A missing or invalid selected package is an error when package reinstall was requested.

The composite coordinator must avoid the existing standalone coordinators independently stopping and restarting the instance. Existing game-only, package hot-update, package full-update, provisioning, scheduled update, and content repository flows keep their current contracts.

## State And Failure Handling

The instance enters the updating state before filesystem mutation. A successful reinstall preserves `SelectedPackageID` and records the same package ID as applied after the forced full deployment commits.

Game reinstall errors use the existing game-update fault behavior. Package deployment keeps the existing transaction journal and rollback behavior. If package deployment fails after a successful game reinstall, the package transaction rolls back and the Job fails; the completed Steam validation is not rolled back.

On failure, the instance is marked faulted consistently with the existing game update workflow. The Job error identifies the failed stage. Recovery must use fresh instance reads so concurrent desired-state changes are not overwritten.

## Compatibility Boundary

- Preserve the existing `/api/instances/{id}/game-update` route and old request behavior when the new fields are omitted.
- Preserve per-instance Job serialization and existing Job JSON/SSE shapes.
- Preserve SteamCMD maintenance-container adoption and retry behavior.
- Preserve package transaction journaling, rollback, content precedence, and private-file replay.
- Preserve independent content repository hot/full deployment actions and scheduled tasks.
- Do not add automatic release checks, package selection, or version comparison to this workflow.

## Testing

Backend tests must cover game-only, package-only, combined reinstall, omitted-field compatibility, explicit empty selection rejection, forced redeployment of an already-applied package, one stop/start cycle for combined work, selected package lookup failure, stage failure, rollback, desired-state preservation, and private replay ordering.

Frontend tests must cover both options being selected by default, request payloads for each selection combination, disabled confirmation for an empty selection, and updated destructive-operation copy.

The browser acceptance path must verify that an administrator can open an instance update dialog, choose either component or both, submit the operation, and observe one background Job.

## Working Drafts

### TaskIntentDraft

Add per-instance selective forced reinstall for game files and the currently selected plugin package. This is a cross-module, user-visible contract and orchestration change with lifecycle and rollback risk.

### BaselineReadSetHint

The design is constrained by `CONTEXT.md`, `README.md`, `internal/updates/game.go`, `internal/updates/coordinator.go`, `internal/updates/pipeline.go`, `internal/httpapi/server.go`, `web/src/app/App.tsx`, and their focused tests.

### ImpactStatementDraft

Affected owners are the instance update API, update coordination, package repository/deployment, lifecycle state, jobs, and the instance-list UI. The central invariant is that combined work is one serialized operation with at most one lifecycle stop/start cycle. Automatic package discovery and unrelated content workflows are non-goals.
