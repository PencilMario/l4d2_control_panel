# Todo Checkpoint Draft

Updated: 2026-07-15

## Current Todo

- Active: Task 5, extend strict instance create/update APIs.
- Next: Task 6, build the shared React configuration modal and preview.
- Later: provisioning, package intent preservation, HTTP reconfiguration, React UI, E2E, completion audit.

## Completed

- Approved reusable design committed on `main` as `01baa4b`.
- Isolated worktree and branch created.
- Baseline Go tests, frontend tests and production build passed.
- Implementation plan and task records committed as `8ddde5d`.
- Task 1 completed: selected/applied package identities now round-trip independently and legacy rows backfill safely.
- Task 2 completed: Go owns argument validation/order and new containers pass validated JSON argv to Supervisor.
- Task 3 completed: maintenance installation and package deployment finish before the first game container is created.
- Task 4 completed: package updates preserve stopped/running intent and Coordinator owns applied-state writes.

## Evidence Refs

- `docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md`
- `docs/aegis/plans/2026-07-15-instance-startup-packages.md`

## Blockers

- None.

## Drift Check Draft

- Scope: unchanged.
- Compatibility: hot/full routes and Pipeline transactions remain unchanged; stopped instances no longer start as a side effect.
- Retirement: HTTP no longer duplicates applied-package persistence; Coordinator is the single deployment-state owner.
- Decision: continue.

## Next Step

Write and run failing strict create/update tests for package validation, extra arguments, name-only edits and one-Job reconfiguration.
