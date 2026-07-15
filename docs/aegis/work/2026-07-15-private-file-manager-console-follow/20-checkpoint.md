# TodoCheckpointDraft

Updated: 2026-07-15 14:18 +08:00

## Todo

- [ ] Task 1: Safe workspace tree and staged diffs
- [ ] Task 2: Transactional apply, lower-layer restoration and snapshots
- [ ] Task 3: Resumable uploads and complete HTTP contract
- [ ] Task 4: Independent private-files Tab
- [ ] Task 5: Console follow-latest state machine
- [ ] Task 6: Browser acceptance
- [ ] Task 7: Full regression, documentation and retirement audit

## Active slice

Task 1. Implement workspace directory/file operations and diffing with TDD, then pass spec and quality reviews.

## Completed

- Design approved and committed at `043cb5b`.
- Plan approved and committed at `05656e6`.
- Isolated branch `feat/private-file-manager-console-follow` created under `.worktrees/private-file-manager-console-follow`.
- Baseline `go test ./...` passed.
- Baseline `cd web && npm test -- --run` passed: 2 files, 24 tests.

## Evidence refs

- `docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md`
- `docs/aegis/plans/2026-07-15-private-file-manager-console-follow.md`
- Baseline command outputs in the controlling session.

## Blocked on

Nothing.

## ResumeStateHint

Read this checkpoint, the intent, baseline read set, approved spec and plan. Confirm the worktree branch and diff agree with this checkpoint. Resume Task 1 only; do not start later tasks until Task 1 implementation, spec review and code-quality review all pass.

## DriftCheckDraft

- Scope: aligned with approved private manager and console behavior.
- Compatibility: no product edits yet.
- Ownership: planned single private-layer owner; no fallback added.
- Retirement: old blind apply and immediate-apply UI remain scheduled for Tasks 2 and 4.
- Decision: continue.
