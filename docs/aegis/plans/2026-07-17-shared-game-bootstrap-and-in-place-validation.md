# 共享游戏首次初始化与原地校验实施计划

## Scope

实现已批准的共享游戏行为：首个实例创建后自动排队首次初始化；后续全局更新在满足在线玩家策略后对 active release 原地执行 SteamCMD validate，不创建新 release、不保留旧 release；未 ready 时禁止实例独立安装。

## Facts and boundaries

- 首次初始化已有 `SharedGameService.Migrate`，且测试要求首次安装 `validate=false`。
- 当前 `SharedGameCoordinator.Update` 会生成新 release、切换 lower 并写入 `previous_release_id`，这是本次要替换的 owner。
- `SharedGameRebuilder.Switch` 已能卸载/重建 upper 并重应用插件包与私有文件。
- HTTP 创建实例当前同步写库并返回 201；保持该接口响应兼容，初始化 Job 作为后台副作用公开在任务列表中。
- 不实现旧 release 回滚；失败状态必须可见且不自动启动实例。

## Files and responsibilities

### Task 1: Lock bootstrap creation behavior

**Files:** `internal/httpapi/server.go`, `internal/httpapi/server_test.go`, `internal/store/store.go` only if an atomic shared-state read helper is required.

Add a shared bootstrap dependency that can inspect `SharedGameState`, count instances, and enqueue one global `shared_game_migration` Job. In `createInstance`, create the instance, then under the maintenance gate re-check whether this is the first instance and shared state is absent/not ready. Queue exactly one migration operation without blocking the 201 response. Preserve the existing instance JSON response.

Tests must first fail for: first creation queues one migration; a ready shared release does not queue; repeated creation while initialization is pending does not queue another job.

### Task 2: Remove independent-install fallback

**Files:** `internal/provisioning/service.go`, `internal/provisioning/service_test.go`, `internal/lifecycle/service.go`, `internal/lifecycle/service_test.go`.

Make provisioning require a ready shared release when shared services are configured. Return an explicit error when state is missing, installing, or failed. Keep package lookup, overlay link creation, upper reset, full package deployment, private apply, and instance metadata update unchanged for ready releases.

Tests must prove no `InstallGame` call occurs when the shared release is unavailable and existing ready-state provisioning remains green.

### Task 3: Replace release staging with in-place validate

**Files:** `internal/updates/shared_game.go`, `internal/updates/shared_game_test.go`, `internal/docker/client.go`, related Docker tests.

Keep the existing player wait and exclusive maintenance gate. After stopping affected running instances, resolve `state.ActiveReleaseID`, unmount/rebuild each instance through the existing reconciler as needed, and call `InstallSharedGame` with the active release path and `validate=true`. Do not allocate a new UUID, call `StagePath`, `Publish`, or `Activate`, and leave `PreviousReleaseID` empty. On success reapply each instance package/private layer and restart only instances whose desired state was running. On validate failure mark shared state failed and leave instances stopped.

Add tests for validate=true, no publisher calls, empty previous release, player policy enforcement, upper rebuild/reapply, and restart behavior.

### Task 4: Keep release storage single-version

**Files:** `internal/updates/shared_game.go`, `internal/updates/shared_game_test.go`, `internal/updates/shared_reconciler.go` only if an explicit unmount/rebuild method is needed, and documentation.

Ensure successful update leaves `game/current` pointing to the active release and removes any non-active release directories. Do not add rollback retention. Add filesystem-level test for cleanup and state invariants.

### Task 5: Regression and deployment verification

Run focused HTTP, provisioning, lifecycle, migration, and updates tests, then `go test ./... -count=1`, `go vet ./...`, and `git diff --check`. Deploy to a disposable/authorized host only after local verification; verify first-instance Job creation, shared state transitions, no duplicate SteamCMD install, and no old release after update.

## Repair track

- Root cause: creation and provisioning still assume per-instance game ownership, while shared migration/update are separate manual flows.
- Canonical owners: HTTP creation owns bootstrap Job enqueue; provisioning owns the ready-state guard; shared coordinator owns in-place validate/update lifecycle.
- Smallest fix: connect existing migration Job to first creation, remove only the independent-install fallback, and replace release staging in the shared coordinator.

## Retirement track

- The old `SharedGameCoordinator` staging/publish/activate branch retires after tests prove in-place validate semantics.
- `InstallGame` remains only as a legacy interface until all callers and compatibility code are explicitly removed in a later cleanup request; it must not be selected by shared provisioning.
- `previous_release_id` and old-release retention remain unused for this flow and can be removed only in a later schema cleanup.
