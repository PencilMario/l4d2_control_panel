# 游戏实例控制台缓存设置验证证据

## 变更范围

- 默认控制台缓存为 8192 行，管理员可在“系统设置”中配置 1～1,000,000 行。
- 配置通过 `system_settings` 中的 `console_history_lines` 键值持久化；保存成功后后端现有会话和前端已打开控制台立即裁剪。
- 控制台 TXT 导出、WebSocket 地址/输入输出协议和后端原有 1 MiB 历史字节上限保持不变。
- 最后实现提交：`f0e3bc7`（包含并发设置更新修复）。

## 验证命令与结果

| 命令 | 结果 |
| --- | --- |
| `go test ./internal/store -run '^TestConsoleHistoryLinesDefaultsPersistsAndRejectsInvalidValues$' -count=1` | 通过 |
| `go test ./internal/httpapi -count=1` | 通过 |
| `go test ./internal/httpapi -run '^TestConsole(SettingsReadUpdateAndRejectInvalidValues\|SettingsUpdateSerializesPersistenceAndHubMutation\|HistoryLimit\|WebSocket)' -count=1` | 通过 |
| `npm test -- --run src/app/consoleBuffer.test.ts src/app/App.test.tsx` | 2 个文件、77 个测试通过 |
| `npm test -- --run` | 22 个文件、237 个测试通过 |
| `npm run build:web` | 通过；Vite 完成生产构建 |
| `npm run build` | 通过；WASM 与 Web 构建完成，构建产生的 `web/public/vpk-cleaner.wasm` 已恢复，未纳入本功能变更 |
| `go vet ./...` | 通过 |
| `git diff --check` | 通过 |

## Go 全量限制

最后一次 `go test ./... -p 1 -count=1` 退出码为 1，但本次涉及的包已单独通过。失败来自既有 Windows 测试临时目录清理竞态：

- `internal/maintenance/TestCleanupPackagesProtectsSelectedAppliedAndLatest`：`packages/releases` 清理时目录仍非空。
- `internal/updates/TestPackageUpdateRebasesDeletedPrivatePath`：清理期间找不到 `PLUGIN.CFG.tmp`。

更早的全量尝试还出现过同类 `internal/content`、`internal/store` 临时目录清理失败；没有修改这些无关测试或生产代码。

`go test -race ...` 未能启动，环境要求 `CGO_ENABLED=1`，当前 Go 环境禁用 CGO；普通目标测试和 `go vet` 已完成。

## 审查与并发修复

独立审查发现两个并发 `PUT /api/settings/console` 可交错执行：请求 A 写入 SQLite 后暂停，请求 B 写入并更新 Hub，随后 A 更新 Hub，造成持久值与内存值分叉。修复为 `Server.consoleSettingsMu` 同时保护设置读取、SQLite 写入和 Hub 更新，并加入 `TestConsoleSettingsUpdateSerializesPersistenceAndHubMutation` 回归测试。

## 远端安可状态

只读 SSH 检查使用别名 `sirphomesv`，目标为 `sirp@sirphomesv`（`100.72.137.92`）：

- `/home/sirp/l4d2_control_panel` 存在，但不是 Git 工作区，当前标记为 `3b60ee4c7f95`。
- 该目录的 Compose 配置校验通过。
- 该 Compose 当前没有容器；`127.0.0.1:18081/api/health` 不可用。
- 18080 端口和运行中的 `deploy-frontend-1` 属于其他 `matchmaking-core`/前端 Compose 项目。
- 未执行 SCP、远端文件覆盖、Compose build/up 或实例重启，因此没有声称远端部署完成。

## 修复轨与退役轨

### 修复轨

- 修复对象：控制台缓存行数的 Store、HTTP API、Hub 和前端设置/显示链路。
- 影响范围：管理员设置、控制台历史恢复、当前控制台显示；既有导出和连接协议保留。
- 证据：上述 Store/HTTP/Vitest/构建/静态检查结果，以及并发回归测试。

### 退役轨

- 退役对象：原后端静态 8192 行限制。
- 退役动作：由动态 Hub 行数上限取代，8192 仅作为缺省值；旧数据库不需要迁移。
- 保留边界：1 MiB 后端字节上限、TXT 导出和 WebSocket 协议继续保留。
- 后续触发：远端部署并完成健康检查后，再确认现场数据库设置和控制台主流程。

## 残余风险与下一步

- Go 全量的既有 Windows TempDir 竞态仍未修复；需要单独的测试清理任务处理。
- 未在真实浏览器/远端实例上执行下载文件和长时间控制台输出手工检查；前端测试覆盖设置保存、即时裁剪和导出调用。
- 远端 Panel 服务停止且启动可能触发实例对账。需要用户明确确认启动并更新该 Compose 后，才能进行远端写操作。
