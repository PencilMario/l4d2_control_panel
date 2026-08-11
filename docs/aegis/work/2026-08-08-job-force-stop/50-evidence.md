# Evidence: 后台任务强制停止

## Local

- `go test -p 1 -count=1 ./...`: PASS。
- `go vet ./...`: PASS。
- `npm test -- --run`: PASS，19 个测试文件、207 条测试。
- `npm test -- --run src/app/JobsPage.test.tsx src/styles/app.test.ts`: PASS，2 个测试文件、33 条测试。
- `npm run build:web`: PASS，TypeScript 与 Vite 构建完成；保留既有 chunk size warning。
- `git diff --check`: PASS。
- `go test -race`：未运行，环境缺少 GCC。
- 并行 `go test ./...` 的 Windows 临时目录/测试 exe 锁失败未在串行模式复现；对应目标测试各连续 3 次通过。

## Review

- 提交前代码审查：通过 advisory review；上一轮发现的终态竞态、孤立 running 快照、并行停止状态和操作列溢出均已修复。
- 审查保留的小项：未运行 `go test -race`；未做真实浏览器桌面/390px 几何截图；取消请求失败、preflight 取消和未认证 POST 没有新增专门测试。

## Deployment

- 部署方式：通过 SSH 使用 `sudo -S bash /opt/l4d2-control-panel/deploy.sh`；两台远端脚本缺少执行位，因此显式用 `bash` 启动，脚本均退出码 0。
- 目标 revision：`f4b28cc1f1ef53365d7d51d66413f7772275fb4a`。
- 琥珀：`main`，revision 为目标值；panel 为 `Up (healthy)`，映射 `0.0.0.0:18081->8080/tcp`；`/api/health` 返回 `{"containers_running":19,"database":"ok","docker_version":"27.2.0","status":"ok"}`。
- 安可：`main`，revision 为目标值；panel 为 `Up (healthy)`，映射 `0.0.0.0:18081->8080/tcp`；`/api/health` 返回 `{"containers_running":7,"database":"ok","docker_version":"29.2.1","status":"ok"}`。
- 两台部署后的 `git status --porcelain --untracked-files=all` 均为空；Compose 其余 helper/proxy 服务均为运行中。
- 琥珀部署前原有的超时/VPK 重启未提交改动已保存为 `stash@{0}: codex-pre-deploy-backup-job-force-stop-2026-08-08`，没有覆盖或删除。

## Sudoers

- 两台均写入 `/etc/sudoers.d/steam-nopasswd`：`steam ALL=(ALL) NOPASSWD:ALL`。
- 两台均通过 `visudo -cf` 和完整 `visudo -c`；文件权限/所有者均为 `0440:root:root`。
- 两台均以无密码 SSH 会话执行 `sudo -n true` 和 `sudo -n id -u`，分别返回成功和 UID `0`。
- 临时文件校验后已移动，最终复核 `/etc/steam-nopasswd.tmp*` 无残留。

## Remote Interaction

- 浏览器级生产交互未在远端执行；本地 React 测试覆盖确认、POST、停止中禁用和终态 SSE。
