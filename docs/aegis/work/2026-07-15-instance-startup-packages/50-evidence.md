# Evidence

## Baseline

- Worktree: `E:/GithubKu/l4d2_control_panel/.worktrees/instance-startup-packages`
- Branch: `feature/instance-startup-packages`
- Base commit: `01baa4b`
- `go test -count=1 ./...`: PASS.
- `cd web && npm test -- --run`: PASS, 17 tests.
- `cd web && npm run build`: PASS.

Implementation evidence will be appended after each red/green slice.

## Task 1: Selected Package Persistence

- RED: `go test -count=1 ./internal/store` failed to compile because `domain.Instance.SelectedPackageID` did not exist.
- GREEN: added additive `selected_package_id`, legacy backfill from `package_version`, explicit selected/applied JSON fields and CRUD scan coverage.
- `go test -count=1 ./internal/store`: PASS.
- `go test -count=1 ./internal/store ./internal/httpapi`: PASS.
- `git diff --check`: PASS.

## Task 5: Strict Instance Configuration API

- RED: HTTP tests returned `400 unknown field` for `package_id` and `extra_args`; installed name-only updates still used the unconditional rebuild path.
- GREEN: shared input validation now requires an existing package, validates extra arguments, sorts plugin ports and plans runtime/package work by diff.
- New instances store selected package with an empty applied package. Containerless edits return `200`; installed runtime/package edits return one serialized `reconfigure` Job.
- `go test -count=1 ./internal/httpapi`: PASS.
- `go test -count=10 -tags=e2e ./cmd/e2e-fixture -run TestFixtureStartupRecoversInterruptedPackageDeployment -v`: PASS after one non-reproducible Windows temporary-directory cleanup failure.
- Root-cause judgment for that transient: the failure was in existing rollback cleanup, the relevant fixture diff only updates SQLite fields, and ten immediate reproductions plus the next tagged full run passed. No retry/fallback was added. Confidence B; retain as Windows filesystem residual risk.
- `go test -count=1 ./...`: PASS.
- `go test -count=1 -tags=e2e ./cmd/e2e-fixture`: PASS.
- `go vet ./...`: PASS.
- `git diff --check`: PASS.

## Task 4: Package Update Intent

- RED: Coordinator tests failed to compile because it had no instance repository and could not own running intent or applied state.
- GREEN: Coordinator conditionally stops/starts, rolls back only the downtime it caused, and records selected/applied IDs after commit for both hot and full modes.
- `go test -count=1 ./internal/updates ./internal/httpapi ./internal/automation`: PASS.
- `go test -count=1 -tags=e2e ./cmd/e2e-fixture`: PASS.
- The original untagged fixture command was corrected in the plan because all fixture sources are intentionally build-tagged.
- `go test -count=1 ./...`: PASS.
- `go vet ./...`: PASS.
- `git diff --check`: PASS.

## Task 3: First-Start Provisioning

- RED: focused tests failed because `provisioning.Service`, Docker `InstallGame`, lifecycle `WithProvisioner` and runtime `require_game` did not exist.
- GREEN: maintenance SteamCMD owns anonymous Windows/Linux bootstrap, package Pipeline runs before game-container creation, applied state is written only after deployment, and runtime refuses missing game content.
- `go test -count=1 ./internal/provisioning ./internal/docker ./internal/lifecycle ./runtime ./cmd/panel`: PASS.
- `go test -count=1 ./...`: PASS.
- `go vet ./...`: PASS.
- `git diff --check`: PASS.

## Task 2: Canonical SRCDS Arguments

- RED: focused tests failed because `internal/srcds.Command` / `ParseExtraArgs` did not exist, `BuildContainerSpec` returned no error, and Supervisor lacked `SRCDS_EXTRA_ARGS_JSON`.
- GREEN: added shellword validation, Panel-owned option rejection, canonical command ordering, JSON token transport and raw-env compatibility fallback.
- `go test -count=1 ./internal/srcds ./internal/docker ./internal/lifecycle ./runtime`: PASS.
- `go test -count=1 ./...`: PASS.
- `go vet ./...`: PASS.
- `git diff --check`: PASS.

## Task 6: Shared Instance Configuration UI

- RED: `cd web && npm test -- --run` initially had three failures: the shared row component referenced a missing `ChevronRight` import, and content repository tests could no longer observe package refresh after package state moved to `App`.
- GREEN: create and edit now share one controlled modal covering all managed startup fields, per-instance package selection, raw extra arguments and a live canonical-order command preview. Instance cards show selected package identity and pending application state.
- Package and instance/health requests remain independent and execute in parallel; the content repository refreshes shared package state after mount and upload.
- `cd web && npm test -- --run`: PASS, 22 tests.
- `cd web && npm run build`: PASS.
- `git diff --check`: PASS.

## Task 7: Real Browser Journey

- The expanded package-first journey initially passed because the Task 5 API, Task 6 UI and existing fixture start/applied-state behavior already covered the new flow.
- Visual inspection exposed a real responsive defect: generic `.modal > div` flex styling placed the field grid and command preview side by side, squeezing mobile controls and clipping text.
- RED: strengthened Playwright geometry checks failed with the command preview above/beside the field-grid bottom and mismatched left/right edges.
- GREEN: `.instance-config-modal > .instance-config-body` now explicitly owns block layout. The preview renders below the fields at full width on desktop and mobile, with no control or viewport overflow.
- The browser flow uploads packages A and B before creation, creates and first-starts two instances with different packages, ports and extra arguments, verifies both selected/applied assignments after refresh, then edits the first to B, observes exactly one `reconfigure` Job and completes console, player, VPK, private overlay, full update, schedule and Job recovery checks.
- Fixture conclusion: no new fake branch was required. First start already copies selected to applied state, and installed package changes use the production `updates.Coordinator` owner.
- `go test -count=1 ./...`: PASS.
- `go vet ./...`: PASS.
- `cd web && npm test -- --run`: PASS, 22 tests.
- `cd web && npm run build`: PASS.
- `cd web && npm run e2e`: PASS, desktop and mobile.
- `docker compose --env-file .env.example config --quiet`: PASS.
- Screenshots: `web/test-results/control-panel-real-HTTP-ad-79bc6--and-streams-recovery-state-desktop/desktop-instance-config.png`, `desktop-journey.png`, and matching mobile outputs.

## Task 8: Review and Completion Audit

- Review scope: `01baa4b..a53475f`, covering persistence, argv transport, Docker maintenance, provisioning, package coordination, HTTP contracts, React UI and Playwright acceptance. The advisory review was performed in the primary session because this task disallowed subagents.
- Design coverage: section 3 maps to Task 1; sections 4-5 to Tasks 2 and 6; section 6 to Tasks 1, 4-7; section 7 to Task 3; section 8 to Tasks 4, 5, 7 and review repairs; section 9 to Task 5; section 10 to Tasks 1-7 plus final regressions; section 11 to the raw-env compatibility fallback, strict API input and fixed runtime owner audits.
- Placeholder/owner audit: no incomplete production branches were found. Applied-package writes remain owned by first-start provisioning and the update Coordinator for their distinct workflows; HTTP no longer writes applied state. New Panel containers use validated JSON argv, while raw `SRCDS_EXTRA_ARGS` remains only for old/manual runtime compatibility.
- Finding 1 RED: `TestStartProvisionsUninstalledInstanceWhenPackageIsAlreadyMarkedApplied` observed `create,start` instead of `prepare,create,start`. A content update could mark a package applied before game installation and make first start skip SteamCMD.
- Finding 1 GREEN: lifecycle now provisions whenever the instance is `uninstalled` or selected/applied packages differ. `go test -count=1 ./internal/lifecycle ./internal/provisioning ./internal/updates`: PASS.
- Finding 2 RED: `TestCreateRejectsRuntimeImageOverride` returned `201` and persisted `attacker/runtime:latest`; the shared request type had accidentally exposed a non-goal.
- Finding 2 GREEN: `runtime_image` was removed from the strict shared create/edit contract, restoring the server-owned fixed runtime image. `go test -count=1 ./internal/httpapi`: PASS.
- Finding 3 RED: the desktop browser journey changed the card to package B only after the Job completed, never showing the persisted selected-B/applied-A `待应用` state.
- Finding 3 GREEN: the `202` branch starts Job polling and immediately reloads the instance list; the running card shows B plus `待应用`, then clears it after commit.
- Browser evidence gap closed: the final desktop/mobile journey creates two instances with independent packages and extra arguments, first-starts both, verifies their assignments through the real HTTP API, and then reconfigures only one.

### Fresh Final Verification

- `go test -count=1 ./...`: PASS, all Go packages.
- `go vet ./...`: PASS.
- `cd web && npm test -- --run`: PASS, 22 tests.
- `cd web && npm run build`: PASS, TypeScript and Vite production bundle.
- `cd web && npm run e2e`: PASS, 2/2 desktop and mobile projects.
- `docker compose --env-file .env.example config --quiet`: PASS.
- `git diff --check`: PASS after the evidence update.
- Browser geometry checks prove the preview follows the field grid at matching full width. Visual readback of the desktop and 390x844 mobile screenshots shows readable controls, fixed actions and no horizontal overflow; the mobile screenshot visibly includes the wrapped argv preview.

### Residual Risk

- The final session did not perform a fresh real Docker Engine/SteamCMD 12 GiB installation on a disposable Linux host. Maintenance command construction, first-start ordering, failure prevention and browser orchestration are covered by Go tests and the real-HTTP fixture; real network/download behavior remains an operational acceptance step.
- The documented raw `SRCDS_EXTRA_ARGS` fallback remains intentionally active for old/manual runtime definitions. All Panel-created containers prefer validated `SRCDS_EXTRA_ARGS_JSON`; removal waits for an explicit old-container upgrade policy.
