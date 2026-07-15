# 最近任务日志与成功记录保留：执行检查点

## TodoCheckpointDraft

- 当前目标：完成结构化任务日志、执行时间和全局成功任务保留设置。
- 活动切片：完成候选，分支交付检查。
- 已完成：规格/计划/基线、Store、Manager、HTTP、JobsPage、Settings、E2E、最终复审修复、完整验证和视觉检查。
- 待完成：分支集成方式确认。
- 阻塞项：无。
- 下一步：执行 finishing-development-branch 流程。

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
- Settings 红灯：三个任务保留设置测试均因表单不存在而失败。
- Settings 绿灯：默认值、1–500 边界、PUT、保存禁用、失败恢复和范围说明通过。
- App 全量组件回归：npm test -- --run src/app/App.test.tsx，41 个测试通过。
- E2E 红灯：fixture_failure 无结构化事件且执行用时为 `--`。
- E2E 绿灯：desktop/mobile 均展开四个生命周期事件、显示 2分18秒、保存设置 25，并通过无横向溢出断言。
- 完整后端：go test ./... 全部通过；go vet ./... exit 0。
- 审查修复：生命周期持久化失败不再领先更新内存；started 写失败不运行业务函数，并持久化 failed 终态。
- 审查修复：无进度成功任务写入 complete / Task completed，旧进度仍保留在事件历史。
- 审查修复：旧快照保留原错误或消息并追加执行时间估算标记；相同结束时间按 ID 稳定清理；设置清理失败完整回滚。
- 审查修复：设置 PUT 严格拒绝第二个 JSON 值和尾随文本；重复任务类型的日志按钮使用完整任务 ID 区分。
- Manager 红灯：started 生命周期写入失败后业务函数仍运行；绿灯：业务函数未运行，最终事件为 queued、failed，StartedAt 为空。
- E2E 隔离红灯：desktop 保存 25 后，mobile 期望 40 实际读到 25；绿灯：每个项目登录后重置 40，随后真实 UI 保存 25 并通过 API 回读。
- 最终复审：Critical/Important 均为零；唯一 Minor 是未开始的终态任务错误显示“排队中”。
- Minor 红灯：failed 且无 StartedAt 的任务摘要显示“排队中”；绿灯：摘要和执行用时显示“未执行”，排队耗时按 CreatedAt 到 FinishedAt 计算。
- 完整前端：6 个 Vitest 文件、92 个测试通过；生产构建 exit 0。
- 完整浏览器：2 个 Playwright 项目通过；桌面与移动任务日志截图已生成并人工检查。
- 副作用：git diff --check 通过；无构建产物、数据库、依赖目录或锁文件进入 Git 状态。

## DriftCheckDraft

- 原始意图：一致。
- 兼容边界：保留任务顶层字段、SSE、轮询和每实例串行执行。
- 新所有者：job_events 由 Store 持久化、Manager 生成事件；没有重复日志路由。
- 退休边界：App.tsx 内旧 JobsPage 在新组件接管后删除。
- 决策：continue，最终复审问题已闭合，进入分支交付。

## Risk / Unknown

- 旧任务执行时间只能估算，必须显示 snapshot 说明。
- 成功记录删除不可恢复，设置更新与清理必须同事务。
- SQLite 为前向迁移，未验证旧二进制降级；回滚部署前需备份数据库。
- Vite 保留既存的单 bundle 体积提示，本任务不扩展到拆包。
