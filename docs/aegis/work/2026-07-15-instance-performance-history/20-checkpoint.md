# Todo checkpoint draft

- Updated: 2026-07-15
- Worktree: `.worktrees/instance-performance-history`
- Branch: `feature/instance-performance-history`
- Current todo: Task 7 of 7, verify the main journey and deployment boundary.
- Active slice: extend Playwright desktop/mobile coverage, run full regression/build/Compose checks and record bounded Linux-runtime residuals.
- Completed todos: Tasks 1-6 implemented with TDD and both review stages approved.
- Evidence refs: Task 6 commits `1d168d2`, `301b4ba`, `c77a98e`, `7f02109`, `51e32ab`; frontend full suite passes 46 tests, build passes; spec and quality approved.
- Blocked on: nothing.
- Explicit non-edits: no new product scope; Task 7 may only repair defects exposed by journey verification.
- Next step: dispatch Task 7 verifier/implementer, then run final whole-feature review.

## ResumeStateHint

Read `00-intent.md`, `10-baseline-readset.md`, `20-checkpoint.md`, `30-plan.md` and current git status. Continue only in the named worktree and branch. Task 7 is the final implementation slice; do not claim completion before final review and verification gates.

## DriftCheckDraft

- Scope: aligned with the approved performance-history feature.
- Compatibility: existing card actions/status/map/player/package flows remain; history bootstrap is independent of the five-second live poll.
- Retirement: old three-cell resource presentation and overview request-time joins are retired; no fallback should reappear during E2E fixes.
- Decision: continue.
