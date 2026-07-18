# 一键部署与更新脚本设计

## 目标

为 Linux x86-64 主机提供无需预先克隆仓库的一行部署入口。首次执行自动准备依赖、生成安全配置、构建并启动完整 Compose 服务；再次执行同一入口时安全更新已有安装，同时保留管理员配置和持久数据。

首选使用方式：

```sh
curl -fsSL https://raw.githubusercontent.com/PencilMario/l4d2_control_panel/main/deploy.sh | sudo bash
```

安装完成后也可直接执行：

```sh
sudo bash /opt/l4d2-control-panel/deploy.sh
```

## 默认行为

- 默认仓库为 `https://github.com/PencilMario/l4d2_control_panel.git`，默认分支为 `main`。
- 默认安装目录为 `/opt/l4d2-control-panel`。
- 默认持久数据目录为 `/srv/l4d2-panel`。
- 默认面板端口为 `18081`。
- 首次部署自动生成高强度管理员密码，并在部署成功后打印一次。
- 默认使用 `host.docker.internal` 作为 Panel 查询宿主机 SRCDS 的地址。
- 通过参数 `--repo`、`--branch` 和 `--install-dir` 覆盖仓库、分支和安装目录，便于镜像仓库与自动化测试。

## 首次部署流程

1. 检查脚本运行在 Linux x86-64 主机，并要求 root 权限。
2. 检查 `curl`、`git`、Docker Engine 和 Docker Compose 插件。
3. Debian 或 Ubuntu 缺少 Docker 时，通过 Docker 官方软件源安装 Docker Engine 与 Compose 插件；其他发行版缺少依赖时停止并打印明确安装提示。
4. 当脚本通过标准输入运行且当前目录不是项目仓库时，将目标仓库克隆到安装目录，然后从已克隆副本继续部署。
5. 若 `.env` 不存在，则原子写入自动生成的配置；已有 `.env` 永不覆盖。
6. 运行 Compose 配置校验，构建 `runtime-image`，再构建并启动完整服务。
7. 等待 Compose 健康状态和 `http://127.0.0.1:<端口>/api/health` 成功；超时则输出容器状态与近期日志并返回失败。
8. 成功后打印访问地址、安装目录、更新命令和首次生成的管理员密码。

## 更新流程

- 已有安装必须是目标仓库的 Git 工作树，并配置可用的 `origin`。
- 更新前检查 tracked 文件和未跟踪文件；存在本地修改时拒绝更新，防止覆盖管理员维护内容。`.env` 由 Git 忽略，不影响检查。
- 记录更新前提交，获取远端目标分支，并仅允许将本地分支快进到 `origin/<branch>`。
- 更新成功后重新校验 Compose、构建 `runtime-image`，并执行 `docker compose up -d --build`。
- `.env`、Compose 命名卷和 `/srv/l4d2-panel` 持久数据均不删除、不重建。
- 新版本部署或健康检查失败时，将工作树回退到更新前提交，并使用旧版本 Compose 定义重新构建和启动；脚本仍返回非零状态并说明已尝试恢复。

## 安全与可恢复性

- 脚本启用严格 Shell 模式，对路径和参数始终加引号。
- 不通过命令行参数传递管理员密码，避免进入 Shell 历史和进程列表。
- 管理员密码优先使用 `openssl rand` 生成，缺少 OpenSSL 时使用 `/dev/urandom` 的等强度回退方案。
- `.env` 使用临时文件写入、权限设为 `0600`，再原子替换。
- Docker 安装仅限明确识别的 Debian/Ubuntu；不对未知发行版修改软件源。
- 更新不执行 `docker compose down -v`、不删除镜像、卷、实例目录或共享游戏本体。

## 测试策略

- 将可测试逻辑拆为小型 Shell 函数，并允许通过环境变量或参数注入临时安装目录、仓库和命令替身。
- 行为测试覆盖参数解析、首次配置生成、已有配置保留、脏工作树拒绝更新、快进更新以及失败回退。
- 静态检查优先使用 `shellcheck`；开发环境未安装时至少执行 `bash -n deploy.sh`。
- Compose 回归继续运行 `docker compose --env-file .env.example config --quiet`。
- 文档示例必须与脚本默认值和支持参数一致。

## 文档调整

- README 的部署章节首先展示一行安装命令。
- 说明首次部署会自动生成密码，以及如何从部署输出保存该密码。
- 保留现有手动 Compose 部署步骤，供高级用户和开发场景使用。
- 增加更新、查看状态、查看日志和常见失败恢复命令。

## 非目标

- 不支持 Windows 或 macOS 生产部署。
- 不自动配置域名、TLS、防火墙、端口转发或反向代理。
- 不自动迁移管理员自行修改过的仓库文件。
- 不自动支持 Debian/Ubuntu 之外发行版的软件包安装。
- 不将管理员密码上传到远端或写入项目日志。

## 验收标准

- 干净的受支持主机可通过一行命令完成部署并通过健康检查。
- 重复执行一行命令会更新代码和镜像，而不会改变 `.env` 或持久数据。
- 本地修改、依赖缺失、Compose 构建失败和健康检查失败均有明确非零退出状态与诊断信息。
- 更新失败时脚本尝试恢复更新前提交和服务版本。
- `README.md` 中的一行部署、更新和手动部署说明可直接执行且相互一致。
