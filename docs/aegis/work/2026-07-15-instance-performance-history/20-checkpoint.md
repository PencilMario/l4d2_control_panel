# Todo checkpoint draft

- Updated: 2026-07-15
- Worktree: `.worktrees/instance-performance-history`
- Branch: `feature/instance-performance-history`
- Current todo: Task 6 of 7, render detailed metrics and Recharts history.
- Active slice: add typed performance UI, unit formatting, four chart modes and responsive instance-card layout without duplicate full-history polling.
- Completed todos: Tasks 1-5 implemented with TDD and both review stages approved.
- Evidence refs: Task 5 commits `7f6737a`, `ea95182`; focused/full Go, tagged fixture and vet passed; overview fan-out retirement and history contract approved.
- Blocked on: nothing.
- Explicit non-edits: no backend contract, proxy, sampler or lifecycle changes during Task 6.
- Next step: dispatch Task 6 implementer, then run spec and quality reviews.

## ResumeStateHint

Read `00-intent.md`, `10-baseline-readset.md`, `20-checkpoint.md`, `30-plan.md` and current git status. Continue only in the named worktree and branch. Do not start a later task until Task 6 has passed both review stages.

## DriftCheckDraft

- Scope: aligned with the approved performance-history feature.
- Compatibility: overview/history contracts are additive and sampler-backed; direct resource/player routes remain.
- Retirement: the old three-cell CPU/memory/player presentation is replaced by the focused performance panel, but status/map/actions remain.
- Decision: continue.
