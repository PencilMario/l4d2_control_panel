# 控制台文本导出验证证据

## RED/GREEN

- `npm --prefix web test -- --run src/app/App.test.tsx -t "keeps the newest 8192 console lines"`：实现前因找不到“导出控制台文本”按钮失败；实现后 `1 passed`。
- `npm --prefix web test -- --run src/app/consoleExport.test.ts`：`2 passed`。

## Regression

- `npm --prefix web test -- --run`：`22` 个测试文件、`230` 个测试通过。
- `go test ./...`：所有 Go 包通过；无失败包。
- `npm --prefix web run build`：TypeScript 检查和 Vite 构建退出码为 `0`；仅有既有 chunk 大小提示。
- 构建产生的 `web/public/vpk-cleaner.wasm` 已恢复为基线版本，未进入差异。
- `git diff --check`：无空白错误。

## Evidence Boundary

- 已验证：导出主路径、文件名边界、8192 行前后端缓存行为、前端全量测试、Go 全量测试和生产构建。
- 未验证：真实浏览器下载目录中的文件落盘；自动化测试已验证 Blob 内容、下载属性、点击触发和 URL 释放。
- 未运行：控制台真实游戏实例 E2E；本次改动不涉及 WebSocket 协议或后端接口。
