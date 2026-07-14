# L4D2 Control Panel 设计规格

- 状态：已批准
- 日期：2026-07-14
- 第一版部署范围：单台 Linux Docker 主机
- 管理模型：单管理员，一个 Panel 管理多个 L4D2 游戏容器

## 1. 目标

构建一个 Go 后端与 React 前端组成的 Web Panel。管理员可从网页创建和管理多个 Left 4 Dead 2 Dedicated Server 实例，并完成：

- SteamCMD 安装、校验和更新 App 222860。
- 游戏实例启动、停止、重启、状态和资源查看。
- 直接操作 SRCDS 原生控制台，而非使用 RCON。
- 上传插件包或从 GitHub Releases 获取插件包。
- 插件热更新、完整更新、版本记录和失败回滚。
- 管理实例私有覆盖层，取代现有 `custom_config.sh` 的复制效果。
- 管理所有实例共用的大型地图及 Mod VPK 文件。
- 查询在线玩家并执行踢出、封禁。
- 通过 Panel 内置 Cron 调度游戏和插件更新任务。

第一版只管理同一宿主机上的容器，但数据契约保留 `node_id`，以便未来引入远程 Agent。

## 2. 非目标

第一版不包含：

- 多用户、角色或细粒度权限。
- 跨宿主机节点管理。
- RCON 控制通道。
- 任意宿主机或容器 Shell。
- Git 工作区形式的插件更新。
- 在线创意工坊浏览与 VPK 内部地图分析。
- 跨实例移动玩家、预约槽位或完整历史玩家系统。
- 自动管理宿主机防火墙。

## 3. 总体架构

```text
Browser
  | HTTPS / WebSocket / SSE
  v
Panel container (Go API + React assets)
  |-- SQLite WAL, jobs, audit
  |-- package/private/VPK file management
  |-- A2S query
  |-- console WebSocket proxy
  `-- restricted Docker API
          |
          v
Docker Socket Proxy
          |
          |-- L4D2 instance A (host network)
          |-- L4D2 instance B (host network)
          `-- L4D2 instance N (host network)
```

Panel 不直接挂载 `/var/run/docker.sock`。Docker Socket Proxy 只开放容器查询、创建、启停、日志、Exec 和必要的镜像 API。游戏容器不加入控制网络，而是统一使用 `network_mode: host`。

### 3.1 宿主机数据布局

```text
/srv/l4d2-panel/
|-- panel/
|   `-- panel.db
|-- packages/
|   |-- uploads/
|   `-- releases/
|-- instances/
|   `-- <instance-id>/
|       |-- game/       # SteamCMD 游戏本体和有效 left4dead2 树
|       |-- private/    # 实例私有覆盖层
|       |-- backups/
|       `-- console/
`-- shared-vpk/         # 所有实例共享的 VPK 源目录
```

游戏目录、配置、插件和数据库均持久化。删除或重建游戏容器不得删除实例数据。

## 4. 游戏运行容器

通用运行镜像 `l4d2-server-runtime` 包含 SteamCMD、L4D2 所需 32 位运行库、解包工具和轻量 Supervisor，不内置特定插件包。游戏以非 root 的 `steam` 用户运行。

容器启动过程：

1. 检查持久化游戏目录。
2. 首次运行时安装 Steam App 222860。
3. 生成共享 VPK 的有效链接。
4. 应用实例私有覆盖层。
5. Supervisor 创建 PTY，以前台方式启动 `srcds_run`。
6. 同时检查进程状态和 A2S 响应。

运行镜像版本与游戏版本独立。升级镜像只重建容器，不修改持久化游戏文件。

### 4.1 Host 网络

所有游戏实例使用 Host 网络，不配置 Docker 端口映射。实例唯一地址为 `宿主机 IP + 游戏端口`。

Panel 管理游戏/A2S 端口、可选 SourceTV 端口和声明的插件监听端口。启动前同时检查其他实例配置与宿主机实际监听状态，发现冲突时拒绝启动。Panel 不修改 iptables 或其他防火墙规则。

## 5. SRCDS 原生控制台

Supervisor 持有 SRCDS 的伪终端。它负责标准输入输出、有限大小的环形日志、退出码、健康状态和受限重启策略。

```text
Browser terminal
  <-> authenticated WebSocket
Panel
  <-> fixed Docker Exec operation
Supervisor attach endpoint
  <-> PTY
SRCDS
```

Supervisor 仅提供固定操作：

```text
l4d2-supervisor attach
l4d2-supervisor status --json
l4d2-supervisor stop
```

Panel 不允许客户端指定任意 Docker Exec 参数。`attach` 只能进入 SRCDS PTY，不能切换到 Shell。浏览器关闭或网络断开不影响游戏进程；重连时回放近期控制台输出。

Web 终端支持实时输出、命令历史、ANSI 显示、搜索、暂停滚动、复制、清屏、连接状态、PID、运行时间和最近退出码。

## 6. 实例生命周期

```text
未安装 -> 安装中 -> 已停止 -> 启动中 -> 运行中
                       ^          |
                       `-- 更新中 / 回滚中 / 故障
```

实例配置至少包括名称、端口、启动地图、游戏模式、Tickrate、最大玩家数、额外 SRCDS 参数、运行镜像、插件包版本和重启策略。修改需要重建容器的配置时，Panel 保留数据卷并重建容器定义。

SRCDS 停止顺序为：控制台 `quit`、Docker 正常停止、宽限期结束后强制终止。崩溃循环超过阈值后停止自动重启并标记故障。

## 7. 插件包与更新

### 7.1 来源

第一版只支持：

- 管理员上传压缩包。
- GitHub Releases 资产，配置仓库与资产匹配规则。

Panel 不在游戏节点维护 Git 仓库。每个包保存版本、归档 SHA-256、文件清单、文件归属和热更新兼容标记。

### 7.2 热更新

热更新不停止 SRCDS，只允许部署以下路径：

```text
addons/sourcemod/configs/
addons/sourcemod/data/
addons/sourcemod/gamedata/
addons/sourcemod/plugins/
addons/sourcemod/translations/
scripts/
cfg/
```

流程为：验证归档、备份将被替换的文件、部署白名单内容、同步共享 VPK、重放私有覆盖层，并在配置允许时执行固定的插件刷新命令。失败时恢复本次替换的文件，不重启正常运行的服务器。

### 7.3 完整更新

完整更新主动停服，不以游戏崩溃作为更新手段。流程为：停止 SRCDS、保存旧版本清单、清理仅由旧插件包拥有的文件、部署完整包、同步共享 VPK、重放私有覆盖层、启动并健康检查。失败时恢复上一插件版本。

### 7.4 游戏更新

游戏更新必须停服，执行 `steamcmd +app_update 222860 validate`，随后重新应用插件包、共享 VPK 与私有覆盖层，再启动并健康检查。Valve 游戏二进制不做自动降级；失败后保留诊断状态并允许重新校验。

## 8. 内容覆盖模型

三类用户内容的固定优先级为：

```text
插件包 < 共享 VPK < 实例私有覆盖层
```

Valve 游戏文件是基础层。共享目录只读挂载到游戏容器的 `/opt/l4d2/shared-vpk/`，不可直接覆盖整个 `left4dead2/addons/`。Panel 或 Supervisor 为共享 VPK 在 `left4dead2/addons/` 生成受控符号链接；私有层存在同名文件时，移除共享链接并写入私有文件。

Panel 记录每个有效文件的来源及被覆盖来源。更新时只删除归属于旧插件包的文件，不删除 Valve 文件、共享源文件或私有文件。

### 8.1 私有覆盖层

`instances/<id>/private/` 是 `custom_config.sh` 功能的正式替代：插件安装或更新后，将私有文件树最后覆盖到 `left4dead2/`。Panel 提供上传、下载、目录树、小型文本编辑、删除、历史版本和立即应用。它不执行插件包或管理员上传的 Shell 脚本。

### 8.2 共享 VPK

Panel 提供分块上传、断点续传、SHA-256、重复检测、下载、重命名和删除。上传先进入临时目录，完成校验后原子移动。游戏容器只读访问共享源目录。替换或删除运行中实例可见的 VPK 需要二次确认，并提示换图或重启才能可靠生效。

## 9. 玩家管理

在线列表通过 UDP A2S 查询获取名称、在线时间、分数、地图和人数。踢出或封禁前，通过 SRCDS 控制台 `status` 将玩家映射到稳定 UserID，避免直接拼接玩家名。

第一版图形界面只提供在线列表、踢出和永久/限时封禁。命令通过原生控制台执行，并调用 `writeid` 持久化；其他玩家操作由管理员在控制台完成。

## 10. 计划任务

Panel 内置 Cron 调度器，不直接修改宿主机 crontab。任务类型包括游戏更新、插件热更新、插件完整更新、Release 检查、备份和清理。

每条任务包含目标实例、Cron 表达式、时区、在线玩家策略和启用状态。同一实例同一时间只允许一个变更任务；多实例自动错峰。Panel 重启后默认跳过错过的执行时间，避免批量停服。手动任务与计划任务使用相同流水线和审计记录。

## 11. 页面结构

- 总览：实例卡片、状态、地图、玩家数、版本、资源、快捷启停和任务进度。
- 实例详情：概览、控制台、玩家、配置、更新、任务、日志。
- 内容管理：插件包、共享 VPK、私有覆盖层、来源与冲突展示。
- 计划任务：Cron 编辑、自然语言预览、上下次时间与执行历史。
- 系统设置：管理员、GitHub Token、目录、端口池、上传/备份限制、审计与系统状态。

所有长操作进入持久化后台 Job。页面使用 WebSocket 或 SSE 接收进度，刷新页面不得丢失任务。

## 12. 数据模型

SQLite 使用 WAL 模式。核心实体为：

- `Instance`：容器、期望状态、端口、启动配置、镜像和插件版本。
- `PackageSource`：上传或 GitHub Release 来源。
- `PackageVersion`：版本、归档、哈希、清单和热更新属性。
- `PrivateFile`：实例私有文件路径、哈希和版本。
- `SharedVpk`：文件名、哈希、大小和链接状态。
- `ScheduledTask`：任务类型、Cron、时区和在线玩家策略。
- `Job`：状态、阶段进度、日志和回滚引用。
- `AuditEvent`：操作、目标、结果、元数据和时间。

容器统一带以下标签，Panel 只操作标签正确的容器：

```text
io.l4d2-panel.managed=true
io.l4d2-panel.instance-id=<uuid>
io.l4d2-panel.role=game
```

## 13. 安全要求

- 单管理员密码使用 Argon2id；会话 Cookie 为 `HttpOnly`、`Secure`、`SameSite=Strict`。
- GitHub Token 等秘密加密保存并在日志中遮罩。
- Panel 文件访问限制在数据根目录，拒绝绝对路径、`..` 和符号链接逃逸。
- 归档部署前检查路径穿越、符号链接、压缩炸弹、单文件限制和磁盘空间。
- 游戏容器不使用 privileged，不挂载 Docker Socket。
- 停服、完整更新、回滚、删除实例和删除 VPK 需要二次确认。
- Panel 不执行任意 Shell，不执行插件包携带脚本，不修改防火墙。

## 14. 故障恢复

- Panel 启动时扫描带管理标签的容器并与数据库对账。
- 容器仍运行时重建控制台连接，不重启游戏。
- 数据库声明运行但容器不存在时标记 `orphaned` 并允许重建。
- 未知受管容器显示为待认领对象，不自动删除。
- Job 使用阶段 journal；Panel 在更新中退出后可继续或回滚。
- 下载、解包前检查空间；空间不足时任务不得开始。
- 保留失败任务输出、控制台尾部、容器日志和更新前后版本。

## 15. 测试策略

- Go 单元测试：路径安全、归档检查、清单、Cron、端口分配和覆盖优先级。
- Supervisor 集成测试：PTY、输入输出、断线重连、日志回放、退出和崩溃循环。
- Docker 集成测试：Host 网络、持久化挂载、实例重建和更新流程。
- 更新测试：热更新不停服、完整更新停服、私有层最终覆盖和失败回滚。
- 文件测试：大型分块上传、续传、重复 VPK、磁盘不足和恶意归档。
- 玩家测试：A2S、`status` 映射、踢出和封禁。
- 浏览器端到端测试：登录、建服、控制台、上传、更新、Cron 和恢复。
- 故障注入：更新中终止 Panel、终止 SRCDS、重启 Docker 和中断下载。

## 16. 分阶段交付

1. 可运行内核：登录、实例创建/安装/启停、Host 端口检查、原生控制台、A2S 和游戏更新。
2. 内容系统：上传/Release 插件包、两类更新、回滚、私有层和共享 VPK。
3. 自动化：Cron、错峰、玩家踢封、审计、崩溃恢复和容器对账。
4. 发布完善：监控、磁盘预警、备份策略、故障注入、文档和镜像发布。

## 17. 第一版验收标准

在一台全新的 Linux Docker 主机上，部署 Panel 与 Docker Socket Proxy 后，管理员可以从网页创建多个 Host 网络 L4D2 实例，完成游戏安装和更新、原生控制台操作、在线玩家查询与踢封、插件热更新和完整更新、私有覆盖、共享 VPK 管理及计划任务。重建 Panel 或游戏容器不得丢失持久化数据，所有更新失败都必须留下可诊断记录并在适用时回滚。

## 18. 设计输入与影响边界

### TaskIntentDraft

目标是以通用容器模板管理 L4D2 游戏本体及任意上传/Release 插件包，同时兼容现有项目的热更新范围和私有文件覆盖需求。主要风险为 Docker 管理权限、原生控制台隔离、Host 网络端口冲突、大文件上传与更新一致性。

### BaselineReadSetHint

设计参考了 `L4D2-Not0721Here-CoopSvPlugins` 的 `Docs/install.md`、`Docs/srcds1`、`update_from_release.sh`、`update_full_from_release.sh`、`custom_config.sh`、`cfg/server.cfg` 及 Release 工作流。后续实现仍需以 Docker Engine API、SteamCMD/App 222860 行为和 Source A2S 协议的官方/稳定契约为准。

### ImpactStatementDraft

实现会新增 Panel Web/API、容器 Supervisor、运行镜像、Docker 编排、持久化数据模型和内容部署流水线。兼容边界是保留插件包对 `left4dead2/` 的覆盖结构以及现有 `custom_config.sh` 的最终复制语义；不兼容任意 Shell hook、直接 Git 工作区更新或 RCON 控制。
