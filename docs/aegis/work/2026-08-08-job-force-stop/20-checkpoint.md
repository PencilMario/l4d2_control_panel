# Checkpoint: 后台任务强制停止

## TodoCheckpointDraft

- 当前 todo：回写远端证据，提交并推送证据文档。
- 已完成：设计与基线读取；Manager 取消 RED/GREEN；认证 HTTP 取消 RED/GREEN；JobsPage 取消交互 RED/GREEN；局部 CSS 兼容；本地全量验证；琥珀和安可部署与 sudoers 验证。
- 当前切片：两个生产节点已运行目标 revision，远端健康检查和免密 sudo 均已通过。
- 下一步：更新 `50-evidence.md`，提交并推送文档更新；无需重新部署运行代码。

## Evidence

- `npm test -- --run src/app/JobsPage.test.tsx src/styles/app.test.ts`: 2 files / 33 tests passed。
- `npm test -- --run`: 19 files / 207 tests passed。
- `npm run build:web`: exit 0；仅有既有 chunk size warning。
- `go test -p 1 -count=1 ./...`: exit 0。
- `go vet ./...`: exit 0。
- `git diff --check`: exit 0。
- 并行 `go test ./...` 曾触发 Windows 临时目录清理和测试 exe 文件锁；对应目标测试各连续 3 次通过，串行全量套件通过。
- 琥珀与安可新鲜 SSH 验证：revision 均为 `f4b28cc1f1ef53365d7d51d66413f7772275fb4a`，分支为 `main`，工作树干净。
- 两台 `sudo docker compose --env-file .env ps`：panel 均为 `Up (healthy)`；`curl -fsS http://127.0.0.1:18081/api/health` 均返回 `status: ok`、`database: ok`。
- 两台 `/etc/sudoers.d/steam-nopasswd`：内容为 `steam ALL=(ALL) NOPASSWD:ALL`，权限/所有者为 `0440:root:root`；`visudo` 和无密码 `sudo -n true` 均通过。

## DriftCheckDraft

- 目标范围：仍服务于执行中后台任务强制停止及两台节点更新。
- 兼容边界：Manager 是取消和终态持久化唯一 owner；HTTP 只转发；前端沿用任务 SSE、表格网格和移动端操作列；无 schema 迁移。
- Owner/retirement：运行时取消由 Manager 新 owner 产生；重启恢复的 `interrupted` 路径继续作为进程恢复兜底。
- 决策：continue；等待远端证据。

## Risk / Unknown

- 未运行 `go test -race`，当前 Windows 环境缺少 GCC。
- 浏览器级生产交互仍未在远端执行；琥珀部署前的另一项超时/VPK 重启未提交改动已保留在 `stash@{0}`，未覆盖。
