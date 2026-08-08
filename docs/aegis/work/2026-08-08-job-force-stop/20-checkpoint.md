# Checkpoint: 后台任务强制停止

## TodoCheckpointDraft

- 当前 todo：提交、推送并更新两个生产节点。
- 已完成：设计与基线读取；Manager 取消 RED/GREEN；认证 HTTP 取消 RED/GREEN；JobsPage 取消交互 RED/GREEN；局部 CSS 兼容；本地全量验证。
- 当前切片：审查复核已通过，提交与部署准备。
- 下一步：复核最终 diff，提交并推送 `main` 所需 revision；再通过 SSH 更新琥珀和安可。

## Evidence

- `npm test -- --run src/app/JobsPage.test.tsx src/styles/app.test.ts`: 2 files / 33 tests passed。
- `npm test -- --run`: 19 files / 207 tests passed。
- `npm run build:web`: exit 0；仅有既有 chunk size warning。
- `go test -p 1 -count=1 ./...`: exit 0。
- `go vet ./...`: exit 0。
- `git diff --check`: exit 0。
- 并行 `go test ./...` 曾触发 Windows 临时目录清理和测试 exe 文件锁；对应目标测试各连续 3 次通过，串行全量套件通过。

## DriftCheckDraft

- 目标范围：仍服务于执行中后台任务强制停止及两台节点更新。
- 兼容边界：Manager 是取消和终态持久化唯一 owner；HTTP 只转发；前端沿用任务 SSE、表格网格和移动端操作列；无 schema 迁移。
- Owner/retirement：运行时取消由 Manager 新 owner 产生；重启恢复的 `interrupted` 路径继续作为进程恢复兜底。
- 决策：continue；等待远端证据。

## Risk / Unknown

- 未运行 `go test -race`，当前 Windows 环境缺少 GCC。
- 远端部署 revision、Compose 状态、健康接口和浏览器级按钮行为尚未验证。
