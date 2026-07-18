# Baseline Read Set

- `internal/httpapi/server.go`: 创建实例、全局迁移和 Job 排队边界。
- `internal/migration/sharedgame.go`: 首次共享本体安装与实例布局迁移。
- `internal/updates/shared_game.go`: 当前全局更新的 release staging、切换和重启流程。
- `internal/provisioning/service.go`: 当前共享 ready 与实例独立安装 fallback。
- `internal/docker/client.go`: SteamCMD 首次安装和 validate 命令构造。
- `internal/overlayfs`: Overlay 卸载、upper 重建和重新挂载。
