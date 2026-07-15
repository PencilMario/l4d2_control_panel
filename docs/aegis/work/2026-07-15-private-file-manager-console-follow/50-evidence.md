# EvidenceBundleDraft

Updated: 2026-07-15 19:25 +08:00

This bundle records verified Task 7 evidence. It is not the final whole-branch review or an authoritative completion signal.

## Commit scope

- Approved design and plan: `043cb5b`, `05656e6`.
- Workspace and transactional apply: `b843a4b` through `bef943c`; `605d518` through `b8e441e`.
- Upload/API contract: `37bf165` through `b86e5aa`.
- Independent Tab and console follow: `7c3bf72`, `b558a08`, `614b55e`, `d46dd9a`, `2724573`.
- Browser acceptance: `828782a`, `da6a3e2`, with evidence checkpoint `7697c2f`.
- Task 7 documentation is committed separately as `docs: 记录私有文件管理验证`; its SHA is the commit containing this file.

## Verification matrix

Run from `E:\GithubKu\l4d2_control_panel\.worktrees\private-file-manager-console-follow` unless noted.

| Command | Result |
| --- | --- |
| `go test ./internal/content ./internal/updates ./internal/httpapi ./cmd/panel -count=1` | PASS on the third exact run: all four packages passed. The first run failed only during `testing.TempDir` cleanup (`shared-vpk: The directory is not empty`); the second failed at a different `internal/content` temporary file with Windows `used by another process`. The other three packages passed on all three runs. |
| `$env:GOTMPDIR=<worktree>/.tmp-go-test; go test -p 1 ./internal/content ./internal/updates ./internal/httpapi ./cmd/panel -count=1` | PASS: all four packages. This is a diagnostic retry with a dedicated temp root and serial package scheduling, not a replacement for the two exact-command failure records. |
| `go test ./... -count=1` | PASS on the first run: all packages passed; packages without tests were reported normally. This run also passed `internal/content`, bounding the preceding failures to intermittent Windows temp/file-lock behavior. |
| `cd web; npm test -- --run` | PASS: 4 files, 52 tests. |
| `cd web; npm run build` | PASS: TypeScript and Vite production build; 1,781 modules transformed. |
| `cd web; npm run e2e -- --project=desktop` | PASS: Playwright `desktop`, 1/1 test. |
| `cd web; npm run e2e -- --project=mobile` | PASS: Playwright `mobile`, 1/1 test. |
| `go test -tags=e2e ./cmd/e2e-fixture` | PASS (Go reported cached). |

## Retirement and ownership audit

- `rg -n "保存并立即应用|private\.Apply\(" web/src internal` returned no matches: the old immediate-apply UI and blind `private.Apply` call are retired from product source.
- `rg -n "RebaseAndApply|ApplyChanges" internal` shows the public private-manager methods in `internal/content/private_state.go`; production manual apply enters through `internal/httpapi/server.go` and `ApplyChangesWithProgress`, while lower-layer deployment enters through `internal/updates/pipeline.go` and the leased `PrivateTransaction.RebaseAndApply` path. Remaining direct calls are compatibility delegation or tests.
- `rg -n "game/left4dead2|os\.Symlink|exec\.Command" web/src/app/PrivateFilesPage.tsx internal/content --glob "private*.go"` found only adversarial `os.Symlink` test fixtures. The UI and private upload implementation expose no game path, create no symlink and execute no command.
- `git status --short` was clean before documentation edits. The final diff/status inspection after edits is recorded by the Task 7 commit review.

## User journey covered

Desktop and mobile Playwright exercised the real HTTP fixture journey, including the independent Private Files Tab, staged changes and apply Job, upload interruption/resume, delete/lower-layer restoration, snapshot restore, refresh/reconnect behavior and console follow pause/resume. The fixture replaces Docker, SRCDS, A2S, Steam and GitHub boundaries.

## Residual risk and unverified scope

- Windows `testing.TempDir` cleanup and transient file-lock noise occurred twice under the exact targeted command. A dedicated temp root plus serial scheduling passed, and the full uncached Go suite passed on its first run. Product code was not weakened for this host behavior.
- `go env` reports `GOOS=windows`, `GOARCH=amd64`, `CGO_ENABLED=0`; the race detector was not available or run in this environment. Deterministic concurrency tests remain the evidence for intermediate windows.
- Windows normal symlinks are covered. Junction/reparse-point subtype reporting remains dependent on Go runtime behavior and was not separately exercised.
- The per-upload session lock map retains UUID entries; this remains a Minor bounded memory-retention risk.
- No real Docker daemon, SteamCMD install/update, live SRCDS process or real existing-data migration was exercised in Task 7. Desktop/mobile acceptance used the tagged fixture.
- No real Linux filesystem, ENOSPC injection or daemon-restart acceptance was run.

## DriftCheckDraft

- Scope: Task 7 changed only README and Aegis work records.
- Compatibility: documented behavior matches the verified API/UI paths; no product behavior changed.
- Retirement: old immediate apply and blind apply caller remain absent; one private transactional owner retains manual apply and lower-layer rebase entry points.
- Decision: ready for controller-owned final whole-branch review; Task 7 spec/code-quality review and final review remain open.

## Confidence and authority

Confidence: B. Core automated regression and desktop/mobile user journeys have direct fresh evidence, with bounded Windows-host and real-runtime gaps above.

Authority: verified Task 7 evidence only. Final spec compliance, code quality and whole-branch completion are not granted here.
