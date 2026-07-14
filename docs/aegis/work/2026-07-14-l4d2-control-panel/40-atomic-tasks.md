# Atomic task checkpoint

## TodoCheckpointDraft

- [x] SQLite WAL persistence, migrations, durable administrator credentials, hashed sessions, jobs, audit, schedules and encrypted secrets.
- [x] Restricted Docker Engine adapter, repository-owned socket proxy, managed labels, host networking, lifecycle, persistent container IDs and startup reconciliation.
- [x] Runtime Supervisor PTY, replay/reconnect, fixed attach/status/stop operations, restart-loop protection and exit-aware health reporting.
- [x] A2S query, status-to-UserID mapping, kick/ban commands and resource stats.
- [x] Chunked VPK storage, private overlay manager, protected ZIP/package ingestion, GitHub Release acquisition, hot/full/game update coordinators, backup and cleanup primitives.
- [x] Scheduler persistence/execution and online-player policy.
- [ ] Complete browser contract and E2E coverage (active: content, player and persistent Job surfaces are connected; full private/VPK controls, Job history and browser E2E still need audit).
- [ ] Complete fault-injection and recovery acceptance (job restart is marked interrupted; update-stage continuation/rollback and Docker/SRCDS interruption matrix still need proof).
- [ ] Complete fresh-host runtime acceptance (images and isolated stack work; actual App 222860 installation requires a subscribed Steam account).
- [ ] Production delivery documentation, TLS/reverse-proxy guidance and final branch integration.

## Active slice

Deploy the latest content/player/Job API and UI slice to the isolated `sirphomesv` smoke stack. Confirm that anonymous Steam installation exits promptly and the persistent Job becomes failed rather than waiting for the long health timeout. Then audit remaining design requirements and implement the next test-first slice.

## Completed in the latest slice

- Added authenticated persistent Job list and SSE feed.
- Added VPK download and private list/read/history/delete APIs.
- Added player actions, game/package update controls and a private text editor to the React UI.
- Rejected invalid private instance identifiers across every manager operation and stopped history responses leaking absolute host paths.
- Added nil-manager HTTP guards and prevented private apply after a failed save or without a selected instance.
- Added backend and React regression coverage for those boundaries.

## Blocked-on

- Valve currently rejects anonymous App 222860 installation with `Missing configuration`; AppInfo marks the Linux depots subscription-only. Full SRCDS/A2S/PTY acceptance needs credentials for a Steam account subscribed to L4D2. Credentials must be entered through `/api/settings/steam` or the Settings UI and must never be copied into repository artifacts or logs.

## Next

1. Commit this verified slice.
2. Rebuild/recreate the isolated remote Panel and runtime images from the commit.
3. Run anonymous start and verify fast failed-Job convergence.
4. Smoke VPK/private/package/scheduler/audit/SSE paths that do not require Steam credentials.
5. Continue requirement-by-requirement gap closure and browser E2E work.

## DriftCheckDraft

- Scope: continues to implement the approved single-host, single-admin design.
- Compatibility: host networking, fixed Supervisor Exec operations and content precedence are unchanged.
- New owner/fallback: none; private-path validation remains owned by `PrivateManager`, and Docker authority remains behind the restricted proxy.
- Decision: `continue`; full acceptance remains blocked only where real App 222860 runtime behavior requires licensed credentials.
