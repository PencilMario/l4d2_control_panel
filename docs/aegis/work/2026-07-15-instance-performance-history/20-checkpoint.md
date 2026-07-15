# Todo checkpoint draft

- Updated: 2026-07-15
- Worktree: `.worktrees/instance-performance-history`
- Branch: `feature/instance-performance-history`
- Current todo: Task 2 of 7, add declared-port traffic accounting to the restricted proxy.
- Active slice: implement payload-free port attribution, session counters, internal Unix-Socket HTTP client and Linux/non-Linux capture boundaries.
- Completed todos: approved design, implementation plan, isolated worktree setup, clean baseline verification and Task 1 Docker runtime metrics with both reviews approved.
- Evidence refs: Task 1 commit `51dd19c`; focused Docker RED/GREEN; `go test -count=1 ./internal/docker`; `go test -count=1 -tags e2e ./cmd/e2e-fixture`; spec and quality reviews approved.
- Blocked on: nothing.
- Explicit non-edits: no Compose transport migration, Panel sampler, public HTTP or React changes during Task 2.
- Next step: dispatch Task 2 implementer, then run spec and quality reviews.

## ResumeStateHint

Read `00-intent.md`, `10-baseline-readset.md`, `20-checkpoint.md`, `30-plan.md` and current git status. Continue only in the named worktree and branch. Do not start a later task until Task 2 has passed both review stages.

## DriftCheckDraft

- Scope: aligned with the approved performance-history feature.
- Compatibility: Task 1 is additive; `Engine.Running` still delegates to `Runtime` for existing callers.
- Retirement: Docker `networks` is not introduced as a fallback; Task 2 establishes the declared-port owner only.
- Decision: continue.
