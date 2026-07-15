# EvidenceBundleDraft

Updated: 2026-07-15 20:33 +08:00

This bundle records verified Task 7 evidence. It is not the final whole-branch review or an authoritative completion signal.

## Commit scope

- Approved design and plan: `043cb5b`, `05656e6`.
- Workspace and transactional apply: `b843a4b` through `bef943c`; `605d518` through `b8e441e`.
- Upload/API contract: `37bf165` through `b86e5aa`.
- Independent Tab and console follow: `7c3bf72`, `b558a08`, `614b55e`, `d46dd9a`, `2724573`.
- Browser acceptance: `828782a`, `da6a3e2`, with evidence checkpoint `7697c2f`.
- Task 7 documentation: `2384176`, with wording correction `6be2080`.
- Final-review upload identity fix: `4a79f45`.
- Integration owner/baseline fix: `82cdfba`.
- Integration leaf-backup fix: `dad4357`.

## Verification matrix

Run from `E:\GithubKu\l4d2_control_panel\.worktrees\private-file-manager-console-follow` unless noted.

| Command | Result |
| --- | --- |
| `go test ./internal/content ./internal/updates ./internal/httpapi ./cmd/panel -count=1` | PASS on the third exact run: all four packages passed. The first run failed only during `testing.TempDir` cleanup (`shared-vpk: The directory is not empty`); the second failed at a different `internal/content` temporary file with Windows `used by another process`. The other three packages passed on all three runs. |
| `$tmp = Join-Path (Get-Location) '.tmp-go-test'; $null = New-Item -ItemType Directory -Force -Path $tmp; $env:GOTMPDIR = $tmp; go test -p 1 ./internal/content ./internal/updates ./internal/httpapi ./cmd/panel -count=1` | PASS: all four packages. This is a directly executable PowerShell diagnostic retry with a dedicated temp root and serial package scheduling, not a replacement for the two exact-command failure records. |
| `go test ./... -count=1` | PASS on the first run: all packages passed; packages without tests were reported normally. This run also passed `internal/content`, bounding the preceding failures to intermittent Windows temp/file-lock behavior. |
| `cd web; npm test -- --run` | PASS: 4 files, 52 tests. |
| `cd web; npm run build` | PASS: TypeScript and Vite production build; 1,781 modules transformed. |
| `cd web; npm run e2e -- --project=desktop` | PASS: Playwright `desktop`, 1/1 test. |
| `cd web; npm run e2e -- --project=mobile` | PASS: Playwright `mobile`, 1/1 test. |
| `go test -tags=e2e ./cmd/e2e-fixture` | PASS (Go reported cached). |

After final review produced `4a79f45`, fresh verification from `web` recorded:

| Command | Result |
| --- | --- |
| `npm test -- --run` | PASS: 4 files, 54 tests, including exact instance/target-path/file-fingerprint resume identity and legacy fingerprint cleanup. |
| `npm run build` | PASS: TypeScript and Vite production build; 1,781 modules transformed. |

## Integration fixes and second final review

`82cdfba` retired the runtime supervisor's staged-private copy owner, allowed only controlled shared-VPK links through private apply/recovery, and established a conservative manifest baseline for legacy instances. The legacy-baseline and controlled-link regressions were verified RED against the pre-fix branch and GREEN after the fix, including `TestPrivateControlledSharedVPKCanBeOverriddenAndRestored`, `TestPrivateRejectsArbitraryGameSymlink`, `TestPrivateMissingManifestMigratesLegacyBaseline`, `TestPrivateNewSaveStillReportsAddedAfterEmptyBaseline`, `TestPrivateCorruptManifestIsNotReinitialized`, `TestPrivateLegacyBaselineCapturesControlledSharedVPKLink`, `TestPrivateConcurrentFirstContactCreatesOneValidBaseline`, `TestPipelineFailureRollbackRestoresControlledSharedVPKLink` and `TestSupervisorDoesNotDeployStagedPrivateContent`.

`dad4357` split private directory leaf backups so package updates preserve controlled sibling links without accepting arbitrary links. Its regression coverage includes `TestPipelineJournalAvoidsUnrelatedNestedDirectoryBackup`, `TestPipelineUpdateWithPrivateDirectoryAndControlledSiblingLink`, `TestPipelineRollbackWithPrivateDirectoryRestoresControlledSiblingLink` and `TestPipelinePrivateDirectoryStillRejectsArbitrarySiblingLink`.

The second whole-branch review covered `05656e6..dad4357` and reported no Critical or Important findings. Fresh verification after both fixes recorded:

| Command | Result |
| --- | --- |
| `go test ./... -count=1` | PASS in the second reviewer's fresh matrix. During this evidence update, two additional exact reruns hit distinct known Windows locks: `TestPrivateWorkspaceCRUDAndDiff`'s `.tmp` file, then `health.test.exe`. Other packages passed, and `internal/content` passed on the second local rerun. These additional host flakes are recorded rather than hidden. |
| `$tmp = Join-Path (Get-Location) '.tmp-go-test'; $null = New-Item -ItemType Directory -Force -Path $tmp; $env:GOTMPDIR = $tmp; go test -p 1 ./... -count=1` | PASS: all Go packages with a dedicated temp root and serial package scheduling. |
| `go vet ./...` | PASS with no output. |
| `cd web; npm test -- --run` | PASS: 4 files, 54 tests. |
| `cd web; npm run build` | PASS: TypeScript and Vite production build; 1,781 modules transformed. |
| `cd web; npm run e2e -- --project=desktop` | PASS: Playwright `desktop`, 1/1 test. |
| `cd web; npm run e2e -- --project=mobile` | PASS: Playwright `mobile`, 1/1 test. |

An initial reviewer attempt to run desktop and mobile Playwright in parallel collided on the shared fixture port `127.0.0.1:18082`. The projects were then run sequentially as shown above and both passed; no product change was made for the test-runner collision.

## Retirement and ownership audit

- `rg -n "保存并立即应用|private\.Apply\(" web/src internal` returned no matches: the old immediate-apply UI and blind `private.Apply` call are retired from product source.
- `rg -n "RebaseAndApply|ApplyChanges" internal` shows the public private-manager methods in `internal/content/private_state.go`; production manual apply enters through `internal/httpapi/server.go` and `ApplyChangesWithProgress`, while lower-layer deployment enters through `internal/updates/pipeline.go` and the leased `PrivateTransaction.RebaseAndApply` path. Remaining direct calls are compatibility delegation or tests.
- `rg -n "game/left4dead2|os\.Symlink|exec\.Command" web/src/app/PrivateFilesPage.tsx internal/content --glob "private*.go"` found only adversarial `os.Symlink` test fixtures. The UI and private upload implementation expose no game path, create no symlink and execute no command.
- `git branch --show-current` returned `feat/private-file-manager-console-follow`.
- `git rev-parse HEAD` returned `dad435741dbefe75b801b4165ef5daa18206323e` before this evidence update.
- `git status --short --branch` returned only `## feat/private-file-manager-console-follow`, proving the worktree was clean at `dad4357` before these documentation edits.

## User journey covered

Desktop and mobile Playwright exercised the real HTTP fixture journey, including the independent Private Files Tab, staged changes and apply Job, upload interruption/resume, delete/lower-layer restoration, snapshot restore, refresh/reconnect behavior and console follow pause/resume. The fixture replaces Docker, SRCDS, A2S, Steam and GitHub boundaries.

## Residual risk and unverified scope

- Windows `testing.TempDir` cleanup and transient file-lock noise occurred twice under the exact targeted command. A dedicated temp root plus serial scheduling passed, and the full uncached Go suite passed on its first run. Product code was not weakened for this host behavior.
- `go env` reports `GOOS=windows`, `GOARCH=amd64`, `CGO_ENABLED=0`; the race detector was not available or run in this environment. Deterministic concurrency tests remain the evidence for intermediate windows.
- Windows normal symlinks are covered. Junction/reparse-point subtype reporting remains dependent on Go runtime behavior and was not separately exercised.
- The per-upload session lock map retains UUID entries; this remains a Minor bounded memory-retention risk.
- No real Docker daemon, SteamCMD install/update, live SRCDS process or real existing-data migration was exercised in Task 7. Desktop/mobile acceptance used the tagged fixture.
- No real Linux filesystem, ENOSPC injection or daemon-restart acceptance was run.

## Controller completion-candidate verification

- `go test -p 1 ./... -count=1` passed all Go packages in a fresh sequential run.
- `go test ./internal/updates -run TestPipelineRollbackDoesNotPrunePrivateSnapshots -count=10` passed all 10 repetitions after an earlier Windows TempDir cleanup interruption.
- `go vet ./...` passed with no output.
- `go test -tags=e2e ./cmd/e2e-fixture -count=1` passed.
- `cd web; npm test -- --run` passed 4 files and 54 tests.
- `cd web; npm run build` passed; Vite transformed 1,781 modules.
- Desktop Playwright passed 1/1 in 20.4 seconds; mobile passed 1/1 in 20.8 seconds, run sequentially.
- `git diff --check` passed and `git status --short` was empty before this evidence update.

## DriftCheckDraft

- Scope: Task 7 changed README and Aegis work records; final reviews added `4a79f45`, `82cdfba` and `dad4357` product corrections.
- Compatibility: the private upload API is unchanged; client resume matching is tighter, legacy upload fingerprints are discarded, controlled shared links remain supported, and legacy instances receive a conservative baseline.
- Retirement: old immediate apply, blind apply callers and the runtime supervisor copy owner are retired; `PrivateManager` remains the transactional apply/rebase/recovery owner.
- Decision: the second whole-branch review found no Critical or Important issues and controller-owned completion-candidate verification passed.

## Confidence and authority

Confidence: B. Core automated regression and desktop/mobile user journeys have direct fresh evidence, with bounded Windows-host and real-runtime gaps above.

Authority: verified implementation evidence and advisory merge readiness. Branch integration remains a user choice.
