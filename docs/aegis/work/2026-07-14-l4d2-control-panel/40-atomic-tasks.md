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
- [ ] Declare, persist and reserve SourceTV/plugin ports across API, lifecycle, runtime and UI.
- [ ] Add durable package-update journal recovery and bounded Panel shutdown/Job drain.
- [ ] Reconcile maintenance writers and recover interrupted uploads/backups/game updates.
- [ ] Add real-HTTP Playwright acceptance and the safe Linux fault-injection matrix.

## Active slice

Declare and reserve SourceTV and plugin ports across persistence, API, lifecycle, runtime and UI, then continue through Tasks 10-12 for journal recovery, maintenance recovery and browser/fault acceptance.

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

## Blocked-on

- No current external blocker. Steam Guard remains unverified for the optional licensed-account path, but anonymous installation no longer depends on credentials.

## Next

1. Implement SourceTV/plugin-port persistence, conflict checking, runtime arguments and UI fields.
2. Implement durable update-stage recovery and graceful shutdown with regression tests.
3. Reconcile maintenance writers and interrupted artifacts.
4. Add real HTTP Playwright coverage and run the remaining safe fault-injection matrix.

## DriftCheckDraft

- Scope: continues to implement the approved single-host, single-admin design.
- Compatibility: host networking, fixed Supervisor Exec operations and content precedence are unchanged.
- New owner/fallback: `Config.GameHost` remains the sole A2S target owner. `ScheduledTask` JSON tags now own the browser schedule contract; UI dialogs add no second authorization owner because HTTP confirmation remains enforced server-side.
- Decision: `continue`; Task 8 stayed within the approved browser/API boundary, retired one-shot VPK upload and direct destructive submissions, and introduced no compatibility fallback.
