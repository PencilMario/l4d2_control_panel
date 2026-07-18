# 一键部署与更新脚本实施计划

## 范围与基线

本计划实现已确认的 `docs/aegis/specs/2026-07-18-one-click-deploy-update-design.md`。当前部署契约由 `README.md`、`docker-compose.yml`、`.env.example` 和 `Makefile` 定义：生产环境是 Linux x86-64 + Docker Compose，Compose 通过 `.env` 注入管理员密码、数据目录、HTTP 端口和游戏主机地址。

## 文件责任

- `deploy.sh`：远程可执行的引导、依赖检查、仓库获取、配置生成、Compose 部署、健康检查、更新和回退。
- `deploy_test.sh`：隔离临时目录与命令替身，覆盖脚本的可观察行为，不接触真实 Docker 主机。
- `README.md`：一行首次部署、重复更新、状态/日志和手动 Compose 流程。
- `Makefile`：提供脚本语法与行为测试入口（若现有 Make 约定适合）。
- `docs/aegis/work/2026-07-18-one-click-deploy-update/40-atomic-tasks.md`：记录按测试先行拆分的原子任务。

## 实施步骤

### 1. 先写可注入的 Shell 测试

为临时安装目录、Git、Docker Compose、系统识别文件和健康检查注入命令替身。先覆盖参数解析、首次 `.env` 生成与权限、已有配置不覆盖、脏工作树拒绝更新、快进更新和失败回退；运行测试确认这些测试因 `deploy.sh` 不存在或行为缺失而失败。

### 2. 实现部署脚本最小行为

使用 Bash 严格模式和清晰的函数边界实现参数解析、root/架构/命令检查、仓库克隆或复用、配置生成、Compose 配置检查、镜像构建、服务启动和健康检查。默认值必须与 Compose 现有默认值及 README 示例一致，所有路径和外部参数加引号。

### 3. 实现 Debian/Ubuntu Docker 准备

仅在识别到 Debian/Ubuntu 且 Docker 或 Compose 插件缺失时执行官方仓库安装；未知发行版不修改系统。将安装逻辑单独封装，测试通过命令替身验证分支选择与失败退出。

### 4. 实现安全更新与回退

检查工作树干净状态，保存当前提交，执行 fetch 和 fast-forward 更新；构建/启动/健康检查失败时 checkout 原提交并恢复旧版本服务，保留 `.env`、命名卷和数据目录。更新成功或失败都输出明确摘要。

### 5. 更新文档与开发入口

将远程一行命令置于 README 部署章节首位，说明自动生成密码、更新命令、状态/日志命令和手动 Compose 方式。增加 `Makefile` 的脚本验证目标（若已有目标命名冲突则复用现有测试入口）。

### 6. 验证与审查

按改动范围运行 `bash -n deploy.sh`、Shell 行为测试、`docker compose --env-file .env.example config --quiet` 和相关现有测试；若 Linux/Docker 不可用，明确记录未覆盖的真实部署路径，不把替身测试当作主机验收。

## 兼容边界

- 不修改 Compose 服务拓扑、数据卷、容器安全约束或业务 API。
- 不覆盖已有 `.env`，不删除任何 Docker 卷或数据目录。
- 远程入口必须能在空目录执行；仓库内入口必须能在安装目录重复执行。
- 更新只允许快进目标分支，不能静默覆盖本地 tracked 或 untracked 修改。

## 风险与未知

- 当前开发主机为 Windows，无法直接证明 Debian/Ubuntu Docker 安装分支和真实 host-network 行为。
- Docker 官方安装脚本/软件源会随发行版和时间变化；实现只在明确识别的系统上操作并保留失败诊断。
- 远程 `curl | sudo bash` 依赖 GitHub raw 内容可达；README 同时保留本地脚本执行方式。
