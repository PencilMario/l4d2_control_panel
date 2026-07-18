# 验证证据

- 基线：日志相关 Vitest 3 个文件、71 个测试通过。
- RED：新增页内实例选择、共享布局 CSS 所有权和应用选择器测试均按预期失败。
- GREEN：`npm test -- --run src/app/GameLogsPage.test.tsx src/app/App.test.tsx src/styles/app.test.ts`：75/75 通过。
- 全量前端：`npm test -- --run`：12 个文件、151 个测试通过。
- 构建：`npm run build`：`tsc -b` 与 Vite 构建退出码 0；仅有既存 bundle 体积提示。
- 端到端：`npm run e2e`：desktop 与 mobile 主流程 2/2 通过。
- 收尾：`git diff --check` 无输出；`git status --short` 清洁；临时分支与 worktree 已删除。

