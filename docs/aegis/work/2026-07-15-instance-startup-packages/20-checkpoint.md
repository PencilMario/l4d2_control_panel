# Todo Checkpoint Draft

Updated: 2026-07-15

## Current Todo

- Active: Task 3, provision game and package before first start.
- Next: Task 4, preserve stopped/running intent during package updates.
- Later: provisioning, package intent preservation, HTTP reconfiguration, React UI, E2E, completion audit.

## Completed

- Approved reusable design committed on `main` as `01baa4b`.
- Isolated worktree and branch created.
- Baseline Go tests, frontend tests and production build passed.
- Implementation plan and task records committed as `8ddde5d`.
- Task 1 completed: selected/applied package identities now round-trip independently and legacy rows backfill safely.
- Task 2 completed: Go owns argument validation/order and new containers pass validated JSON argv to Supervisor.

## Evidence Refs

- `docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md`
- `docs/aegis/plans/2026-07-15-instance-startup-packages.md`

## Blockers

- None.

## Drift Check Draft

- Scope: unchanged.
- Compatibility: additive database and raw extra-argument fallback both remain covered by full Go regression.
- Retirement: Python-only raw parsing is now a fallback; new Panel container definitions use the Go-validated JSON token list.
- Decision: continue.

## Next Step

Write and run failing first-install order tests for maintenance SteamCMD, package deployment and lifecycle container creation.
