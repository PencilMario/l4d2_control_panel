# Evidence bundle

## Fresh local verification (2026-07-15)

- With `GOTMPDIR` isolated under the worktree, `go test -count=1 -p 1 ./...`: all Go packages passed, including the content and HTTP regression tests added in the current slice. The temporary directory was removed after the run.
- `go vet ./...`: exit 0.
- `cd web && npm test -- --run`: 7/7 React component tests passed.
- `cd web && npm run build`: TypeScript and Vite production build exited 0.
- `docker compose --env-file .env.example config --quiet`: exit 0.
- `git diff --check`: no whitespace errors.
- Root deployment regression test verifies the Panel and Docker proxy both bind loopback under host networking.

The default Windows temporary directory intermittently locked randomly named Go test executables (`maintenance.test.exe`, then `health.test.exe`). Both affected packages passed 10 consecutive targeted runs; the complete serial run passed after isolating `GOTMPDIR`. No product-code fallback was added for this host-only verification issue.

Targeted red/green evidence in the latest slice:

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

## Residual acceptance gaps

- Rebuild the remote smoke stack from the latest commit and verify exit-aware failed-Job convergence.
- Exercise VPK/private/package/scheduler/audit/SSE paths on Linux against the latest commit.
- Start the installed SRCDS through the rebuilt Panel/runtime, verify A2S, PTY attach/input/reconnect, stop/start/rebuild persistence and game update.
- Browser E2E for login, create, console, upload, update, Cron and recovery.
- Full fault injection for Panel termination during update, SRCDS termination, Docker restart and interrupted download.
- Verify or strengthen true stage-journal continuation/rollback rather than only marking active jobs interrupted.

## Evidence judgment

Confidence is A for the fresh local test/build claims and direct anonymous Steam installation, and B for the previously exercised isolated Docker contracts. The Panel-managed SRCDS main journey is still pending after the runtime rebuild. This bundle is verified evidence, not an authoritative completion signal.
