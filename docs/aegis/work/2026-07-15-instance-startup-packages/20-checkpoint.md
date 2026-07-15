# Todo Checkpoint Draft

Updated: 2026-07-15

## Current Todo

- Active: Task 8, review the complete branch and run the completion audit.
- Next: record final review evidence and complete the feature branch workflow.
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
- Task 7 completed: the desktop/mobile browser journey uploads two packages, creates with package A and extra args, verifies first-start application, edits to package B through one reconfiguration Job, and preserves all existing operational workflows.

## Evidence Refs

- `docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md`
- `docs/aegis/plans/2026-07-15-instance-startup-packages.md`

## Blockers

- None.

## Drift Check Draft

- Scope: unchanged.
- Compatibility: route paths, strict decoding and persistent Job polling remain unchanged; name-only edits now avoid unnecessary downtime.
- Retirement: the old browser journey that created an instance before any package existed has been replaced by the required package-first flow.
- Decision: continue.

## Next Step

Map the approved design to the implementation, audit changed code for contract drift and placeholders, then run final verification under `verification-before-completion`.
