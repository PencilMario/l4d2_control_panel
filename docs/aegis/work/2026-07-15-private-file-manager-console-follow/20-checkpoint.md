# TodoCheckpointDraft

Updated: 2026-07-15 15:01 +08:00

## Todo

- [x] Task 1: Safe workspace tree and staged diffs
- [ ] Task 2: Transactional apply, lower-layer restoration and snapshots
- [ ] Task 3: Resumable uploads and complete HTTP contract
- [ ] Task 4: Independent private-files Tab
- [ ] Task 5: Console follow-latest state machine
- [ ] Task 6: Browser acceptance
- [ ] Task 7: Full regression, documentation and retirement audit

## Active slice

Task 2. Implement journaled private application, lower-layer restoration/rebase, snapshot retention and update-pipeline integration with TDD.

## Completed

- Design approved and committed at `043cb5b`.
- Plan approved and committed at `05656e6`.
- Isolated branch `feat/private-file-manager-console-follow` created under `.worktrees/private-file-manager-console-follow`.
- Baseline `go test ./...` passed.
- Baseline `cd web && npm test -- --run` passed: 2 files, 24 tests.
- Task 1 implementation commits `b843a4b`, `0b5c3b8`, `bba64b9`, `2704948`, `7558325`, `bef943c`.
- Task 1 `go test ./internal/content -count=1` passed.
- Task 1 spec compliance review passed after atomic replacement and symlink-test corrections.
- Task 1 code quality review approved after shared cross-manager locking and same-directory staging corrections.

## Evidence refs

- `docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md`
- `docs/aegis/plans/2026-07-15-private-file-manager-console-follow.md`
- Baseline command outputs in the controlling session.

## Blocked on

Nothing.

## ResumeStateHint

Read this checkpoint, the intent, baseline read set, approved spec and plan. Confirm the worktree branch and diff agree with this checkpoint. Resume Task 2 only; do not start Task 3 until Task 2 implementation, spec review and code-quality review all pass.

## DriftCheckDraft

- Scope: aligned with approved private manager and console behavior.
- Compatibility: existing private APIs and blind Apply remain callable; Task 1 tests pass.
- Ownership: shared in-process workspace locking is canonical across manager instances; no new deployment owner added.
- Retirement: old blind apply and immediate-apply UI remain scheduled for Tasks 2 and 4.
- Evidence gap: Windows race detector unavailable because CGO is disabled; deterministic barrier tests cover the relevant intermediate windows.
- Residual Minor risks: lexical lock aliases, lock-map retention, and history snapshot on failed final Save rename.
- Decision: continue to Task 2.
