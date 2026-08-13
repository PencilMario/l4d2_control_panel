# 崩溃 AI 分析 Markdown 阅读页

## TaskIntentDraft

- 目标：让管理员在崩溃详情中通过“查看 AI 分析”进入独立的大阅读视图，并将服务端保存的 AI 分析原文以 Markdown 呈现。
- 范围：仅前端 UI、前端依赖和相应 Vitest 覆盖。
- 非目标：不修改崩溃报告 API、`ai_analysis` 的存储格式、AI 请求、后台任务或实例生命周期。
- 风险：AI 生成内容不可信；渲染不得执行其中的 HTML 或脚本。

## ImpactStatementDraft

`CrashReportsPage` 保持当前报告选择与下载/重分析能力；新增阅读态只能由已成功且有 `ai_analysis` 的详情进入，返回后恢复同一详情。Markdown 渲染只读取现有字符串，采用不启用 raw HTML 的渲染器。
