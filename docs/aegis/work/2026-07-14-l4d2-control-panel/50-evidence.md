# Evidence bundle

## Fresh local verification (2026-07-15)

- For the A2S/GameHost slice, `go test -count=1 -p 1 ./...` passed after using an isolated worktree `GOTMPDIR`; `go vet ./...`, `docker compose --env-file .env.example config --quiet`, and `git diff --check` exited 0.
- The first fresh full Go run hit the already-observed Windows executable lock on `maintenance.test.exe`. `go test -count=10 ./internal/maintenance` then passed, followed by the complete serial run passing. The verification wrapper now preserves the `go test` exit code across temporary-directory cleanup.
- With `GOTMPDIR` isolated under the worktree, `go test -count=1 -p 1 ./...`: all Go packages passed, including the content and HTTP regression tests added in the current slice. The temporary directory was removed after the run.
- `go vet ./...`: exit 0.
- `cd web && npm test -- --run`: 7/7 React component tests passed.
- `cd web && npm run build`: TypeScript and Vite production build exited 0.
- `docker compose --env-file .env.example config --quiet`: exit 0.
- `git diff --check`: no whitespace errors.
- Root deployment regression test verifies the Panel and Docker proxy both bind loopback under host networking.

The default Windows temporary directory intermittently locked randomly named Go test executables (`maintenance.test.exe`, then `health.test.exe`). Both affected packages passed 10 consecutive targeted runs; the complete serial run passed after isolating `GOTMPDIR`. No product-code fallback was added for this host-only verification issue.

Targeted red/green evidence in the latest slice:

- Before the A2S protocol fix, `TestQueryInfoAndPlayers` failed with `unexpected A2S_INFO response` when the test server returned `0x41`; after copying the challenge into a repeated INFO request, the protocol test passed.
- A separate direct-`0x49` test preserves compatibility with servers that do not challenge INFO requests. A temporary mutation that rejected direct INFO made only that test fail; restoring `client.go` returned the A2S package to green.
- Before the configuration fix, Panel startup silently fell back to `127.0.0.1`; `TestLoadRequiresGameHost` now proves startup rejects a missing `L4D2_PANEL_GAME_HOST`, and the deployment test proves Compose requires it explicitly.
- Private history originally exposed the absolute temporary/data root, and invalid instance IDs were accepted by list/history/apply; the new content tests failed on both observations before the manager validation fix.
- Content read routes originally panicked on a nil `UploadManager`; the new HTTP test captured the nil dereference before handlers were aligned with the existing structured `503` contract.
- React tests originally showed that a failed private save continued to queue apply and that update buttons remained enabled without an instance; both tests passed after the UI boundary fix.

## Prior isolated Linux smoke evidence

- Host: SSH alias `sirphomesv`; Linux Docker 29.6.1 and Compose 5.2.0.
- Isolated source/data: `/tmp/l4d2-panel-smoke-src` and `/srv/l4d2-panel-smoke-data`.
- Panel, runtime and repository-owned restricted socket-proxy images built successfully with digest-pinned base images and the authorized download proxy.
- Supervisor self-test passed and the Compose stack reached healthy on `127.0.0.1:18080`.
- Health API, SPA, login and instance creation worked.
- Docker proxy allowed `/info` and denied `/volumes` with HTTP 403.
- Administrator session and instance data survived a Panel restart.
- The managed game container used host networking, UID 10001, persistent game/private/shared mounts, required labels and proxy environment.

## Steam runtime finding

- Linux-only anonymous first install reproducibly failed with `Missing configuration`, but that was not a licensing requirement.
- The upstream `Left4DevOps/l4d2-docker` commit `761f007985ab511186d4f438765c08d7ff1b3364` identified the current first-install workaround: install App 222860 with `@sSteamCmdForcePlatformType windows`, then switch to `linux` and validate.
- An isolated runtime container executed that exact fixed sequence anonymously. The Windows depot downloaded/verified 9,710,287,462 bytes, then the Linux transition downloaded approximately 202,762,857 bytes. Both stages reported `Success! App '222860' fully installed` and the container exited 0.
- The smoke game directory contains `srcds_run` and `steamapps/appmanifest_222860.acf`; installed size is approximately 9.3 GB.
- The Supervisor now uses the dual-platform sequence only for an anonymous missing-game install. Licensed credentials remain encrypted optional settings and are not logged or committed.

## A2S runtime finding

- The installed SRCDS instance answered A2S immediately at the host LAN/Tailscale-reachable address but timed out when queried through `127.0.0.1`, even though it was bound for host-network gameplay.
- Its first A2S_INFO response was a Source challenge packet (`0x41`) rather than direct INFO data (`0x49`). The previous client rejected that legal response, independently of the target-address problem.
- The canonical A2S target is now the required `L4D2_PANEL_GAME_HOST` configuration consumed by both health reconciliation and player queries. The implicit `127.0.0.1` fallback has been removed instead of retained as a second owner.

## Latest isolated Linux smoke (2026-07-15)

- Deployed commit `b8a5bb0` to `sirphomesv`, rendered Compose successfully, rebuilt the Panel image and force-recreated only the Panel service. The managed game container ID remained unchanged during this operation.
- `/api/health` returned `status=ok`, SQLite and Docker were available, and Compose later reported the Panel healthy. `ss` showed only `127.0.0.1:18080` for Panel and `127.0.0.1:23759` for the restricted proxy.
- Startup recovery marked the previously active start Job `interrupted` with a diagnostic message and reconciled instance `b2bf3fe9-baea-4d3a-9a12-18be64ee043b` to actual state `running` without restarting SRCDS.
- An authenticated `/players` request returned map `c2m1_highway`, maximum players 4 and an empty player list. This exercises both challenged A2S_INFO and A2S_PLAYER against the real server through `L4D2_PANEL_GAME_HOST=192.168.0.102`.
- A raw authenticated WebSocket smoke received HTTP 101, sent a harmless unique `echo` marker through the fixed Supervisor attach path, disconnected, reconnected and found that marker in the replay buffer.
- The interrupted old start left `DesiredState=stopped` even though reconciliation correctly found `ActualState=running`; the next explicit stop/start will align the desired state. No fallback or automatic game restart was introduced.
- Panel API stop, start and same-configuration rebuild Jobs all reached `succeeded`. Stop produced `stopped/stopped`; start and rebuild produced `running/running`. Rebuild replaced container `e7300291f4a4...` with `f66b76805fab...` while SHA-256 for `srcds_run` and `steamapps/appmanifest_222860.acf` remained unchanged; A2S still returned `c2m1_highway` afterward.
- VPK smoke completed begin/chunk/complete/list/download/rename/delete with byte-for-byte download verification. Private overlay smoke saved two versions, listed/read history, applied through a successful Job and deleted the source. The known effective smoke file was then removed explicitly.
- A protected hot-compatible ZIP was uploaded and listed, then its returned UUID artifacts were removed from the isolated smoke data because no package-delete API is exposed. A disabled cleanup schedule was saved, run explicitly with a ten-year retention period, observed as a successful `scheduled_cleanup` Job and deleted.
- Job SSE returned a decoded `event: jobs` frame containing 10 persistent Jobs. Audit returned 22 events and included successful mutations for VPK, private, package and schedule endpoints. Follow-up list checks found no smoke VPK, private source, package or schedule.

## Residual acceptance gaps

- Verify the game-update maintenance path against the installed SRCDS when another full Steam validation is acceptable for the host disk/time budget.
- Browser E2E for login, create, console, upload, update, Cron and recovery.
- Full fault injection for Panel termination during update, SRCDS termination, Docker restart and interrupted download.
- Verify maintenance-writer adoption and desired-running recovery after interrupted game updates.

## Requirement-gap audit (2026-07-15)

- A real Chromium walk against a temporary Go/SQLite server reproduced a `400` when the React Cron form sent snake_case fields to an untagged `ScheduledTask` under strict JSON decoding. Login and instance creation completed, proving this was a route-contract defect rather than a fixture failure.
- Browser review found direct `confirm: true` calls for game/full updates and player actions, one-shot VPK PATCH despite the server's 64 MiB chunk cap, no Playwright setup, missing `interrupted` terminal handling, swallowed errors and an unconditional Docker-online label.
- Recovery review found that package rollback state exists only in memory, full-update health failure occurs after backups are deleted, maintenance containers are omitted from startup reconciliation, and Panel shutdown uses `log.Fatal` without HTTP drain or active-Job waiting.
- The additive remediation plan is recorded as Tasks 8-12. Current priority is the directly reproduced browser contract/safety slice, followed by auxiliary ports, durable update journal/shutdown, maintenance/artifact recovery and real-browser/fault acceptance.

## Task 8 browser-contract repair (2026-07-15)

- RED evidence: the focused Vitest run reported seven failures covering Docker health text, Cron success/error, game/full-update confirmation, player confirmation and interrupted Job convergence; the added Steam settings test separately failed on the old licensed-account warning. The schedule HTTP regression had previously failed because strict decoding rejected browser snake_case fields.
- `ScheduledTask` now has explicit snake_case JSON tags and the HTTP test proves accepted public fields still reject an unknown `surprise` field with HTTP 400.
- Game updates, full package updates, player kicks and permanent bans now stop at an accessible confirmation dialog before sending `confirm:true`; the existing server-side HTTP 428 enforcement remains the authorization owner.
- VPK uploads calculate SHA-256 incrementally and send sequential 8 MiB PATCH bodies. The regression uses a file larger than one chunk, observes offsets `0` and `8388608`, and proves the whole-file `arrayBuffer()` path is unused.
- The UI now exposes Cron pending/success/error, content/player/settings failures, the real health endpoint result and interrupted Job errors. Steam settings correctly describe anonymous installation as supported, matching upstream commit `761f007985ab511186d4f438765c08d7ff1b3364` and the direct Linux smoke.
- Fresh verification passed: `go test -count=1 -p 1 ./...`, `go vet ./...`, `npm test -- --run` (16 tests), `npm run build`, `npm audit --omit=dev` (0 vulnerabilities), `docker compose --env-file .env.example config --quiet`, and `git diff --check`.
- Review judgment: no Critical or Important Task 8 finding. Real HTTP, Secure-cookie, SSE/WebSocket, focus-trap and responsive Chromium journeys remain explicitly assigned to Task 12 rather than inferred from jsdom.

## Task 9 managed-port completion (2026-07-15)

- RED evidence: store tests could not compile without `PluginPorts`, the migration test failed because `instance_plugin_ports` did not exist, HTTP strict decoding rejected `sourcetv_port`, lifecycle checked only `27015`, Docker omitted the new environment values, the React form had no corresponding fields, and the portable runtime source assertion found no `SRCDS_TV_PORT` support.
- SQLite now creates additive migration version 2 and transactionally stores sorted plugin ports with instance creation/update. Reopen round trips and `ON DELETE CASCADE` cleanup are covered.
- `ports.Checker` consumes real database reservations for game, SourceTV and plugin ports, excludes the current instance, rejects duplicate/invalid declarations, propagates provider errors and rejects host listeners. Lifecycle checks every declared port before starting an existing or newly created container.
- Docker passes `SRCDS_TV_PORT` and `L4D2_PLUGIN_PORTS`; the Supervisor only appends `+tv_enable 1 +tv_port <port>` for a nonzero TV port. The API and React create/card flow expose the same declarations.
- On `sirphomesv`, direct Docker socket access as the SSH user was denied, while the existing restricted proxy at `127.0.0.1:23759` and `sudo -n docker` both reported Docker 29.6.1. No daemon change or restart was needed.
- The copied current Supervisor passed its Linux PTY self-test. With `SRCDS_TV_PORT=0`, the rendered command had no TV arguments; with `27020`, it ended in `+tv_enable 1 +tv_port 27020`. Temporary remote files were removed.
- Fresh verification passed: `go test -count=1 -p 1 ./...`, `go vet ./...`, `npm test -- --run` (17 tests), `npm run build`, `npm audit --omit=dev` (0 vulnerabilities), `docker compose --env-file .env.example config --quiet`, and `git diff --check`.
- Review judgment: no Critical or Important Task 9 finding. Port allocation remains intentionally advisory at the host-listener boundary; the database declaration check prevents cross-instance races inside the single Panel owner.

## Task 10 durable-update and shutdown completion (2026-07-15)

- RED evidence: updates had no `Begin`/`Recover` transaction contract, a restart-health failure never rolled files back, `jobs.Manager.Start` could not return the initial `SaveJob` failure, no active-job Wait existed, and `cmd/panel` had no shutdown coordinator.
- Package deployment now backs up every package/private affected path plus the prior manifest, then fsyncs and atomically renames `prepared`, `applying`, `deployed` and `committed` journal stages below `instances/<id>/backups/update-*`. Recovery validates journal identity and safe relative paths before rolling back any uncommitted stage.
- Coordinator retains the transaction through restart health. Health or pre-commit journal failure stops the new run, restores old files/manifest and restarts the previous version; successful commit is the only point that retires backups.
- Initial Job persistence is now a prerequisite for launching its goroutine. HTTP and scheduler callers propagate that failure, and `jobs.Manager.Wait` provides context-bounded drain for all accepted work.
- Panel startup recovers package journals before container reconciliation. SIGINT/SIGTERM shutdown is ordered HTTP, bounded scheduler stop, then Job wait; the old `log.Fatal(server.ListenAndServe())` path is gone.
- Fresh local verification passed: `go test -count=1 -p 1 ./...`, `go vet ./...`, `npm test -- --run` (17 tests), `npm run build`, `npm audit --omit=dev` (0 vulnerabilities), Compose config and `git diff --check`. A transient Windows executable lock occurred on the first runtime run; a standalone runtime binary passed 10 repetitions and a fresh full run then passed.
- A disposable static Linux Panel on `sirphomesv` served healthy at `127.0.0.1:18081` through the existing restricted Docker proxy, received Docker SIGTERM, logged its drain and exited `0` without OOM. The managed game container remained `f66b76805fab` before and after; no Docker daemon change/restart occurred. All Task 10 remote artifacts were removed.
- Windows race instrumentation was unavailable because the local Go toolchain has CGO disabled. Review found no Critical or Important Task 10 issue; Linux process behavior and all deterministic transaction/drain paths have direct regression coverage.

## Task 11 maintenance and artifact recovery (2026-07-15)

- RED evidence: Docker tests showed the role-specific scan hid maintenance containers, update retries created a new writer and wait errors deleted the unclassified writer. Lifecycle tests showed maintenance could replace the persisted game container and did not block mutations. Game-update tests showed duplicate Stop, lost desired-running state, unconditional restart and stale fault writes. VPK, Release and backup tests reproduced offset divergence, unmanaged download staging and publication of a canceled archive.
- `ListManaged` now scans every managed container, while reconciliation adopts only explicit `role=game` and classifies explicit `role=maintenance` as `updating`. Missing or unknown roles remain pending objects. Start, Stop, Rebuild and Delete reject a same-instance maintenance artifact.
- `UpdateGame` adopts one existing writer, starts it only from `created`, retains it when Docker wait is interrupted, and removes it only after exit classification. A retry reuses that container. The command remains fixed to anonymous or configured login with Linux App 222860 validate; first installation still follows upstream `Left4DevOps/l4d2-docker@761f007985ab511186d4f438765c08d7ff1b3364` (`windows` bootstrap then `linux` validate).
- `GameCoordinator` checkpoints `ActualState=updating` while preserving desired-running state, skips duplicate Stop during adoption, leaves desired-stopped instances stopped, and faults the latest database record rather than an update-start snapshot.
- VPK recovery treats `.part` size as the offset authority for Write and Complete, syncing appended data before metadata. GitHub Release assets stage as managed `packages/uploads/release-*.part` files. Backups use `.partial`, close tar/gzip, fsync the file, atomically rename and sync the directory before returning a final `.tar.gz`.
- Fresh local evidence: every `go list ./...` package passed independently with `-count=1`; the seven Task 11 target packages passed again sequentially; `internal/updates` passed 10 repetitions; `go vet ./...`, 17 Vitest tests, the Vite production build, Compose config and `git diff --check` exited 0. Multi-package Windows runs intermittently hit the previously documented executable/temp cleanup locks; no product fallback was added.
- Fresh Linux evidence: a digest-pinned Go builder compiled the complete synchronized source and six Task 11 Go packages passed in the Linux container. A disposable Docker integration fixture forced the first maintenance wait to time out, observed the same writer still present, retried and adopted it, then observed classification and removal. The fixture and builder image were removed; the only remaining managed container was the original running game `f66b76805fab`. The runtime Supervisor module self-test exited 0. Docker daemon configuration and process were unchanged.
- Review judgment: no Critical or Important Task 11 finding remains after tightening role adoption. Task 12 still owns real-browser recovery journeys and the broader safe interruption matrix.

## Evidence judgment

Confidence is A for the fresh local test/build claims, browser contract regressions, direct anonymous Steam installation, writer adoption and the exercised Panel-managed SRCDS/A2S/PTY/lifecycle journey. Confidence remains B for real-browser recovery and the broader fault-injection paths assigned to Task 12. This bundle is verified evidence, not an authoritative completion signal.
