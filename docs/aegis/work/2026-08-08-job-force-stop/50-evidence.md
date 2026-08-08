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

- 分支/revision：待提交并推送。
- 琥珀：待 SSH 部署；待记录 revision、Compose 状态、`/api/health`。
- 安可：待 SSH 部署；待记录 revision、Compose 状态、`/api/health`。
- 浏览器级生产交互：未在远端浏览器执行；本地 React 测试已覆盖确认、POST、停止中禁用和终态 SSE。
