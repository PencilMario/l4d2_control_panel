# Todo Checkpoint Draft

Updated: 2026-07-15

## Current Todo

- Active: none; Tasks 1-8 are implemented and verified.
- Next: complete the feature branch workflow and choose integration handling.
- Later: real-host Docker/Steam acceptance when a disposable Linux host is available.

## Completed

- Approved reusable design committed on `main` as `01baa4b`.
- Isolated worktree and branch created.
- Baseline Go tests, frontend tests and production build passed.
- Implementation plan and task records committed as `8ddde5d`.
- Task 1 completed: selected/applied package identities now round-trip independently and legacy rows backfill safely.
- Task 2 completed: Go owns argument validation/order and new containers pass validated JSON argv to Supervisor.
- Task 3 completed: maintenance installation and package deployment finish before the first game container is created.
- Task 4 completed: package updates preserve stopped/running intent and Coordinator owns applied-state writes.
- Task 5 completed: strict create/update validation and one-Job reconfiguration are covered by HTTP integration tests.
- Task 6 completed: create/edit share one controlled modal with package selection, editable managed startup values, extra arguments, live command preview and pending package status.
- Task 7 completed: the desktop/mobile browser journey uploads two packages, creates two independently configured instances, verifies first-start application, edits one instance through one reconfiguration Job, and preserves all existing operational workflows.
- Task 8 completed: branch-wide review repaired uninstalled pre-applied provisioning, rejected arbitrary runtime-image input, exposed pending package state immediately, and closed the two-instance browser evidence gap.

## Evidence Refs

- `docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md`
- `docs/aegis/plans/2026-07-15-instance-startup-packages.md`

## Blockers

- None.

## Drift Check Draft

- Scope: unchanged.
- Compatibility: route paths, persistent data, Host networking, fixed Supervisor operations, strict decoding and Job polling remain unchanged.
- Retirement: runtime-image input accidentally introduced by the shared request type is removed; raw `SRCDS_EXTRA_ARGS` remains only as the documented old-container fallback.
- Decision: continue to the feature-branch completion workflow.

## Next Step

Use `finishing-a-development-branch` to present the verified branch integration options.
