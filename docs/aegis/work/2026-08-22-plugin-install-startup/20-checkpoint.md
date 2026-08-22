# Checkpoint

## Current todo

- [x] Read baseline instructions, project context, existing recovery implementation, and remote evidence.
- [x] Add and run RED regression tests.
- [x] Implement lifecycle/provisioning/Compose repair.
- [x] Run local verification.
- [x] Deploy and repair existing remote package ownership.
- [x] Verify plugin installation, Overlay recovery, and instance startup through the background task/API.

## Active slice

Complete final evidence and leave the remote verification instance stopped.

## Evidence

- Worktree was clean at `b281cdc` before this task.
- Existing startup recovery is present in `d26591e` plus the safe current-release fallback from `b52b686`.
- Remote root cause and current failed shared-game state are recorded in `10-baseline-readset.md`.
- RED/GREEN evidence: lifecycle tests first observed `accelerator -> start` and `accelerator -> create`; both now assert `overlay -> accelerator -> create/start`.
- Focused verification passed for `internal/lifecycle`, `internal/provisioning`, `internal/updates`, and the deployment package.
- Fresh local verification passed: `go test -count=1 -p 1 . ./internal/lifecycle ./internal/provisioning ./internal/updates`, `go vet ./...`, `go build ./cmd/panel`, and `git diff --check`.
- 琥珀 Panel release `plugin-startup-b281cdc` is running image `sha256:47c4cb7ca7469ec5624a068268fe68ba921fbacfa4f8982f17a033a6fce830ae`; Panel health is `status=ok`, database `ok`, Docker `27.2.0`.
- Existing package tree was repaired with `sudo chown -R 10001:10001 /srv/l4d2-panel/packages`; package manifests and archives are readable by the Panel UID.
- Instance `16a28495-d5c9-4645-a88e-a20758a727ca` package reinstall job `cad80c0c-31e2-4ebf-9ce8-e58f6932a461` succeeded without `permission denied`.
- The same instance start job `d00b1197-bea6-4e39-8cc2-38966eb684da` succeeded; its existing container ID remained `4458683fd3af294565fc449e269f35e7de67b3902277bc549c9463eb692d1bf0`.
- The target Overlay mount was present with lower release `89ff6353-fafe-48b2-8c0a-fb949e2744fa`; UID `10001` read `overlay/merged/left4dead2/addons/sourcemod/configs/core.cfg` successfully.
- The verification instance was returned to `desired=stopped`, `actual=stopped`; the other six instances remained stopped.

## Drift check

- Scope remains limited to package readability and Overlay-before-start ordering.
- No new privileged owner or public API is planned.
- The fallback remains bounded to a validated `game/current` link.
- Remote runtime configuration had an empty `L4D2_PANEL_CRASH_REPORT_TOKEN`; the value already present in the previous install configuration was synchronized into the active release so enabled Accelerator reconciliation could complete. No code or public API change was needed for this pre-existing deployment configuration gap.
- A first remote package task failed only at Accelerator reinstall because that token was empty; it was not a package permission or Overlay failure. The retry after configuration repair succeeded.

## Next

Review the final diff and decide whether to commit the repair. Keep the remote token/configuration migration in the deployment runbook if releases are created outside `deploy.sh`.
