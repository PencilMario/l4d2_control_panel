# 验证证据

## 自动化验证

- `bash -n ./deploy.sh ./deploy_test.sh`：退出码 0，部署脚本和行为测试脚本语法有效。
- `bash ./deploy_test.sh`：退出码 0，覆盖默认参数、配置生成与保留、旧安装 `.env` 兼容、脏工作树拒绝、快进更新、引导克隆、Compose 调用、健康检查、Debian Docker 安装、未知发行版拒绝、失败回退、平台校验和主流程编排。
- `docker compose --env-file .env.example config --quiet`：退出码 0，默认环境示例与 Compose 配置兼容。
- `go test . -run 'TestControlServicesUseSharedUnixProxyAndPublishOnlyPanel|TestSocketProxyImageDoesNotExposeRetiredTCPPort' -count=1`：退出码 0，现有部署安全契约测试通过。
- `git diff --check`：退出码 0，没有空白错误。

## 外部契约核对

- 对照 Docker 官方 Debian 与 Ubuntu 安装文档核对软件源、冲突包和 `docker-ce`、`containerd.io`、Buildx、Compose 插件包名。

## 未覆盖范围

- 当前开发主机是 Windows + WSL，未在全新 Debian/Ubuntu 虚拟机上实际修改 APT 软件源或启动 systemd Docker 服务。
- 未执行真实 GitHub `curl | sudo bash` 和生产镜像完整构建；这些路径由临时 Git 仓库、命令替身、Compose 配置校验和现有 Go 部署契约测试覆盖。
