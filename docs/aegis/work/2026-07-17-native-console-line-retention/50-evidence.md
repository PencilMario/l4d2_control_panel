# 原生控制台千行保留实施证据

- RED：新增测试因 `consoleBuffer` 不存在而失败。
- GREEN：纯函数测试覆盖单帧 1001 行、跨帧 1150 行、未换行尾部跨帧拼接。
- 旧 App 测试要求保留 500 个 WebSocket 块，按新规格更新为 1002 行输入后仅保留最新 1000 行。
- `npm test -- --run`：10 个文件、118 项测试全部通过。
- `npm run build`：生产构建通过；仅保留既有大 chunk 警告。
- `npx playwright test e2e/control-panel.spec.ts`：desktop 与 mobile 共 2 项通过。
- 兼容性：WebSocket、PTY、二进制解码、命令发送和滚动跟随未改变。
- 退休项：`[...old, text].slice(-500)` 已删除，没有保留消息块计数回退。
