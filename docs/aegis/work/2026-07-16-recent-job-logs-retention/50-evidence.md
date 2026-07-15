# 最近任务日志与成功记录保留：完成证据

## Acceptance Evidence

- Store：迁移版本 6 增加任务开始/结束时间、`job_events` 和 `system_settings`；旧任务回填 snapshot 事件，成功任务清理只删除超额成功记录并级联事件。
- Manager：生成 queued、started、progress 和终态事件；成功后按全局设置清理，失败、中断和运行记录不计入上限。
- HTTP：`GET /api/jobs/{id}` 加法式返回 Events，摘要和 SSE 不携带事件；`GET/PUT /api/settings/jobs` 支持 1–500，非法输入返回 422。
- Web：最近任务支持单项行内展开、终态缓存、运行态刷新、失败原因、事件时间线、排队耗时和实际执行用时；系统设置可调整全局成功任务保留数量。
- Main journey：Playwright 通过真实 HTTP 在桌面与 390px 移动视口展开 `fixture_failure`，确认四个生命周期事件、`2分18秒` 执行用时、失败原因、无横向溢出；每个项目先建立上限 40，再通过 UI 保存 25 并用设置 API 回读确认。

## Verification Commands

- `go test ./...`：exit 0，所有 Go 包通过。
- `go vet ./...`：exit 0，零诊断输出。
- `cd web && npm test -- --run`：exit 0，6 个测试文件、92 个测试通过。
- `cd web && npm run build`：exit 0，TypeScript 和 Vite 生产构建通过；主 JS 616.81 kB，Vite 输出大于 500 kB 的非阻断提示。
- `cd web && npm run e2e`：exit 0，desktop 与 mobile 两个 Playwright 项目通过。
- `git diff --check`：exit 0，零格式错误。
- `git status --short`：验证时仅包含预期的实现、测试和 Aegis 记录；无 `node_modules`、`dist`、数据库或锁文件进入版本控制。

## Red-Green Evidence

- JobsPage：运行中任务 Expanded 后接收新 `UpdatedAt` 时原实现没有刷新详情；增加定向测试后以摘要/详情时间戳差异触发重新读取，目标测试转绿。
- 详情错误：重试测试暴露错误容器缺少 alert 语义；在规范容器增加 `role="alert"` 后转绿。
- Settings：默认值、保存禁用和失败恢复三个测试先因表单不存在而失败；实现 GET/PUT 状态流后转绿。
- E2E：桌面主流程先显示失败详情“没有可显示的结构化事件”、执行时间 `--`；夹具改用 `SaveJobWithEvent` 写确定性生命周期后 desktop/mobile 均转绿。
- Manager started 写失败：回归测试先显示业务函数仍被调用；`setStatus` 返回持久化错误且 `Start` 写入 failed 终态后，目标测试和 Manager 全包转绿。
- E2E 项目隔离：desktop 保存 25 后 mobile 先因期望 40、实际 25 失败；每个项目在认证后 PUT 40，完整 Playwright 转为 2/2 通过。
- 未执行终态：failed 且无 StartedAt 的测试先收到“排队中”；展示分支改为“未执行”并按 CreatedAt 到 FinishedAt 计算排队耗时后，目标测试、92 项 Vitest 和 Playwright 2/2 均通过。

## Visual Evidence

- Desktop：`web/test-results/control-panel-real-HTTP-ad-79bc6--and-streams-recovery-state-desktop/recent-job-logs.png`（242585 bytes）。
- Mobile：`web/test-results/control-panel-real-HTTP-ad-79bc6--and-streams-recovery-state-mobile/recent-job-logs.png`（44472 bytes）。
- 人工检查：桌面事件层级和失败摘要清晰；移动端失败原因正常换行，时间线保持可读，底部导航不遮挡任务面板；自动断言确认两种视口均无页面横向溢出。

## Drift And Risk

- 原始意图、全局范围、默认 25、1–500 边界和成功任务专属清理均保持一致。
- 旧 `GET /api/jobs/{id}`、jobs SSE 事件名、顶层任务字段、轮询与每实例串行执行保持兼容。
- App 内旧 JobsPage 已删除，任务列表展示只有一个所有者；摘要最后 Message 不再充当完整日志。
- SQLite 迁移为前向迁移，未执行旧二进制降级；回滚部署前仍需备份数据库。
- Vite bundle 体积提示不由本任务引入，也未在本任务范围内拆包。
- 最终复审对 `fba77f0..f310ba6` 给出 advisory merge-ready；两项 Important 均闭合，唯一 Minor 已通过后续前端回归闭合。

## Confidence

- Grade：A。
- 依据：跨 Store、Manager、HTTP、React 的单元/集成回归、生产构建、真实 HTTP 桌面/移动主流程和截图检查均有直接证据。
- Authority：以上是完成候选的验证证据；分支是否合并仍由仓库集成流程决定。
