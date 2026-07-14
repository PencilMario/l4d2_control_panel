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
- [ ] Complete fresh-host runtime acceptance (anonymous App 222860 dual-platform installation is verified; Panel-managed SRCDS/A2S/PTY start and restart persistence are the active checks).
- [ ] Production delivery documentation, TLS/reverse-proxy guidance and final branch integration.

## Active slice

Deploy the anonymous first-install fix and expanded content/Job UI to the isolated `sirphomesv` smoke stack. Start the installed SRCDS through the Panel and verify A2S, PTY, Job completion and restart persistence. Then exercise the content/scheduler/audit paths and continue the fault-recovery audit.

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

## Blocked-on

- No current external blocker. Steam Guard remains unverified for the optional licensed-account path, but anonymous installation no longer depends on credentials.

## Next

1. Commit this verified slice.
2. Rebuild/recreate the isolated remote Panel and runtime images from the commit.
3. Start the anonymously installed game through the Panel and verify SRCDS/A2S/PTY/Job state.
4. Smoke VPK/private/package/scheduler/audit/SSE paths.
5. Continue requirement-by-requirement gap closure and browser E2E work.

## DriftCheckDraft

- Scope: continues to implement the approved single-host, single-admin design.
- Compatibility: host networking, fixed Supervisor Exec operations and content precedence are unchanged.
- New owner/fallback: none; private-path validation remains owned by `PrivateManager`, and Docker authority remains behind the restricted proxy.
- Decision: `continue`; the licensed-credential assumption was retired after direct dual-platform anonymous-install evidence.
