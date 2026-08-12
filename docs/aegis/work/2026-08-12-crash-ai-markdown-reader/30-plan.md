# 崩溃 AI 分析 Markdown 阅读页 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `aegis:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变崩溃报告 API 的前提下，提供可返回的独立 AI 分析 Markdown 阅读视图。

**Architecture:** `CrashReportsPage` 继续拥有报告详情和阅读态；新的 `CrashAnalysisReader` 只负责展现已加载的分析原文与返回操作。`react-markdown` 加 `remark-gfm` 将文本变为安全的 React 元素，不安装或启用 HTML 原文解析插件。

**Tech Stack:** React 19、TypeScript、Vitest、Testing Library、`react-markdown`、`remark-gfm`。

**Baseline / Authority Refs:** `CONTEXT.md`；`docs/aegis/specs/2026-08-10-accelerator-management-analysis-design.md`；本目录 `00-intent.md`、`10-baseline-readset.md`。

**Compatibility Boundary:** 保持所有现有 HTTP 调用、DTO 和报告详情行为；仅成功且含非空 `ai_analysis` 的报告显示入口。AI 输出永远不作为可执行 HTML 处理。

**Verification:** `npm test -- --run src/app/CrashReportsPage.test.tsx`、`npm test -- --run`、`npm run build`、`git diff --check`，以及安可上真实 AI 报告的 Playwright 主流程检查。

---

### Task 1: 为阅读流程建立回归测试

**Files:**

- Modify: `web/src/app/CrashReportsPage.test.tsx`

**Why this task exists:** 锁定“详情进入阅读页、GFM 标题/列表/代码块语义渲染、返回原详情”的用户可见流程。

**Impact / Compatibility:** 不改 mock API；保留已有列表、详情和重新分析断言。

**Verification:** `cd web; npm test -- --run src/app/CrashReportsPage.test.tsx` 在实现前以缺少入口失败。

- [ ] 写入带 Markdown 标题、列表与 fenced code block 的详情 fixture。
- [ ] 断言“查看 AI 分析”按钮出现，点击后显示阅读页语义元素而不是 `<pre>` 文本块。
- [ ] 断言返回后现有崩溃详情仍在。

### Task 2: 实现安全阅读组件与入口

**Files:**

- Create: `web/src/app/CrashAnalysisReader.tsx`
- Modify: `web/src/app/CrashReportsPage.tsx`, `web/package.json`, `web/package-lock.json`

**Why this task exists:** 将长篇分析从紧凑诊断卡中移出，同时保留服务器保存的 Markdown 结构。

**Impact / Compatibility:** 原 AI 卡只保留状态、错误和打开按钮；`CrashReportsPage` 在切换报告时关闭阅读态，避免错读另一份报告。仅添加渲染依赖，不改变浏览器请求。

**Verification:** Task 1 的测试变绿；测试用 `<h1>`、`<ul>`、`<code>` 验证 GFM/代码块输出。

- [ ] 添加 `react-markdown` 和 `remark-gfm` 生产依赖。
- [ ] 创建接收 `analysis`、`title` 和 `onBack` 的无状态阅读组件。
- [ ] 使用 `<ReactMarkdown remarkPlugins={[remarkGfm]}>` 渲染文本，不添加 `rehype-raw`。
- [ ] 将已有 AI 原始 `<pre>` 替换为可访问的“查看 AI 分析”按钮，并在阅读态渲染独立组件。

### Task 3: 将阅读态融入现有视觉与验证

**Files:**

- Modify: `web/src/styles/app.css`, `web/src/app/CrashReportsPage.test.tsx`

**Why this task exists:** 保证长文、表格和代码在桌面/手机屏幕都能阅读，并保留面板的深色信息层级。

**Impact / Compatibility:** CSS 仅限定到阅读器类名；不重设全局排版或更改其他页面。

**Verification:** 目标测试、全量前端测试、生产构建与真实报告人工/Playwright 主流程。

- [ ] 增加全宽阅读容器、返回栏和报告身份摘要。
- [ ] 增加限定的 Markdown 标题、段落、链接、列表、引用、代码、表格和窄屏横向滚动样式。
- [ ] 运行全量测试和构建，审查 diff 与依赖锁文件。

## Rollback Surface

删除新组件、阅读态与两个依赖即可回到既有紧凑 `<pre>` 展示；后端数据、报告文件和 API 不需要迁移或回滚。
