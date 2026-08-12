# Task Intent

## Requested outcome

崩溃转储上传只保存报告并执行必要的 Stackwalk，不自动生成 AI 报告；只有管理员在崩溃报告详情点击“重新分析”后，才创建包含 AI 分析的后台任务。

## Scope

- 修改上传后的崩溃分析触发路径。
- 保留手动 `POST /api/crash-reports/{id}/analyze` 接口和前端按钮行为。
- 保留 `AutoCrashAnalysis` 实例字段、设置接口和数据库兼容性，但不再让它触发上传后的 AI 任务。
- 增加后端回归测试和前端手动入口回归测试。
- 部署到安可服并验证线上上传/手动分析边界。

## Non-goals

- 不删除 `AutoCrashAnalysis` 字段或迁移既有数据。
- 不改变 Stackwalk 自动处理、报告保存、符号上传、二进制上传或 AI Markdown 格式。
- 不改变手动分析接口的请求格式、后台任务类型或持久化等待行为。

## Risk hints

- 上传路径和手动分析路径共享 `crashanalysis.Worker`，必须只移除上传侧的 AI 入队，不要误删 Stackwalk。
- 既有实例可能仍保存 `auto_crash_analysis=true`，其存在不应继续造成 AI 任务。
- 上传协议是 Accelerator 兼容边界，不能因调整回调而改变 `OK|<report id>` 响应。
