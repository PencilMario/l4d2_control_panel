# Todo Checkpoint Draft

Updated: 2026-07-15

## Current Todo

- Active: Task 7, update the real-browser journey and operational checks.
- Next: Task 8, review the complete branch and run the completion audit.
- Later: final evidence and branch completion.

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

## Evidence Refs

- `docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md`
- `docs/aegis/plans/2026-07-15-instance-startup-packages.md`

## Blockers

- None.

## Drift Check Draft

- Scope: unchanged.
- Compatibility: route paths, strict decoding and persistent Job polling remain unchanged; name-only edits now avoid unnecessary downtime.
- Retirement: the create-only form and content-page-owned package list have been replaced by one shared configuration contract and App-owned package state.
- Decision: continue.

## Next Step

Update the Playwright journey to upload two packages before creation, verify preview and first-start application, then edit to the second package through one reconfiguration Job.
