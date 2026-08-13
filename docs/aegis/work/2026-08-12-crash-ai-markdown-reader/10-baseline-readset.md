# Baseline Read Set

- `CONTEXT.md`：项目与“游戏实例”术语边界。
- `docs/aegis/specs/2026-08-10-accelerator-management-analysis-design.md`：崩溃报告与 AI 分析的既有功能边界。
- `web/src/app/CrashReportsPage.tsx`：详情加载、AI 状态、现有小窗展示和回退状态的唯一 UI owner。
- `web/src/app/CrashReportsPage.test.tsx`：页面加载、下载和重新分析的现有行为保护。
- `web/src/styles/app.css`：当前深色控制面板与崩溃页响应式规则。
- `web/package.json`：React/Vite/Vitest 依赖与验证入口。

## Compatibility Boundary

`GET /api/crash-reports/{id}` 返回的 `ai_analysis` 字符串保持原样；现有报告列表、筛选、详情、stackwalk、artifact 下载、重新分析和错误/加载状态不改变。浏览器不解析或执行 Markdown 内嵌 HTML。
