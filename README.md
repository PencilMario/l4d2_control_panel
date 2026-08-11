# L4D2 Control Panel

面向单台 Linux 主机、单管理员的 Left 4 Dead 2 专用服务器控制面板。Go 服务负责实例状态、SQLite 数据、后台任务、内容部署、A2S 查询和原生控制台连接，React 页面提供实例、玩家、文件、更新、日志与计划任务管理。

项目默认采用受限 Docker Socket 代理，不会把 `/var/run/docker.sock` 挂载到 Panel；游戏实例以非特权用户运行，并使用持久化目录保存游戏文件和管理数据。

## 页面预览

> 截图来自实际部署环境，实例名、节点、端口、插件包和版本标识已替换为演示数据。

![服务器总览与性能监控](docs/images/overview.png)

![共享游戏本体、VPK 与插件包管理](docs/images/content-repository.png)

## 核心能力

- **实例管理**：创建、配置、启动、停止、更新和永久删除多个 L4D2 游戏实例。
- **实时观测**：查看实际状态、地图、玩家数、CPU、内存、网络、磁盘、进程数、运行时间和 A2S 延迟。
- **性能历史**：每 5 秒采样一次，内存中保留最近约 1 小时的性能曲线；Panel 重启后重新开始记录。
- **内容仓库**：管理共享游戏本体、共享 VPK、插件包和每实例私有覆盖层。
- **文件与控制台**：编辑私有文件、导入/导出 ZIP、查看快照，并连接原生 SRCDS 控制台。
- **玩家操作**：查看对局摘要和在线玩家，可执行踢出与永久封禁。
- **任务与日志**：后台任务持久化、SSE 实时进度、完整任务日志以及游戏日志浏览和下载。
- **计划维护**：使用 Cron 安排游戏更新、插件更新、重启和日志清理，并配置在线玩家处理策略。

## 环境要求

- Linux x86-64 主机。
- Debian 或 Ubuntu 可由部署脚本自动安装 Docker Engine 和 Compose 插件；其他发行版需预先安装。
- 未安装实例首次启动前至少保留 1 GiB 可用空间；共享游戏本体单独管理。
- 生产环境需要同主机 TLS 反向代理。
- 本地开发需要 Go 1.24+ 和 Node.js 22+。

## 一键部署

在全新的 Debian 或 Ubuntu 主机执行：

```sh
curl -fsSL https://raw.githubusercontent.com/PencilMario/l4d2_control_panel/main/deploy.sh | sudo bash
```

部署脚本会：

1. 安装缺失的 Docker 组件；
2. 克隆仓库到 `/opt/l4d2-control-panel`；
3. 创建权限为 `0600` 的 `.env`；
4. 生成随机管理员密码；
5. 构建并启动服务；
6. 等待 `/api/health` 就绪。

首次成功后请立即保存终端打印的管理员密码。持久数据默认位于 `/srv/l4d2-panel`。

再次执行同一命令即可更新。脚本会保留 `.env`、命名卷和数据目录；检测到仓库存在本地修改时会拒绝覆盖，只允许快进 `main`，新版本部署失败时会尝试恢复上一提交和服务版本。

也可以指定仓库、分支和安装目录：

```sh
curl -fsSL https://raw.githubusercontent.com/PencilMario/l4d2_control_panel/main/deploy.sh \
  | sudo bash -s -- --repo https://github.com/PencilMario/l4d2_control_panel.git \
      --branch main --install-dir /opt/l4d2-control-panel
```

## 手动部署

```sh
cp .env.example .env
# 设置高强度管理员密码，并按下文确认 L4D2_PANEL_GAME_HOST。
docker compose --env-file .env config --quiet
docker compose --env-file .env --profile images build runtime-image
docker compose --env-file .env up -d --build
```

Panel 使用 host network，直接监听 `0.0.0.0:${L4D2_PANEL_HTTP_PORT:-18081}`，不使用 Docker `ports` 映射。受限 Docker 代理只通过命名卷中的 `/run/l4d2-panel/proxy.sock` 与 Panel 通信，不开放 TCP 监听。

## 必要配置

主要配置位于部署目录的 `.env`：

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `L4D2_PANEL_ADMIN_PASSWORD` | 管理员密码，首次启动必须提供 | 无 |
| `L4D2_PANEL_DATA_ROOT` | Panel 与游戏实例持久数据根目录 | `/srv/l4d2-panel` |
| `L4D2_PANEL_HTTP_PORT` | 宿主机 HTTP 端口 | `18081` |
| `L4D2_PANEL_ACCELERATOR_PORT` | 写入 Accelerator 的宿主机 loopback 上传端口，默认跟随 HTTP 端口 | `L4D2_PANEL_HTTP_PORT` |
| `L4D2_PANEL_GAME_HOST` | Panel 发起 A2S 查询时使用的宿主机地址 | `host.docker.internal` |
| `L4D2_PANEL_CRASH_REPORT_TOKEN` | Accelerator 接收端共享 token；为空时关闭接收端 | 空 |
| `L4D2_PANEL_CRASH_RETENTION_DAYS` | dump 与 metadata 保留天数，范围 `1..3650` | `90` |
| `L4D2_PANEL_STACKWALK_PATH` | Panel 容器内 `minidump_stackwalk` 可执行文件路径 | `/usr/local/bin/minidump_stackwalk` |
| `L4D2_PANEL_DOWNLOAD_PROXY` | GitHub Release、SteamCMD 等下载代理 | 空 |
| `L4D2_PANEL_SECURE_COOKIE` | 是否只通过 HTTPS 发送会话 Cookie | `true` |

`L4D2_PANEL_GAME_HOST` 是必填项。使用仓库提供的 Compose 配置时可保留 `host.docker.internal`，也可以填写 SRCDS 实际响应的宿主机地址。Panel 与游戏服务使用 host network；不要改成 `127.0.0.1`，除非 SRCDS 确实在该回环地址响应 A2S 数据。

如需代理下载，在 `.env` 中设置 `L4D2_PANEL_DOWNLOAD_PROXY`。该值会同时作为 `HTTP_PROXY` 和 `HTTPS_PROXY` 传入 Panel 与 SteamCMD 维护容器；仅在确有额外内网地址时覆盖 `L4D2_PANEL_NO_PROXY`。

如需仅加速 GitHub Release 文件下载，可在“系统设置 > GitHub Release 下载”中填写 HTTPS 加速地址；默认留空并直连 GitHub。GitHub API 查询仍保持直连；加速器只接收公开 Release 文件 URL，GitHub token 不会转发给加速器。

Accelerator 下载设置没有保存记录时，默认使用官方 Linux 包 URL：`https://builds.limetech.io/files/accelerator-2.6.0-git165-dcf3449-linux.zip`。可以在“系统设置 > Accelerator 下载”中替换为自建或 fork 的 HTTPS 包地址；显式保存空地址会关闭自动安装。

## Accelerator 崩溃报告接收与分析

Panel 内置完整的 Accelerator 兼容接收端，不依赖 Throttle。必须设置 `L4D2_PANEL_CRASH_REPORT_TOKEN`；未设置时公开接收路径返回 `503`。Accelerator URL 使用 query token，并且必须通过宿主机 loopback 访问：

```text
http://127.0.0.1:${L4D2_PANEL_ACCELERATOR_PORT:-${L4D2_PANEL_HTTP_PORT:-18081}}/submit?token=<L4D2_PANEL_CRASH_REPORT_TOKEN>
http://127.0.0.1:${L4D2_PANEL_ACCELERATOR_PORT:-${L4D2_PANEL_HTTP_PORT:-18081}}/symbols/submit?token=<L4D2_PANEL_CRASH_REPORT_TOKEN>
http://127.0.0.1:${L4D2_PANEL_ACCELERATOR_PORT:-${L4D2_PANEL_HTTP_PORT:-18081}}/binary/submit?token=<L4D2_PANEL_CRASH_REPORT_TOKEN>
```

接收端只接受 `127.0.0.0/8` 或 `::1` 的来源，不信任 `X-Forwarded-For`。Compose 中 Panel 与游戏实例使用 host network，因此 Accelerator 应配置为 `127.0.0.1`；直接通过宿主机其他地址或公网地址上传会被拒绝。反向代理只有在它连接 Panel 时仍使用 loopback 才能通过来源校验。

三个端点都可用：

- `/submit` 接收预提交、minidump 和 metadata。
- `/symbols/submit` 接收 Breakpad symbol 文件。
- `/binary/submit` 接收 Accelerator 的 `code_file` 模块二进制；内容会经过大小限制、SHA-256 内容寻址和报告模块关联校验，不会把调用方文件名当作服务器路径，也不是任意文件上传接口。

Accelerator 安装后，Panel 会把三个上传 URL、预提交和 symbol/binary 上传开关写入 SourceMod `core.cfg`，其中 `MinidumpBinaryUpload` 必须为 `yes` 才能保持完整协议兼容：

```text
MinidumpUrl          http://127.0.0.1:${L4D2_PANEL_ACCELERATOR_PORT:-${L4D2_PANEL_HTTP_PORT:-18081}}/submit?token=<token>
MinidumpSymbolUrl    http://127.0.0.1:${L4D2_PANEL_ACCELERATOR_PORT:-${L4D2_PANEL_HTTP_PORT:-18081}}/symbols/submit?token=<token>
MinidumpBinaryUrl    http://127.0.0.1:${L4D2_PANEL_ACCELERATOR_PORT:-${L4D2_PANEL_HTTP_PORT:-18081}}/binary/submit?token=<token>
MinidumpPresubmit    yes
MinidumpSymbolUpload 3
MinidumpBinaryUpload yes
```

每次上传还必须匹配本项目已登记实例的 Accelerator `ServerID`。Panel 会查询 managed instances，并校验对应文件：

```text
<data-root>/instances/<instance-id>/game/left4dead2/addons/sourcemod/data/dumps/server-id.txt
```

文件内容必须与请求的 `ServerID` 相同，`GameDirectory` 必须为空或 `left4dead2`。因此仅持有 token、伪造 `ServerID` 或使用其他项目的游戏实例都不能上传。

报告存储在 `panel/crash-dumps/`，包括 minidump、原始 metadata、模块二进制、pending token 和受控 symbol 文件；默认保留 90 天，过期报告由 Panel 启动时及每天清理一次。报告和其派生的 stackwalk/AI 文件遵循同一保留策略，内置 SourceMod/Metamod 符号不会随报告清理。metadata 可能包含命令行、路径或服务器环境信息，只能通过已登录的管理员 API 查看：

```text
GET /api/crash-reports
GET /api/crash-reports/<crash-id>
GET /api/crash-reports/<crash-id>/download?file=minidump
GET /api/crash-reports/<crash-id>/download?file=metadata
GET /api/crash-reports/<crash-id>/download?file=stackwalk
GET /api/crash-reports/<crash-id>/download?file=ai
GET /api/crash-reports/<crash-id>/download?file=binary&artifact=<artifact-id>
POST /api/crash-reports/<crash-id>/analyze
```

Panel 的标准镜像在构建阶段从可配置的 Breakpad 仓库构建并内置 `minidump_stackwalk`，默认路径由 `L4D2_PANEL_STACKWALK_PATH` 指定；自定义镜像或该变量可以提供其它绝对路径。工具缺失、符号不足或 stackwalk 失败只会把分析状态标为失败，不会使 Accelerator 上传失败。仓库只内置 SourceMod 和 Metamod 符号，不分发 Valve/L4D2 游戏符号。

在实例设置中可分别开启“安装 Accelerator”和“自动分析崩溃”。开启后，首次部署、插件重装/更新、游戏重装和容器重建都会重新对账并安装 Accelerator 文件；下载地址以及是否使用 GitHub Release 加速链接在“系统设置 > Accelerator 下载”中配置，不锁定目标仓库或版本。AI 在“系统设置 > 崩溃 AI”中配置 OpenAI-compatible endpoint、model 和 API key；密钥加密保存，发送前只保留脱敏后的崩溃签名、metadata 摘要和 stackwalk 文本。

URL 中包含共享 token，生产环境应使用 HTTPS 或受信任的本机网络，并避免把 token 写入公共日志。

## HTTPS 与安全边界

默认会话 Cookie 使用 `Secure`、`HttpOnly` 和 `SameSite=Strict`，正常使用时应通过 HTTPS 访问。例如使用 Caddy：

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:18081
}
```

仅在可信网络中直接使用 HTTP 时，才把 `L4D2_PANEL_SECURE_COOKIE` 设置为 `false`。部署脚本不会配置 TLS、防火墙、DNS 或反向代理。

安全模型要点：

- Panel 不挂载 Docker Engine Socket。
- 仓库自带的 Socket 代理只暴露带指定标签的游戏/维护容器所需 API 路径。
- 代理通过权限为 `0660` 的 Unix Socket 提供能力，Panel 只接收这个受限文件系统入口。
- 游戏实例使用宿主机网络，但以 UID/GID `10001`、非特权模式运行。
- 游戏、私有覆盖层、备份、控制台和日志目录均持久化；共享内容按只读或受控流程挂载。

## A2S 攻击防御

管理员可在“系统设置 > A2S 攻击防御”中启用固定策略的 IPv4 查询限速。启用后，Panel 会自动保护全部实例的游戏端口和非零 SourceTV 端口；插件端口不在保护范围内。新增或修改实例端口后会自动对账，防御已启用但新端口尚未被 Helper 确认时，该实例不会启动。已经运行的实例不会因 Helper 暂时不可用而被自动停止。

只有 `a2s-defense-helper` 容器拥有 `NET_ADMIN`，Panel 和游戏容器仍无防火墙权限。Helper 使用宿主网络命名空间，但只管理 `L4D2_A2S_*` IPv4 链和 INPUT 中指向项目链的一条跳转，不修改 INPUT 默认策略，也不清空管理员、UFW、firewalld 或 Docker 的规则。

设置页会分别显示期望开关与实际生效状态、受保护端口、策略版本、各查询类型丢弃计数、黑名单数量、最近应用时间和对账错误。显示“不兼容”时，应检查宿主 iptables 1.8+ 及 `u32`、`hashlimit`、`recent`、`multiport` 扩展；Helper 不会自动安装软件或加载模块。

正常停用应先在设置页关闭开关并确认实际状态为“已停用”。仅删除或停止 Helper 容器不会删除内核中已生效的规则。若 Panel/Helper 已不可用，可在宿主机执行以下窄范围紧急清理；命令只处理项目链，不要使用 `iptables -F INPUT`：

```sh
sudo iptables -w 5 -D INPUT -j L4D2_A2S_DEFENSE 2>/dev/null || true
for chain in L4D2_A2S_DEFENSE L4D2_A2S_SLOT_A L4D2_A2S_CLASS_A L4D2_A2S_DROP_A \
             L4D2_A2S_SLOT_B L4D2_A2S_CLASS_B L4D2_A2S_DROP_B; do
  sudo iptables -w 5 -F "$chain" 2>/dev/null || true
done
for chain in L4D2_A2S_DROP_B L4D2_A2S_CLASS_B L4D2_A2S_SLOT_B \
             L4D2_A2S_DROP_A L4D2_A2S_CLASS_A L4D2_A2S_SLOT_A L4D2_A2S_DEFENSE; do
  sudo iptables -w 5 -X "$chain" 2>/dev/null || true
done
```

该策略用于缓解 A2S 查询洪水，不是上游 DDoS 清洗。它不能阻止链路被打满，也不能可靠识别伪造源地址；项目不支持 IPv6，因此不会创建或修改任何 IPv6 规则。

## 首次使用

1. 登录 Panel，进入“内容仓库”。
2. 上传至少一个 ZIP 插件包。
3. 初始化或更新共享游戏本体。
4. 创建游戏实例，选择插件包并设置端口、地图、模式、Tickrate 与玩家上限。

共享 VPK 支持一次选择多个文件。确认弹窗会列出文件名、大小和处理方式，默认在浏览器本地通过 Go WebAssembly 清理无扩展名、`.vtf`、`.mp3`、`.wav`、`.vmf`、`.vmx` 资源后再上传，也可逐项改为直接上传。待上传文件和分片进度保存在浏览器 IndexedDB 中，刷新页面后会自动从服务端确认 offset 并继续。
5. 启动实例，确认 A2S、玩家、性能、控制台和日志均正常。

当前 L4D2 Steam 内容在空 Linux 目录直接安装时可能返回 `Missing configuration`。Panel 的首次安装流程会先使用 Windows 平台内容完成引导，再切回 Linux 完成 App 222860 安装；后续更新和完整性检查使用 Linux SteamCMD 与 `validate`。

内容覆盖优先级为：

```text
插件包 < 共享 VPK < 实例私有覆盖层
```

实例配置或插件包变更会进入串行后台任务。更新实例时可以分别选择重装游戏文件、重新部署当前插件包，或同时执行两项操作。

## 常用运维

```sh
cd /opt/l4d2-control-panel

# 查看服务状态
sudo docker compose --env-file .env ps

# 跟踪核心服务日志
sudo docker compose --env-file .env logs --tail=100 -f \
  panel socket-proxy overlay-helper a2s-defense-helper

# 健康检查
curl --fail http://127.0.0.1:18081/api/health

# 手动执行已安装副本的更新
sudo bash ./deploy.sh
```

部署后至少确认：

```sh
docker compose --env-file .env ps
curl --fail http://127.0.0.1:${L4D2_PANEL_HTTP_PORT:-18081}/api/health
docker compose exec panel test -S /run/l4d2-panel/proxy.sock
docker compose exec panel test ! -e /var/run/docker.sock
docker compose exec panel test -S /run/l4d2-panel-a2s-defense/a2s-defense.sock
```

## 持久数据

默认数据根目录为 `/srv/l4d2-panel`：

```text
panel/panel.db
packages/uploads/
packages/releases/
instances/<id>/game/
instances/<id>/private/
instances/<id>/backups/
instances/<id>/console/
instances/<id>/logs/game/
instances/<id>/logs/sourcemod/
panel/crash-dumps/
shared-vpk/
```

重建或删除游戏容器不会自动删除这些目录。只有在永久删除实例时明确确认删除数据，相关实例目录才会被移除。

游戏日志默认保留 14 天，可在“系统设置 > 游戏日志”调整为 1 至 365 天；私有文件应用快照默认尽力保留最近 20 份。性能历史仅保存在内存中，不属于长期监控或审计数据。

## 本地开发

```sh
# Go 服务
go test -count=1 ./...
go vet ./...

# React 前端
cd web
npm ci
npm test -- --run
npm run build
npm run e2e
cd ..

# Compose 配置
docker compose --env-file .env.example config --quiet
```

Playwright 会在 `127.0.0.1:18082` 启动带 `e2e` 构建标签的 Go fixture，通过真实 HTTP、SQLite、任务、SSE 和 WebSocket 覆盖主要管理流程，同时替换 Docker、SRCDS、A2S、Steam 与 GitHub 等外部边界。该 fixture 不会进入生产构建。

Windows 上如因杀毒软件或文件索引临时锁定 Go 测试可执行文件，可为 `GOTMPDIR` 设置独立临时目录并使用 `go test -p 1` 串行执行；不要因此放宽产品代码约束。

## 设计与实现文档

- [项目文档索引](docs/aegis/INDEX.md)
- [总体设计](docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md)
- [总体实施计划](docs/aegis/plans/2026-07-14-l4d2-control-panel.md)
- [总体验证证据](docs/aegis/work/2026-07-14-l4d2-control-panel/50-evidence.md)

具体功能的设计、计划与验证记录位于 `docs/aegis/`。
