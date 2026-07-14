# Atomic task checkpoint

## TodoCheckpointDraft

- [x] SQLite WAL persistence, migrations, durable administrator credentials, hashed sessions, jobs, audit, schedules and encrypted secrets.
- [x] Restricted Docker Engine adapter, repository-owned socket proxy, managed labels, host networking, lifecycle, persistent container IDs and startup reconciliation.
- [x] Runtime Supervisor PTY, replay/reconnect, fixed attach/status/stop operations, restart-loop protection and exit-aware health reporting.
- [x] A2S query, status-to-UserID mapping, kick/ban commands and resource stats.
- [x] Chunked VPK storage, private overlay manager, protected ZIP/package ingestion, GitHub Release acquisition, hot/full/game update coordinators, backup and cleanup primitives.
- [x] Scheduler persistence/execution and online-player policy.
- [ ] Complete browser contract and E2E coverage (browser/API contract regressions are repaired; real HTTP Playwright coverage remains).
- [ ] Complete fault-injection and recovery acceptance (job restart is marked interrupted; update-stage continuation/rollback and Docker/SRCDS interruption matrix still need proof).
- [ ] Complete fresh-host runtime acceptance (anonymous install and Panel-managed SRCDS/A2S/PTY/lifecycle persistence are verified; game-update maintenance remains).
- [ ] Production delivery documentation, TLS/reverse-proxy guidance and final branch integration.
- [x] Repair browser/API contracts, destructive confirmations, large VPK chunking and truthful loading/error/health states.
- [x] Declare, persist and reserve SourceTV/plugin ports across API, lifecycle, runtime and UI.
- [ ] Add durable package-update journal recovery and bounded Panel shutdown/Job drain.
- [ ] Reconcile maintenance writers and recover interrupted uploads/backups/game updates.
- [ ] Add real-HTTP Playwright acceptance and the safe Linux fault-injection matrix.

## Active slice

Persist package-update transactions and add bounded Panel shutdown/Job drain, then continue through Tasks 11-12 for maintenance recovery and browser/fault acceptance.

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

## Blocked-on

- No current external blocker. Steam Guard remains unverified for the optional licensed-account path, but anonymous installation no longer depends on credentials.

## Next

1. Implement durable update-stage recovery and graceful shutdown with regression tests.
2. Reconcile maintenance writers and interrupted artifacts.
3. Add real HTTP Playwright coverage and run the remaining safe fault-injection matrix.

## DriftCheckDraft

- Scope: continues to implement the approved single-host, single-admin design.
- Compatibility: host networking, fixed Supervisor Exec operations and content precedence are unchanged.
- New owner/fallback: `ports.Checker` now owns configured and listening conflicts using SQLite-backed reservations. The empty production provider, single-game-port check and dormant SourceTV path are retired; no fallback was added.
- Decision: `continue`; Task 9 stayed within the approved host-network port boundary and fresh local/Linux evidence covers its public and runtime contracts.
