# Todo Checkpoint Draft

Updated: 2026-07-15

## Current Todo

- Active: Task 4, preserve stopped/running intent during package updates.
- Next: Task 5, extend strict instance create/update APIs.
- Later: provisioning, package intent preservation, HTTP reconfiguration, React UI, E2E, completion audit.

## Completed

- Approved reusable design committed on `main` as `01baa4b`.
- Isolated worktree and branch created.
- Baseline Go tests, frontend tests and production build passed.
- Implementation plan and task records committed as `8ddde5d`.
- Task 1 completed: selected/applied package identities now round-trip independently and legacy rows backfill safely.
- Task 2 completed: Go owns argument validation/order and new containers pass validated JSON argv to Supervisor.
- Task 3 completed: maintenance installation and package deployment finish before the first game container is created.

## Evidence Refs

- `docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md`
- `docs/aegis/plans/2026-07-15-instance-startup-packages.md`

## Blockers

- None.

## Drift Check Draft

- Scope: unchanged.
- Compatibility: production still uses the established anonymous Windows/Linux bootstrap and persistent game bind, now inside a restricted maintenance container.
- Retirement: runtime-owned SteamCMD bootstrap is removed; game containers only run content provisioned by the Panel.
- Decision: continue.

## Next Step

Write and run failing package Coordinator tests for stopped/running intent, rollback and applied-state persistence.
