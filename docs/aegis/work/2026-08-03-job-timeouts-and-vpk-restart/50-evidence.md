# Job 超时与 VPK 重启验证证据

## 验证命令

- `go test ./... -count=1`
  - 全部 Go 包通过，无失败。
- `cd web && npm test -- --run`
  - 17 个测试文件、199 个测试通过。
- `cd web && npm run build`
  - TypeScript、Vite 和 WASM 构建成功；仅产生已知的 `web/public/vpk-cleaner.wasm` 二进制改写，未纳入提交。
- 相关增量验证：
  - `go test ./internal/store -count=1`
  - `go test ./internal/jobs -count=1`
  - `go test ./internal/vpkrestart -count=1`
  - `go test ./internal/scheduler ./internal/automation ./internal/httpapi -count=1`
  - `npm test -- --run src/app/JobsPage.test.tsx src/app/SchedulesPage.test.tsx src/app/SelfServiceVPKPage.test.tsx`

## 已证明行为

- Job 与计划任务超时字段可持久化，旧库和零值默认 1440 分钟；非法分钟数被拒绝。
- Job Manager 支持显式超时和锁前 preflight，普通 Job 默认入口保持兼容。
- 计划任务创建/编辑页面可按分钟配置超时，并传递到调度 Job。
- VPK 发布登记会立即创建带 `JobID` 的 `shared_vpk_restart`，同实例后续发布合并 publication。
- VPK 等待阶段不占实例锁；空服重启、查询失败持续等待、测试窗口到期后强制重启均有测试覆盖。
- 任务列表显示 Job 超时分钟数；公开 VPK 拖放上传仍通过回归测试。

## 残余风险

- 真实 24 小时等待未在自动化测试中实际等待，使用短注入窗口验证相同状态转换。
- 旧 `shared_vpk_restarts` 无 JobID 记录仍保留合成任务投影作为恢复兼容；需要后续现场迁移验证后退役。
- 外部进程是否及时响应 context 仍依赖各底层实现；Job Manager 不强杀忽略取消的进程。
