# Todo Checkpoint Draft

Updated: 2026-07-15

## Current Todo

- Active: Task 6, build the shared React configuration modal and preview.
- Next: Task 7, update the real-browser journey and operational checks.
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
- Task 5 completed: strict create/update validation and one-Job reconfiguration are covered by HTTP integration tests.

## Evidence Refs

- `docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md`
- `docs/aegis/plans/2026-07-15-instance-startup-packages.md`

## Blockers

- None.

## Drift Check Draft

- Scope: unchanged.
- Compatibility: route paths, strict decoding and persistent Job polling remain unchanged; name-only edits now avoid unnecessary downtime.
- Retirement: unconditional installed-instance rebuild has been replaced by runtime/package diff planning.
- Decision: continue.

## Next Step

Load frontend-design and React performance guidance, then write failing shared-modal, preview, create/edit payload and Job-response tests.
