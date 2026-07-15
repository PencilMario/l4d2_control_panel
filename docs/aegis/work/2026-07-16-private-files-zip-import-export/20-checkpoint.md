# Todo Checkpoint Draft

Updated: 2026-07-16 03:27 +08:00

## Current Todo

- Active: completion review and branch integration choice.
- Pending: branch completion workflow only.
- Completed: approved design and plan, isolated baseline, Tasks 1-5 implementation, desktop/mobile acceptance, full verification and scope audit.

## Evidence Refs

- Design commit: `e220bad`.
- Plan commit: `1b90e14`.
- Baseline: targeted HTTP test passed 5 consecutive runs; serial full Go suite passed with dedicated `GOTMPDIR`; Vitest passed 5 files and 83 tests.
- Task 1: `go test ./internal/content -run 'TestPrivateSnapshot|TestPrivateRestore' -count=1` and `go test ./internal/content -count=1` passed.
- Task 2 RED: `go test ./internal/content -run 'TestPrivateZIP' -count=1` failed because the archive API did not exist.
- Task 2 GREEN: `go test ./internal/content -run 'TestPrivateZIP|TestPrivateSnapshot|TestPrivateRestore' -count=1` and `go vet ./internal/content` passed.
- Task 2 broad run: the full content package intermittently failed the pre-existing Windows manifest atomic-reader test and one temporary-directory cleanup; the focused archive/restoration matrix stayed green.
- Task 3 RED: archive route tests returned `405 Method Not Allowed` before route registration.
- Task 3 GREEN: `go test ./internal/httpapi -run 'TestPrivateFileAPIContract|TestPrivateArchiveAPI' -count=1` and `go vet ./internal/content ./internal/httpapi` passed.
- Task 3 broad run: the combined content/API command hit Windows-only `testing.TempDir` cleanup locks in unrelated tests; all behavioral assertions in the focused API matrix passed.
- Task 4 RED: `npm test -- --run src/app/PrivateFilesPage.test.tsx` failed 4 new tests because the ZIP import/export controls did not exist.
- Task 4 GREEN: `npm test -- --run src/app/PrivateFilesPage.test.tsx` passed 23 tests; `npm run build` completed without TypeScript errors. Vite retained the repository's existing bundle-size advisory.
- Recovery regression RED: the first full Go run exposed `TestRecoverRollsBackUncommittedDeployment` waiting on the same instance lease for 10 minutes; the focused clean-instance recovery test then failed in 0.12 seconds.
- Recovery regression GREEN: `TestPrivateZIPRecoverySkipsCleanInstanceLease` and `TestRecoverRollsBackUncommittedDeployment` passed; the combined content/update recovery matrix and `go vet ./internal/content ./internal/updates` passed.
- Final backend: `go test ./internal/content ./internal/httpapi -count=1` passed; `go test ./... -count=1` passed every package in 35.1 seconds.
- Final frontend: `npm test -- --run` passed 5 files and 87 tests; `npm run build` passed.
- Final browser journey: desktop passed 1/1 in 24.2 seconds and mobile passed 1/1 in 25.6 seconds, run sequentially on fixture port `127.0.0.1:18082`.
- Ownership/scope audit and `git diff --check` passed. Full detail is in `50-evidence.md`.

## Blockers

None. Windows temporary-directory cleanup failed in two earlier attempts, but the final exact focused and full Go commands both passed without retries.

## Drift Check Draft

- Scope remains private workspace ZIP import/export only.
- Compatibility boundary remains staged import, manual apply, unchanged snapshots/applied manifest and exact archive root paths.
- HTTP remains a contract adapter: it spools export before committing headers and delegates import semantics to `PrivateManager`; mutation auditing remains middleware-owned.
- The React controls call only the archive routes, keep in-flight work owned by the captured instance and leave `/private/apply` exclusively on the explicit apply button.
- Recovery now discovers private recovery work before taking an instance lease, preserving the prior update-recovery lock order while still cleaning pre-journal ZIP work.
- Decision: `continue`.

## Next Step

Run completion review, commit acceptance/evidence records, then present the branch integration options.
