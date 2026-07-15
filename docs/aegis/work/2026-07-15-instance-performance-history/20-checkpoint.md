# Todo checkpoint draft

- Updated: 2026-07-15
- Worktree: `.worktrees/instance-performance-history`
- Branch: `feature/instance-performance-history`
- Current todo: completion candidate after all 7 tasks and final whole-feature review.
- Active slice: branch finishing and user integration choice.
- Completed todos: Tasks 1-7 implemented with TDD; every task passed spec and quality review; final whole-feature review approved after cross-layer repairs.
- Evidence refs: controller fresh full Go tests, vet, 50 frontend tests, production build, Compose render, Linux amd64 CGO-disabled Panel/proxy builds and Playwright desktop/mobile 2/2 all passed.
- Blocked on: no local implementation blocker. Real Linux AF_PACKET/Unix ownership smoke and race instrumentation remain external acceptance items.
- Explicit non-edits: no persistent metrics database, alerts, undeclared-port inspection, packet content, firewall changes or cross-host monitoring were added.
- Next step: use branch-finishing workflow; do not delete the worktree or branch without user choice.

## ResumeStateHint

Read `00-intent.md`, `10-baseline-readset.md`, `20-checkpoint.md`, `30-plan.md`, `50-evidence.md` and current git status. Continue only in the named worktree and branch. Local verification is complete; Linux operational acceptance remains explicitly unexecuted.

## DriftCheckDraft

- Scope: aligned with the approved performance-history feature.
- Compatibility: existing card actions/status/map/player/package flows remain; overview map omission semantics were restored; direct `/resources` and `/players` routes remain.
- Retirement: old three-cell resource presentation, overview request-time joins and proxy TCP deployment path are retired with no fallback.
- Decision: needs user branch-integration choice; implementation evidence is locally verified.
