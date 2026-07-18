# 基线读取集

- `docs/aegis/specs/2026-07-18-persistent-game-log-viewer-design.md`：已批准的日志页视觉与交互契约。
- `docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md`：私有文件页的既有工作区行为。
- `web/src/app/PrivateFilesPage.tsx`：页头、目标实例、工具栏、双栏树、查看卡片和抽屉的参考实现。
- `web/src/styles/app.css`：`private-*` 共享视觉契约及 759px 响应式断点的唯一样式所有者。
- `web/src/app/GameLogsPage.tsx`、`web/src/app/App.tsx`：当前偏差实现。
- `web/src/app/GameLogsPage.test.tsx`、`web/src/app/App.test.tsx`、`web/src/styles/app.test.ts`、`web/e2e/control-panel.spec.ts`：回归证据所有者。

基线命令 `npm test -- --run src/app/GameLogsPage.test.tsx src/app/App.test.tsx src/styles/app.test.ts` 已通过：3 个文件、71 个测试。这证明功能基线正常，同时确认现有测试未约束布局一致性。

