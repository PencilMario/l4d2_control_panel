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
- Verify or strengthen true stage-journal continuation/rollback rather than only marking active jobs interrupted.

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

## Evidence judgment

Confidence is A for the fresh local test/build claims, browser contract regressions, direct anonymous Steam installation and the exercised Panel-managed SRCDS/A2S/PTY/lifecycle journey. Confidence remains B for recovery paths not yet fault-injected. This bundle is verified evidence, not an authoritative completion signal.
