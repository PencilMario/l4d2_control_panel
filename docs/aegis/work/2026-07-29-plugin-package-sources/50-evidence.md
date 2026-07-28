# 插件包来源重整验证证据

日期：2026-07-29

## 修复轨道

- 实例以互斥的 `package_id` 或 `source_id` 保存稳定插件来源，`applied_package_id` 只记录成功部署的具体包。
- GitHub 源实例更新先同步最新 Release，再停止实例并执行完整部署。
- 内容仓库 GitHub “同步”只下载/登记 Release，不部署实例。
- 实例更新对话框识别 `source_id`，不再因固定包 ID 为空而禁用。

## 退役轨道

- 删除内容仓库包行上的热更新/完整更新按钮和旧实例包更新 API 路由。
- 新建计划任务不接受 `package_hot`、`release_hot`；迁移 9 将旧任务转换为对应完整任务。
- 调度器的完整插件任务统一调用实例重装协调器。
- `package_source_repository` 只作为旧数据库迁移/恢复兼容字段保留，规范写入只使用 `source_id`。
- 底层更新事务仍可读取历史 `hot` journal，并保留内部相关测试；没有 UI、API 或调度生产入口可创建新的热更新任务。

## 自动化证据

- `go test ./...`：退出码 0，所有 Go 包通过。
- `npm test -- --run`：退出码 0，15 个测试文件、190 项测试通过。
- `npm run build:web`：退出码 0，TypeScript 构建和 Vite 生产构建成功。
- `git diff --check`：退出码 0。
- 定向红灯：新增来源同步测试最初因 `GameCoordinator` 不存在 `Sources` 字段而编译失败；实现后 `internal/updates` 通过。
- 定向主路径：React 测试确认 `source_id` 实例可以提交 `/api/instances/{id}/game-update` 完整插件重装请求。

## 风险边界

- 未连接真实 GitHub 和 Docker 执行浏览器端到端部署；网络下载和实例文件事务由现有 Release、Pipeline 及协调器自动化测试覆盖。
- Vite 报告现有主 chunk 超过 500 kB，该警告与本次插件包行为重整无关。
