# Game Log Layout Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 使游戏日志页面在页头、实例选择、工具栏、工作区、文件树和移动端抽屉上与私有文件页面保持同一设计契约。

**Architecture:** `GameLogsPage` 接收实例集合并在页内拥有目标实例选择状态，`App` 不再为日志页在侧栏维护专用选择器。结构直接使用经过验证的 `private-*` 布局类，日志只保留查看器和 token 高亮的专属样式，消除第二套布局所有者。

**Tech Stack:** React 19、TypeScript、Vitest、Testing Library、Playwright、CSS。

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-18-persistent-game-log-viewer-design.md`、`web/src/app/PrivateFilesPage.tsx`、`web/src/styles/app.css`。

**Compatibility Boundary:** 保持日志树/预览/下载 API、切换实例清空旧内容、刷新当前预览、轮转提示、安全高亮和移动端焦点管理；不复用私有文件的写操作。

**Verification:** `npm test -- --run src/app/GameLogsPage.test.tsx src/app/App.test.tsx src/styles/app.test.ts`、`npm test -- --run`、`npm run build`，并更新 Playwright 主流程的可访问名称。

---

### Task 1: 页内实例选择与共享页面结构

**Files:**
- Modify: `web/src/app/GameLogsPage.test.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/app/GameLogsPage.tsx`
- Modify: `web/src/app/App.tsx`

**Why this task exists:** 管理员在与私有文件相同的位置选择目标实例，并在相同层级中操作日志树和查看器。

**Impact / Compatibility:** `GameLogsPage` 成为日志页实例选择的唯一所有者；`App` 中侧栏日志选择器退役。实例切换的请求竞态保护保持有效。

**Verification:** 目标组件与应用测试证明目标实例选择器位于日志页中，切换后请求新实例且旧预览立即消失。

- [ ] 写入失败测试，要求日志页包含 `private-files-page`、页内“目标实例”、共享工具栏/工作区和本地化抽屉名称。
- [ ] 运行目标测试，确认因现有单实例 props 和旧结构而失败。
- [ ] 将 `GameLogsPage` 改为接收 `instances`，迁入实例选择状态并复用 `private-*` 结构类。
- [ ] 从 `App` 移除侧栏专用实例选择器并传递实例集合。
- [ ] 运行目标测试并确认通过。

**Repair Track:** 修复结构所有权偏离已批准设计的问题，最小变更限定在日志页装配与 DOM 结构。

**Retirement Track:** 删除 `App` 侧栏的“当前实例”日志专用选择器；不保留兼容分支。

### Task 2: 共享样式所有权与响应式回归

**Files:**
- Modify: `web/src/styles/app.test.ts`
- Modify: `web/src/styles/app.css`
- Modify: `web/e2e/control-panel.spec.ts`

**Why this task exists:** 桌面双栏、边框、间距和移动端抽屉必须由私有文件同一套规则控制，避免视觉再次漂移。

**Impact / Compatibility:** 删除 `game-logs-layout` 和独立 700px 断点；日志查看器只保留内容呈现样式。

**Verification:** CSS 契约测试证明日志结构不再拥有独立布局规则；完整前端测试和生产构建通过。

- [ ] 写入失败 CSS 契约测试，禁止第二套日志布局/断点并要求日志工作区扩展规则。
- [ ] 运行样式测试，确认在旧 `game-logs-*` 布局上失败。
- [ ] 删除重复布局 CSS，仅添加日志正文适配共享工作区所需的专属规则。
- [ ] 更新 Playwright 选择器为“目标实例”“打开文件树”等共享可访问名称。
- [ ] 运行目标测试、全部前端测试和生产构建。

**Repair Track:** 把布局样式所有权收敛到 `private-*` 契约。

**Retirement Track:** `game-logs-layout`、`game-logs-tree-trigger`、独立抽屉覆盖层和 700px 断点全部退役；日志 token 色彩规则继续保留。

