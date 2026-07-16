# Todo Checkpoint Draft

## Current todo

Commit the verified repair and enter branch completion handling.

## Active slice

- Stage and review the final repair diff, then create one Conventional Commit.
- Do not add product behavior, retries, fallback paths, or unrelated refactors.
- After commit, use the branch finishing workflow without assuming merge authority.

## Completed todos

- Read project terminology, Docker client, lifecycle, supervisor, proxy policy, compose configuration, and recent Unix Socket migration diff.
- Established the two root-cause hypotheses and their canonical owner.
- Created an isolated worktree and passed baseline tests after one bounded Windows file-lock rerun.
- Wrote the task intent, baseline read set, implementation plan, atomic task list, and baseline evidence.
- RED proved Unix attach attempted `dial tcp: lookup docker: no such host`.
- GREEN moved hijack transport selection into `NewEngine`; Unix and TCP attach tests pass.
- RED proved both `Engine.Stop` and lifecycle Restart failed on `304 Not Modified` before Start.
- GREEN preserves Docker status in `apiError`, accepts 304 only in Stop, propagates 500, and completes Restart to running.
- Related Docker/lifecycle/HTTP API tests, full Go, frontend 96/96, production build, desktop E2E, and mobile E2E passed.
- Two desktop E2E attempts exposed a pre-existing strict-locator race in ZIP import status; the imported tree/diff were correct, and the assertion now selects the exact success text. Desktop and mobile then passed.
- Repaired-path tests passed 10 consecutive repetitions; final diff, process, port, generated-output, and checklist checks passed.

## Evidence refs

- `10-baseline-readset.md`
- `30-plan.md`
- `50-evidence.md`
- Commit `7d0aa62` Unix Socket migration diff.
- `go test ./internal/docker -run TestUnixEngineAttachSupervisorUsesSocketTransport -count=1 -v` failed at the expected TCP lookup before the repair.
- `go test ./internal/docker -run "Test(UnixEngineAttachSupervisorUsesSocketTransport|AttachSupervisorHijacksFixedExecStream)$" -count=1 -v` passed after the repair.
- `go test ./internal/docker ./internal/lifecycle -run "Test(StopTreatsAlreadyStoppedContainerAsSuccess|StopPropagatesDockerFailure|RestartContinuesWhenSupervisorAlreadyStoppedContainer)$" -count=1 -v` failed both 304 expectations before the repair while the 500 assertion passed.
- The expanded GREEN target passed the two Stop tests, existing supervisor-first Stop test, and lifecycle Restart test.
- `go test ./internal/docker ./internal/lifecycle ./internal/httpapi -count=1` passed.
- `go test ./... -count=1` passed every package on the clean rerun.
- `npm test -- --run` passed 6 files and 96 tests; `npm run build` passed.
- Desktop and mobile Playwright projects each passed 1/1 after the exact-text locator repair.
- The 10-repeat target run passed Unix/TCP attach, 304/502 Stop, and lifecycle Restart; `git diff --check` passed and port 18082 had no owner.

## Blocked-on

- No implementation blocker.
- Direct inspection of the user's deployed containers remains unavailable because the local Docker daemon is not running.

## Next step

Stage and inspect the complete diff, commit it, then present branch integration options.

# Drift Check Draft

- Original intent: unchanged.
- Compatibility boundary: unchanged; TCP attach remains protected by its existing test.
- New owner/fallback/adapter: internal `apiError` preserves existing response ownership and error text; no fallback added.
- Retirement track: attach-local transport selection and accidental 304 failure interpretation are retired.
- Evidence state: direct RED/GREEN, related/full regression, build, browser journey, repeated-target, and cleanup evidence are present.
- Decision: `continue`.
