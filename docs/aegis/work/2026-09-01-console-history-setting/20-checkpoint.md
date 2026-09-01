# Checkpoint

## TodoCheckpointDraft

- Completed: design, baseline read set, implementation plan, and task records committed in `24c7a2e`.
- Completed: Task 1 Store persistence and Task 2 backend dynamic Hub/API, both with RED→GREEN target evidence.
- Completed: Task 3 frontend shared setting, settings card, immediate display trimming, and TXT export compatibility.
- Completed: concurrency review fix in `f0e3bc7`; SQLite persistence and in-memory Hub updates are serialized.
- Active slice: Task 4, verification evidence and deployment readiness.
- Pending: remote deployment requires confirmation because the target Panel Compose is stopped; unrelated Windows TempDir races remain in Go full regression.
- Next step: preserve the branch, then obtain confirmation before starting/updating the remote Panel Compose.

## Evidence

- Isolated worktree: `.worktrees/console-history-setting`, branch `feature/console-history-setting`.
- Go target evidence: `go test ./internal/store -run TestConsoleHistoryLinesDefaultsPersistsAndRejectsInvalidValues -count=1` passed; `go test ./internal/httpapi -run 'TestConsoleHistoryLimit|TestConsoleSettingsReadUpdateAndRejectInvalidValues|ConsoleWebSocket' -count=1` passed.
- Concurrency evidence: the new serialization regression failed to compile before the lock existed, then passed after `f0e3bc7`; final `go test ./internal/httpapi -count=1` passed.
- Frontend dependencies installed in the isolated worktree with `npm ci` (npm reported 3 audit vulnerabilities; no dependency changes were requested).
- Frontend RED evidence: new settings tests failed because the settings control was absent; the console buffer test failed before `trimConsoleOutput` existed.
- Frontend target GREEN evidence: `npm test -- --run src/app/consoleBuffer.test.ts src/app/App.test.tsx` passed with 2 files and 77 tests.
- Full frontend evidence: `npm test -- --run` passed with 22 files and 237 tests; `npm run build:web`, `npm run build`, and `go vet ./...` passed.
- Full Go evidence: `go test ./... -p 1 -count=1` reached all packages but exited 1 on unrelated Windows TempDir cleanup races; details are in `50-evidence.md`.
- Remote read-only evidence: `sirphomesv` Compose config passed, Panel Compose has no containers, and `127.0.0.1:18081/api/health` is unavailable; no remote write was made.

## DriftCheckDraft

- Scope: unchanged; only configurable console cache behavior is in scope, plus its required concurrency repair.
- Compatibility: existing TXT export, WebSocket protocol, and 1 MiB backend byte cap remain protected.
- Retirement: static backend line cap will be replaced by a dynamic limit with 8192 as its default.
- Decision: verification evidence recorded; pause before remote mutation because starting the stopped Panel Compose may trigger instance reconciliation.

## Risk / Unknown

- Go full regression is not clean due existing Windows TempDir cleanup races; target packages, frontend regression/build, and vet are clean.
- SSH alias is available and remote target is identified, but the Panel Compose is stopped and the deployment directory is not a Git worktree; starting it is a material remote mutation requiring confirmation.
