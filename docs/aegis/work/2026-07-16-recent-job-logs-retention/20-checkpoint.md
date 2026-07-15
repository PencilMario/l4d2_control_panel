# 最近任务日志与成功记录保留：执行检查点

## TodoCheckpointDraft

- 当前目标：完成结构化任务日志、执行时间和全局成功任务保留设置。
- 活动切片：E1，系统设置任务记录表单测试红灯。
- 已完成：规格/计划/基线、Store、Manager、HTTP 详情与设置、JobsPage。
- 待完成：Settings、E2E、完整验证。
- 阻塞项：无。
- 下一步：提交 JobsPage 切片，再为成功任务保留数量表单写失败测试。

## Evidence

- go test ./...：基线全部包通过。
- cd web && npm test -- --run：基线 5 个文件、83 个测试通过。
- 分支：feature/recent-job-logs-retention。
- 计划提交：8d9953a。
- 红灯：go test ./internal/store -run TestJobHistoryMigrationBackfillsLegacySnapshot -count=1，因缺少 StartedAt、FinishedAt 和 JobEvents 构建失败。
- 绿灯：同一迁移测试通过；旧计划任务时间解析所有者保持不变。
- 绿灯：go test ./internal/store -count=1。
- 扩展回归：go test ./internal/store ./internal/jobs ./internal/httpapi -count=1。
- Manager 红灯：Details 缺失；清理测试显示旧成功任务仍可从内存访问。
- Manager 绿灯：生命周期顺序与全局清理目标测试通过。
- 竞态回归：TestPersistentManagerReloadsCompletedJob 连续 5 次及 Manager 全包通过。
- HTTP 红灯：详情缺 Events；设置路由 404；非法上限被错误映射为 500。
- HTTP 绿灯：详情/摘要、设置清理、422 边界和 HTTP 全包通过。
- JobsPage 红灯：运行任务摘要 UpdatedAt 更新后详情没有重新读取；详情错误缺少 alert 语义。
- JobsPage 绿灯：运行态刷新、单项展开、收起、终态缓存、错误重试和空事件状态通过。
- App 集成回归：npm test -- --run src/app/JobsPage.test.tsx src/app/App.test.tsx，2 个文件、42 个测试通过。

## DriftCheckDraft

- 原始意图：一致。
- 兼容边界：保留任务顶层字段、SSE、轮询和每实例串行执行。
- 新所有者：job_events 由 Store 持久化、Manager 生成事件；没有重复日志路由。
- 退休边界：App.tsx 内旧 JobsPage 在新组件接管后删除。
- 决策：continue，进入系统设置切片。

## Risk / Unknown

- 旧任务执行时间只能估算，必须显示 snapshot 说明。
- 成功记录删除不可恢复，设置更新与清理必须同事务。
