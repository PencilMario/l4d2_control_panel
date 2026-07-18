# 共享游戏本体首次初始化与原地校验设计

## 目标

当系统还没有共享游戏本体时，创建第一个游戏实例会自动排队一次共享本体初始化。首次初始化使用 SteamCMD 普通安装，不带 `validate`。初始化完成后，实例只能通过 OverlayFS 使用共享本体，不能回退到实例独立下载。

后续全局游戏更新不创建新的 release：在在线玩家策略满足后停止受影响实例，卸载 Overlay，对当前 active release 原地执行 SteamCMD `+app_update 222860 validate`，然后重建每个实例的 upper 层并重新应用插件包和私有文件，最后恢复原本期望运行的实例。更新成功后不保留旧 release，`previous_release_id` 为空。

## 用户流程

- 创建第一个实例仍立即返回实例资源；后台生成 `shared_game_migration` Job。
- 共享本体未处于 `ready` 状态时，实例启动返回明确的“共享游戏本体正在初始化或不可用”错误，不执行实例独立安装。
- 初始化失败不会删除实例；管理员可重试共享迁移 Job。
- 后续创建实例不触发 SteamCMD 下载，只创建实例配置，首次启动使用现有共享本体。
- 全局游戏更新保持现有确认和在线策略接口，目标改为 active release 的原地 validate。

## 并发与状态

- 创建实例检查共享状态和实例数量必须在共享维护 Gate 下完成，避免并发创建重复排队初始化。
- 已有 active release 且状态为 `ready` 时不排队初始化。
- 已有初始化/更新 Job 时不重复排队；共享状态 `installing`、`updating` 或已有未完成全局 Job 视为进行中。
- 原地 validate 失败将共享状态标记为 `failed`，不自动启动实例；不提供旧 release 回滚。
- 更新过程中的 `previous_release_id` 保持为空，成功后仍只保留 active release 目录。

## 兼容边界

- 首次初始化继续复用 `SharedGameService.Migrate`，保持首次安装不带 validate。
- 插件包和私有文件仍由每个实例的 upper 层管理；更新后必须重新应用。
- 现有 `/api/game/migrate` 确认接口和全局游戏更新接口保持兼容。
- 旧的实例独立 `InstallGame` fallback 退役为共享本体未 ready 时的明确错误，不再允许创建重复游戏本体。

## 风险与非目标

- 原地 validate 失败可能留下部分已校验内容；由于明确不保留旧 release，恢复方式是重新执行 validate 或人工修复。
- 本次不实现旧 release 回滚、不改变 OverlayFS 的 lower/upper 目录模型，也不改变插件包/私有文件的实例隔离。

## 验证

- HTTP 测试证明无共享本体时创建第一个实例只创建一个初始化 Job。
- 并发/重复创建测试证明不会生成重复初始化 Job。
- 生命周期测试证明共享本体未 ready 时不会调用实例独立安装。
- 更新测试证明满足在线策略后停止、原地 validate、重建 upper、重应用内容并恢复运行实例，且没有新 release/旧 release 保留。
- 运行 `go test ./... -count=1`、`go vet ./...` 和部署 smoke 测试。
