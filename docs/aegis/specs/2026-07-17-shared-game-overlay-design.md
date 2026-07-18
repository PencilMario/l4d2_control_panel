# Shared Game Body And Instance Overlay Design

## Intent

All game instances use one L4D2 installation while retaining independent plugin packages and private files. Game-body updates become global operations. A global update may proceed only when every dependent server satisfies the selected online-player policy.

The shared installation is the authoritative game baseline. Instance-local mutations are overlays, not another copy of the game. Managed content operations restore affected paths to the expected shared/package/private result when a server has changed them at runtime.

## Storage Model

```text
<data-root>/
  game/releases/<release-id>/
  game/staging/<job-id>/
  game/current
  instances/<instance-id>/overlay/{upper,work,merged}/
  instances/<instance-id>/private/
  shared-vpk/
```

`game/releases/*` is immutable after publication. Every instance mount uses the resolved active release as `lowerdir`, its own `upper` and `work`, and exposes `merged` to the game container. The game container remains unprivileged and receives only `instances/<id>/overlay/merged:/opt/l4d2/game`.

The Panel never treats `upper` as durable configuration. Package archives, selected package metadata, and the private workspace remain the sources of truth.

## OverlayFS Authority

A dedicated `overlay-helper` service owns mount operations. It has no network, mounts only the configured data root, exposes a group-restricted Unix socket, validates identifiers, and supports only preflight, ensure, inspect, managed-path reset, and unmount.

Only this helper receives `CAP_SYS_ADMIN`. The Panel and game containers remain unprivileged. The data-root bind uses shared propagation so helper mounts are visible to the host Docker daemon. Preflight fails closed when the host lacks OverlayFS, `d_type`, same-filesystem upper/work support, or shared propagation.

## Instance Content Semantics

Content precedence remains shared game release, selected plugin package, private files, then runtime writes. Existing package and private manifests define managed paths.

- Hot package/private apply resets paths owned by the old or new operation manifest, including whiteouts, then writes expected content. Unrelated runtime files remain.
- Full package reinstall, repair, migration, or shared release switch rebuilds an empty upper, reapplies package then private content, and discards undeclared runtime mutations.
- Files that must survive full reconciliation must be represented in the private workspace. Runtime-only logs are disposable.

The helper handles whiteout/opaque-directory cleanup. Existing content services retain ownership of manifests and rollback journals.

## Global Game Update

Game update is a singleton global Job with no instance target. Instance lifecycle/content operations take a shared maintenance lease; game publication and migration take an exclusive lease. No instance can start or mutate content between the final player check and publication.

Execution order:

1. List all dependent instances and evaluate the online policy.
2. Acquire the exclusive gate and repeat player evaluation.
3. Record active instances and stop them.
4. Install app `222860` into `game/staging/<job-id>` with existing cache, credential, retry, and log behavior.
5. Validate and rename staging to an immutable release.
6. Rebuild every instance upper from its selected package and private workspace.
7. Remount each instance against the new release.
8. Atomically switch `game/current` and persist active release state.
9. Restart only instances whose latest desired state remains running.

Publication failure keeps the old release active. Partial remount failure rolls affected instances back before releasing the exclusive gate. Retain old releases until no mount references them and at least one previous known-good release remains.

## Aggregate Online Policy

`game_update` has no `instance_id`. Its policy applies to all dependent instances:

- `force`: proceed without player checks.
- `skip`: proceed only when every query succeeds and every active server has zero players; otherwise skip the whole update.
- `wait`: repeat all queries until every query succeeds and every active server has zero players.

Stopped, uninstalled, faulted, and orphaned instances need no A2S query. Query failure never counts as empty. Job events identify each blocking instance and distinguish players from query failure.

## API And User Experience

- Add global `GET /api/game` and `POST /api/game/update` endpoints.
- Remove game reinstall selection from the overview instance update dialog; the action becomes package reinstall only.
- Keep `/api/instances/{id}/game-update` for one compatibility release, reject `reinstall_game:true`, and accept package-only requests.
- Hide the instance selector for scheduled `game_update`, submit an empty `instance_id`, and describe its all-server online policy.
- Migrate stored `game_update` tasks to `instance_id=''`; disable duplicates for administrator review instead of launching repeated global updates.
- Global game Jobs use an empty instance ID and remain visible in existing Job/log views.

## Initial Migration

Migration is explicit and resumable:

1. Preflight OverlayFS/free space and require every instance stopped.
2. Install a fresh validated shared release instead of choosing an instance copy as canonical.
3. Rename each legacy `instances/<id>/game` to `legacy-game.<migration-id>`.
4. Create/mount overlays and fully reapply selected package and private workspace.
5. Verify `srcds_run`, manifests, mount identity, and an instance start/stop smoke test.
6. Mark complete only after every instance passes.

Before completion, failure unmounts overlays and restores renamed legacy directories. Keep legacy directories for a grace period and remove them only through explicit verified cleanup.

## Compatibility And Retirement

- Preserve package selection and hot/full actions, private APIs, shared VPK, Job JSON/SSE, Steam credentials, and desired-state behavior.
- Retire per-instance SteamCMD install/update and `instances/<id>/game` as a persistent game copy.
- Retire `game_update.instance_id` as meaningful while retaining the generic database column.
- Never grant `CAP_SYS_ADMIN` to Panel or game containers.
- Production requires Linux OverlayFS and shared mount propagation; Windows uses a fake mount manager for tests only.

## Verification

- Instances share one resolved lower release but have distinct upper/work directories.
- Runtime mutations do not leak between instances.
- Hot applies repair managed paths and preserve unrelated runtime files.
- Full reconciliation discards undeclared mutations and restores package/private precedence.
- One online or unqueryable active instance blocks global `skip`/`wait`.
- No start crosses final validation and publication.
- Staging, remount, restart, and process-restart failures recover to a known-good release.
- Legacy schedules migrate once and UI no longer implies per-instance game updates.
