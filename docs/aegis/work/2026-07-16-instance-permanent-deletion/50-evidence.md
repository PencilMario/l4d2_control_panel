# 实例永久删除实施证据

## 行为证据

- 组件 RED：`npm test -- --run src/app/App.test.tsx` 在缺少“删除实例 深夜战役”按钮时失败。
- 组件 GREEN：48 项 App 测试通过，覆盖完整名称解锁、固定 `DELETE` 请求体、成功刷新、重复提交保护和失败保留。
- 永久删除请求固定为 `{ "confirm": true, "delete_data": true }`。
- 后端仍由 `lifecycle.Service.Delete` 负责停止实例、删除容器、删除记录和清理数据目录；浏览器没有新增分步清理逻辑。

## 自动化验证

- `npm test -- --run`：9 个文件、115 项测试全部通过。
- `npm run build`：TypeScript 与 Vite 生产构建通过；仅保留既有的大 chunk 警告。
- `npx playwright test e2e/control-panel.spec.ts`：desktop 与 mobile 共 2 项通过。
- Playwright 覆盖错误名称禁用、完整名称解锁、DELETE 请求体、删除后卡片消失、弹窗视口边界和横向溢出。
- `go test ./internal/lifecycle ./internal/httpapi`：两个相关 Go 包通过。
- `go vet ./...`：通过。
- `git diff --check`：通过。

## 视觉证据

- 桌面和移动截图均显示永久删除后果、实例名称输入和明确的危险按钮。
- 删除入口位于实例卡片右上角，与配置工具并列，未改变卡片固定布局和高频操作区高度。
- 移动弹窗在 390x844 视口内完整显示，无横向溢出。

## 兼容性与剩余状态

- 删除 API、任务格式、任务日志、实例目录结构和生命周期清理顺序未改变。
- UI 不提供保留数据选项。
- 尚未部署到安可服务器；服务器此前整机失联，集成后需在主机恢复时部署并进行真实实例删除前的非破坏性线上检查。
