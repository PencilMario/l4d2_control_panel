# Atomic task checkpoint

## TodoCheckpointDraft

- [x] SQLite WAL persistence, migrations, durable administrator credentials, hashed sessions, jobs, audit, schedules and encrypted secrets.
- [x] Restricted Docker Engine adapter, repository-owned socket proxy, managed labels, host networking, lifecycle, persistent container IDs and startup reconciliation.
- [x] Runtime Supervisor PTY, replay/reconnect, fixed attach/status/stop operations, restart-loop protection and exit-aware health reporting.
- [x] A2S query, status-to-UserID mapping, kick/ban commands and resource stats.
- [x] Chunked VPK storage, private overlay manager, protected ZIP/package ingestion, GitHub Release acquisition, hot/full/game update coordinators, backup and cleanup primitives.
- [x] Scheduler persistence/execution and online-player policy.
- [x] Complete browser contract and E2E coverage with real HTTP desktop/mobile Playwright journeys.
- [x] Complete the safe shared-host fault-injection and recovery matrix; Docker-daemon restart and ENOSPC remain dedicated-host-only acceptance.
- [ ] Complete fresh-host runtime acceptance (anonymous install and Panel-managed SRCDS/A2S/PTY/lifecycle persistence are verified; game-update maintenance remains).
- [x] Production delivery documentation, TLS/reverse-proxy guidance and final branch integration.
- [x] Repair browser/API contracts, destructive confirmations, large VPK chunking and truthful loading/error/health states.
- [x] Declare, persist and reserve SourceTV/plugin ports across API, lifecycle, runtime and UI.
- [x] Add durable package-update journal recovery and bounded Panel shutdown/Job drain.
- [x] Reconcile maintenance writers and recover interrupted uploads/backups/game updates.
- [x] Add real-HTTP Playwright acceptance and the safe Linux fault-injection matrix.

## Active slice

No active implementation slice. The verified feature branch was fast-forwarded into `main`.

## Completed in the latest slice

- Added authenticated persistent Job list and SSE feed.
- Added VPK download and private list/read/history/delete APIs.
- Added player actions, game/package update controls and a private text editor to the React UI.
- Rejected invalid private instance identifiers across every manager operation and stopped history responses leaking absolute host paths.
- Added nil-manager HTTP guards and prevented private apply after a failed save or without a selected instance.
- Added backend and React regression coverage for those boundaries.
- Replaced the incorrect licensed-Steam assumption with a tested anonymous Windows-to-Linux first-install bootstrap.
- Added the persistent Job/SSE page plus VPK download/rename/delete and private file tree/edit/download/history/delete controls.
- Bound both host-network control services to loopback and refreshed deployment/TLS documentation.
- Traced the remote A2S failure to two independent runtime contracts: SRCDS answers the host LAN address but not loopback, and its INFO response first returns a `0x41` challenge before `0x49` data.
- Added A2S INFO challenge handling and made the SRCDS-reachable game host a required configuration value shared by health and player queries; the old loopback query fallback is retired.
- Rebuilt only the remote Panel while preserving the running game container; health, loopback control listeners and startup reconciliation all recovered.
- Verified the old active Job became `interrupted`, the instance reconciled to `running`, and the authenticated players endpoint returned `c2m1_highway` without A2S timeout.
- Verified the native console WebSocket upgraded, accepted a harmless `echo` command, and replayed its marker after disconnect/reconnect.
- Verified Panel-managed stop/start and same-configuration rebuild Jobs all succeeded; desired/actual state converged to `running`, the game container ID changed, and persistent game file hashes remained unchanged.
- Exercised VPK upload/chunk/download/rename/delete, private save/read/history/apply/delete, protected ZIP upload/list, Cron save/run/delete, Job SSE and audit records on Linux, then removed every temporary smoke object.
- Made `ScheduledTask` own an explicit snake_case JSON contract while preserving strict unknown-field rejection.
- Added visible Cron, content, player, settings and Docker health error states; Job polling now classifies `interrupted` as terminal.
- Added explicit confirmation dialogs for game updates, full package updates, player kicks and permanent bans without weakening server-side confirmation enforcement.
- Replaced whole-file VPK hashing and one-shot PATCH with incremental SHA-256 and sequential 8 MiB chunks.
- Corrected the Steam settings copy to state that anonymous dual-platform installation is supported and licensed credentials are optional.
- Added additive plugin-port persistence and explicit SourceTV/plugin JSON fields, with transactional instance CRUD and cascade cleanup.
- Made `ports.Checker` the sole configured/listening conflict owner, including all declared port kinds, current-instance exclusion and real SQLite reservations.
- Passed SourceTV/plugin declarations through Docker and the React create/display flow; Supervisor enables SourceTV only for a nonzero managed port.
- Verified the POSIX command path and Supervisor PTY self-test on `sirphomesv` without changing or restarting Docker daemon.
- Replaced in-memory-only package rollback with atomic stage journals that retain backups until explicit commit and restore files plus the package manifest on startup.
- Made full package updates commit only after restart health succeeds; failed health or journal commit stops the new run, rolls back and restarts the old version.
- Made initial Job persistence mandatory, tracked accepted goroutines for context-bounded drain, and propagated creation errors through HTTP and scheduled dispatch.
- Replaced fatal serving with SIGINT/SIGTERM handling ordered as HTTP shutdown, bounded Cron stop and Job drain.
- Verified a disposable Linux Panel received Docker SIGTERM, logged drain, exited 0 and left the existing game container ID unchanged.
- Expanded managed-container scans to include maintenance writers while requiring an explicit valid role before reconciliation can adopt a container.
- Blocked same-instance lifecycle mutations while a maintenance artifact remains unclassified; game-update retries adopt the existing writer instead of creating a second one.
- Persisted the pre-update desired-running intent and made game-update fault writes merge against the latest instance record.
- Recovered VPK offsets from `.part` length, staged Release downloads below managed package storage and published backups only after close/fsync plus atomic rename.
- Verified writer retention and adoption against Docker 29.6.1 on `sirphomesv`; the existing game container remained unchanged and no maintenance container was left behind.
- Added a build-tagged Go/SQLite fixture and pinned Playwright 1.61.1 desktop/mobile journeys over real HTTP, Secure cookies, SSE and WebSocket.
- Covered login, instance creation, refresh recovery, console reconnect, confirmation focus trapping, a 9 MiB two-chunk VPK, package/private updates, Cron persistence and deterministic succeeded/failed/running/interrupted Jobs.
- Corrected mobile Job status truthfulness, isolated scrolling above the fixed navigation and icon-only navigation overflow with red/green browser geometry assertions.
- Made the local Playwright fixture bypass configured download proxies while keeping the browser journey on the loopback origin.
- Added fixture startup package-journal recovery and proved hard Panel, VPK metadata divergence, deployed package and bounded fake-SRCDS interruption on `sirphomesv`.
- Verified the remote matrix removed every Task 12 artifact, preserved game container `f66b76805fab` and left the Docker daemon signature unchanged.

## Blocked-on

- No current external blocker. Steam Guard remains unverified for the optional licensed-account path, but anonymous installation no longer depends on credentials.

## Next

1. Run Docker-daemon restart, ENOSPC and another full game-update validation only on a dedicated disposable host with sufficient disk.
2. Verify optional licensed Steam Guard behavior only if that deployment path is needed.

## DriftCheckDraft

- Scope: continues to implement the approved single-host, single-admin design.
- Compatibility: host networking, fixed Supervisor Exec operations and content precedence are unchanged.
- New owner/fallback: Playwright owns real-browser acceptance; its build-tagged fixture reuses production HTTP/auth/store/content/update owners and replaces only external Docker/SRCDS/A2S/Steam/GitHub boundaries. No production fallback was added.
- Decision: `continue`; Task 12 is verified inside the approved API, authentication, content precedence and fixed Supervisor boundaries. Docker restart, ENOSPC and optional Steam Guard remain explicit external acceptance gaps rather than implementation fallbacks.
