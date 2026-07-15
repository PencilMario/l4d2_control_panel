# Todo Checkpoint Draft

Updated: 2026-07-15

## Current Todo

- Active: Task 2, canonical SRCDS argv parsing and JSON transport.
- Next: Task 3, provision game and package before first start.
- Later: provisioning, package intent preservation, HTTP reconfiguration, React UI, E2E, completion audit.

## Completed

- Approved reusable design committed on `main` as `01baa4b`.
- Isolated worktree and branch created.
- Baseline Go tests, frontend tests and production build passed.
- Implementation plan and task records committed as `8ddde5d`.
- Task 1 completed: selected/applied package identities now round-trip independently and legacy rows backfill safely.

## Evidence Refs

- `docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md`
- `docs/aegis/plans/2026-07-15-instance-startup-packages.md`

## Blockers

- None.

## Drift Check Draft

- Scope: unchanged.
- Compatibility: additive database migration passed store and HTTP regressions.
- Retirement: `PackageVersion` now retains applied-state ownership; remaining callers will move desired state to `SelectedPackageID` in later slices.
- Decision: continue.

## Next Step

Write and run failing SRCDS parser, reserved-option, Docker JSON transport and runtime argv tests.
