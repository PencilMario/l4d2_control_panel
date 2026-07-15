# Todo checkpoint draft

- Updated: 2026-07-15
- Worktree: `.worktrees/instance-performance-history`
- Branch: `feature/instance-performance-history`
- Current todo: Task 3 of 7, move proxy transport to a shared Unix Socket.
- Active slice: remove proxy TCP exposure, add Unix transport support to the Docker client and preserve the Panel-without-raw-socket boundary.
- Completed todos: Tasks 1-2 implemented with TDD and both review stages approved.
- Evidence refs: Task 2 commits `c9b7ad6`, `c2a2d37`, `0933c22`, `8aaf462`; focused/full Go tests and vet passed; Linux amd64 CGO-disabled binaries compiled; parser/retry corrective tests passed 100 repetitions.
- Blocked on: nothing.
- Explicit non-edits: no sampler, public HTTP, React or lifecycle behavior changes during Task 3.
- Next step: dispatch Task 3 implementer, then run spec and quality reviews.

## ResumeStateHint

Read `00-intent.md`, `10-baseline-readset.md`, `20-checkpoint.md`, `30-plan.md` and current git status. Continue only in the named worktree and branch. Do not start a later task until Task 3 has passed both review stages.

## DriftCheckDraft

- Scope: aligned with the approved performance-history feature.
- Compatibility: Docker endpoint whitelist is unchanged; packet capture failure leaves Docker proxy available.
- Retirement: `LISTEN_ADDR` and `tcp://socket-proxy:23750` are the active retirement targets for Task 3; no TCP fallback should remain.
- Decision: continue.
