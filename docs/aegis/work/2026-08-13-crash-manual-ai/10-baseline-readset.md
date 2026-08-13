# Baseline Read Set

- `cmd/panel/main.go`: 生产启动时组装崩溃报告接收器和分析 Worker；当前上传回调读取实例 `AutoCrashAnalysis` 并调用 `analysisWorker.Enqueue(..., true)`。
- `internal/crashreports/config.go`: `Config.EnqueueAnalysis` 是上传完成后的可选回调，属于接收器扩展点。
- `internal/crashreports/manager.go`: `Receive` 保存报告后调用上传分析回调；Stackwalk 本身不由该回调实现。
- `internal/crashanalysis/worker.go`: `Enqueue`/`Analyze` 区分 `requestAI=false` 与 `requestAI=true`，手动 AI 请求必须继续使用后者。
- `internal/httpapi/accelerator.go`: `POST /api/crash-reports/{id}/analyze` 创建 `crash_analysis` 任务，默认请求 AI。
- `internal/httpapi/accelerator_test.go`: 现有手动分析 API、空请求体和任务持久化等待测试。
- `internal/crashreports/manager_test.go`: 接收器测试和上传回调测试位置。
- `web/src/app/CrashReportsPage.tsx`、`web/src/app/CrashReportsPage.test.tsx`: 前端只通过详情页按钮调用手动分析接口。
- `CONTEXT.md`: 项目术语与现有边界；本任务不改变领域模型字段。
