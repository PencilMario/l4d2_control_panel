# Todo checkpoint draft

- Updated: 2026-07-15
- Worktree: `.worktrees/instance-performance-history`
- Branch: `feature/instance-performance-history`
- Current todo: Task 5 of 7, expose additive overview and history contracts.
- Active slice: switch overview reads to sampler snapshots, add authenticated bounded history and retire request-time Docker/A2S fan-out.
- Completed todos: Tasks 1-4 implemented with TDD and both review stages approved.
- Evidence refs: Task 4 commits `f4629f4`, `36bf32b`, `e85b354`, `b9f25b6`, `43fa8ed`; focused transition/concurrency tests passed up to 100 repetitions; full Go/vet passed; spec and quality reviews approved.
- Blocked on: nothing.
- Explicit non-edits: no React UI changes during Task 5.
- Next step: dispatch Task 5 implementer, then run spec and quality reviews.

## ResumeStateHint

Read `00-intent.md`, `10-baseline-readset.md`, `20-checkpoint.md`, `30-plan.md` and current git status. Continue only in the named worktree and branch. Do not start a later task until Task 5 has passed both review stages.

## DriftCheckDraft

- Scope: aligned with the approved performance-history feature.
- Compatibility: sampler is the sole current/history metrics owner; lifecycle and existing resource/player endpoints remain unchanged.
- Retirement: Task 5 must remove overview request-time Docker/A2S fan-out while retaining `/resources` and `/players` for direct consumers.
- Decision: continue.
