# Todo checkpoint draft

- Updated: 2026-07-15
- Worktree: `.worktrees/instance-performance-history`
- Branch: `feature/instance-performance-history`
- Current todo: Task 1 of 7, expand canonical Docker runtime metrics.
- Active slice: add tested Docker runtime start time, memory limit, block I/O and PID metrics.
- Completed todos: approved design, implementation plan, isolated worktree setup and clean baseline verification.
- Evidence refs: `go test -count=1 ./...` passed; `npm test -- --run` passed 2 files and 27 tests in the isolated worktree.
- Blocked on: nothing.
- Explicit non-edits: no proxy transport, packet capture, sampler, HTTP or React changes during Task 1.
- Next step: dispatch Task 1 implementer, then run spec and quality reviews.

## ResumeStateHint

Read `00-intent.md`, `10-baseline-readset.md`, `20-checkpoint.md`, `30-plan.md` and current git status. Continue only in the named worktree and branch. Do not start a later task until Task 1 has passed both review stages.

## DriftCheckDraft

- Scope: aligned with the approved performance-history feature.
- Compatibility: baseline is clean; no implementation changes exist yet.
- Retirement: Boolean-only Docker runtime probe retires only after all callers move to the richer runtime state.
- Decision: continue.
