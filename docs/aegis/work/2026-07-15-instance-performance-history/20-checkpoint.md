# Todo checkpoint draft

- Updated: 2026-07-15
- Worktree: `.worktrees/instance-performance-history`
- Branch: `feature/instance-performance-history`
- Current todo: Task 4 of 7, build the five-second observation sampler and ring buffer.
- Active slice: centralize current metrics, rate deltas, run boundaries, traffic session registration and one-hour in-memory history.
- Completed todos: Tasks 1-3 implemented with TDD and both review stages approved.
- Evidence refs: Task 3 commits `7d0aa62`, `790920c`, `0e63500`, `2dd2cba`; full Go/vet/Compose validation passed; Linux CGO-disabled Panel/proxy builds passed; spec and quality reviews approved.
- Blocked on: nothing.
- Explicit non-edits: no public HTTP DTO/route or React changes during Task 4.
- Next step: dispatch Task 4 implementer, then run spec and quality reviews.

## ResumeStateHint

Read `00-intent.md`, `10-baseline-readset.md`, `20-checkpoint.md`, `30-plan.md` and current git status. Continue only in the named worktree and branch. Do not start a later task until Task 4 has passed both review stages.

## DriftCheckDraft

- Scope: aligned with the approved performance-history feature.
- Compatibility: Panel still does not mount the raw Docker socket; proxy TCP deployment path is retired.
- Retirement: request-time overview fan-out remains until Task 5 switches handlers to the sampler; do not create a second persistent metric owner.
- Decision: continue.
