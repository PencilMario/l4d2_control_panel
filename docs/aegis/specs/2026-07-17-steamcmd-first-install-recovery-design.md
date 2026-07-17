# SteamCMD 首装恢复设计

- 状态：已批准
- 日期：2026-07-17
- 范围：匿名安装 App 222860 的首次安装路径

## 1. 问题与目标

安可服务器上的第一个游戏实例通过既有的 Windows 引导、Linux 收尾流程完成安装。第二个实例在空游戏目录中执行相同 Windows 引导时，SteamCMD 五次返回 `ERROR! Failed to install app '222860' (Missing configuration)`。失败目录没有 `appmanifest_222860.acf`，而已有实例凭本地 manifest 可以继续更新和校验。

当前维护容器是一次性的，只持久化实例游戏目录。SteamCMD 位于 `/home/steam/Steam` 的 appinfo、client config 和下载站点元数据随容器删除，导致每个新实例都完全依赖 Valve 当次返回完整的 App 222860 配置。

本设计让匿名首装在临时配置缺失时可恢复，同时保持游戏实例的数据所有权和更新语义不变。

## 2. 约束

- 继续使用既有匿名首装顺序：Windows 平台安装 App 222860，再切换 Linux 完成首装。
- 每个实例继续独立持有 `instances/<id>/game/` 和自己的 `appmanifest_222860.acf`。
- 普通游戏更新和显式完整性校验继续使用 Linux-only `validate`，不进入首装恢复逻辑。
- 不复制第一个实例的游戏文件、manifest、插件或私有覆盖层。
- 不持久化 Steam 登录密码、Steam Guard 输入或任务日志中的秘密。
- 不新增数据库状态或前端配置。

## 3. 数据布局与所有权

Panel 数据根目录新增全局 SteamCMD 元数据目录：

```text
<data-root>/panel/steamcmd/
`-- Steam/              # 挂载到维护容器 /home/steam/Steam
```

该目录由 Panel 管理用户创建，并以读写方式挂载到所有 SteamCMD 维护容器。它只缓存 Steam 客户端的 appinfo、client config 和内容站点元数据，不替代实例级 `game/` 安装目录。

维护容器仍使用镜像内的 `/home/steam/steamcmd/steamcmd.sh` 作为固定入口。缓存目录不得改变容器角色标签、网络模式、安全选项或实例游戏目录绑定。

## 4. 首装恢复流程

匿名 `InstallGame` 最多执行三次独立尝试：

1. 创建维护容器并运行既有 Windows 引导、Linux 收尾命令。
2. 捕获完整 SteamCMD 输出，并保持现有实时任务日志行为。
3. 退出码为零时立即成功。
4. 非零退出且输出不包含 `Failed to install app '222860' (Missing configuration)` 时立即返回原始失败。
5. 明确匹配该错误且仍有剩余次数时，记录一条结构化警告，删除本次维护容器，保留共享 SteamCMD 元数据和实例目录，然后创建新容器重试。
6. 第三次仍失败时返回包含尝试次数和 SteamCMD 退出码的错误；任务日志保留每次尝试的原始输出。

重试不使用任意字符串模糊匹配，不处理磁盘不足、认证失败、下载失败、容器 API 错误或上下文取消。上下文取消立即停止，现有维护写入器恢复规则继续生效。

## 5. 组件变更

`internal/docker` 是该行为的唯一所有者：

- `runMaintenance` 返回退出分类所需的受限结果，而不是只返回格式化后的退出码错误。
- 日志采集同时写入现有 Job 日志，并在内存中保留有界的末尾输出用于错误分类。
- `InstallGame` 负责仅针对匿名首装执行最多三次恢复尝试。
- 维护容器创建逻辑增加全局 SteamCMD 元数据绑定，并确保宿主机目录存在。

`internal/provisioning`、HTTP API、数据库和 Web UI 不感知尝试次数，继续把安装视为一个 Job。

## 6. 错误与诊断

每次恢复尝试在任务日志中记录当前次数，例如：

```text
SteamCMD configuration missing during first install; retrying attempt 2 of 3
```

最终失败保留 SteamCMD 原始输出，并在任务错误中说明三次尝试均耗尽。日志分类缓冲必须有固定上限，不能因 SteamCMD 的长下载输出无限增长。

缓存目录创建、权限或绑定失败属于 Panel/Docker 配置错误，不进行 SteamCMD 重试。

## 7. 测试与验收

自动化回归必须先失败后实现，并覆盖：

- 第一次返回目标 `Missing configuration`、第二次成功时，创建两个维护容器并完成安装。
- 非目标 SteamCMD 失败不重试。
- 三次目标错误后停止并返回包含尝试次数的错误。
- 上下文取消或 Docker API 失败不重试。
- 每个维护容器同时挂载实例游戏目录和全局 SteamCMD 元数据目录。
- 普通 `UpdateGame` 保持一次 Linux-only `validate`，不启用首装重试。
- 日志分类缓冲有界，实时日志仍完整写入现有 Job 日志存储。

本地验证包括 `go test -count=1 ./internal/docker`、相关 provisioning/lifecycle 测试、`go test -count=1 ./...`、`go vet ./...` 和 `git diff --check`。

安可服务器验收使用现有失败实例 `7c991725-7345-42ad-a030-61c8e59a7746`：部署新 Panel 后重试其安装，确认生成实例自己的 `appmanifest_222860.acf`、游戏目录达到合理大小、安装 Job 成功并能创建或启动游戏容器。不得创建第三个游戏实例，也不得改动第一个实例的游戏目录。

## 8. 非目标与兼容边界

- 不保证绕过 Valve 长期撤销匿名访问或改变 App 222860 depot 配置。
- 不自动复制完整游戏内容以节省下载。
- 不实现跨主机缓存同步、缓存管理 UI 或缓存清理计划。
- 不改变可选授权 Steam 账号的安装路径；该路径仍按现有行为执行一次 Linux 首装。
- 现有维护容器中断后采用同一写入器的恢复契约保持不变。

## 9. 设计输入与影响

### TaskIntentDraft

目标是修复安可服务器上第二个全新游戏实例无法取得 App 222860 配置的问题，并用真实实例完成验证。主要风险是把外部临时错误误判为可重试、泄露秘密、破坏实例独立性或重复启动并发写入器。

### BaselineReadSetHint

本设计受 `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`、`docs/aegis/specs/2026-07-15-instance-startup-package-design.md`、`internal/docker/client.go`、`internal/docker/client_test.go` 和安可服务器 2026-07-16 的 SteamCMD 任务日志约束。

### ImpactStatementDraft

变更集中在 Docker 维护容器的目录绑定、退出分类和匿名首装编排。实例级内容、普通更新、数据库、API 和界面契约不变。旧的一次性 SteamCMD home 行为由全局元数据缓存取代；一次性维护容器和实例级游戏目录所有权继续保留。
