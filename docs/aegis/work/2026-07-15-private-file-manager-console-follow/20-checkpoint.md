# TodoCheckpointDraft

Updated: 2026-07-15 16:06 +08:00

## Todo

- [x] Task 1: Safe workspace tree and staged diffs
- [x] Task 2: Transactional apply, lower-layer restoration and snapshots
- [ ] Task 3: Resumable uploads and complete HTTP contract
- [ ] Task 4: Independent private-files Tab
- [ ] Task 5: Console follow-latest state machine
- [ ] Task 6: Browser acceptance
- [ ] Task 7: Full regression, documentation and retirement audit

## Active slice

Task 3. Implement resumable private uploads and the complete authenticated HTTP file-manager contract with TDD.

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
- Task 2 implementation commits `605d518`, `b0c6491`, `40fa4ed`, `075d98e`, `8f97884`, `545cf50`, `b8e441e`.
- Task 2 `go test ./internal/content ./internal/updates -count=1` passed; focused recovery/restore/pipeline tests passed repeated runs.
- Task 2 spec review passed after union rebase, mandatory snapshots, validated crash recovery, exact outer rollback, transaction leasing, deferred prune and trusted-root journal validation.
- Task 2 code quality review approved after restore journaling, exact snapshot validation, non-overlapping backups and pre-journal cleanup ownership.

## Evidence refs

- `docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md`
- `docs/aegis/plans/2026-07-15-private-file-manager-console-follow.md`
- Baseline command outputs in the controlling session.

## Blocked on

Nothing.

## ResumeStateHint

Read this checkpoint, the intent, baseline read set, approved spec and plan. Confirm the worktree branch and diff agree with this checkpoint. Resume Task 3 only; do not start Task 4 until Task 3 implementation, spec review and code-quality review all pass.

## DriftCheckDraft

- Scope: aligned with approved private manager and console behavior.
- Compatibility: existing private API remains callable; Apply now delegates transactionally; package rollback and overlay order remain covered.
- Ownership: `PrivateManager` is the sole private apply/rebase/snapshot/recovery owner; Pipeline holds its content-owned lease.
- Retirement: blind Apply is retired; immediate-apply UI remains scheduled for Task 4.
- Evidence gap: Windows race detector unavailable because CGO is disabled; deterministic barrier tests cover the relevant intermediate windows.
- Residual Minor risks: lexical lock aliases, lock-map retention, and history snapshot on failed final Save rename.
- Task 2 residual: Windows normal symlinks are covered; junction/reparse subtype reporting remains dependent on Go runtime behavior.
- Intermittent Windows TempDir cleanup/file-lock noise occurred across unrelated tests; required fresh and repeated focused suites passed.
- Decision: continue to Task 3.
