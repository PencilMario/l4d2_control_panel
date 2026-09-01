# 游戏实例控制台缓存设置设计

## 目标

让管理员在“系统设置”中自由设置游戏实例控制台最多保留的文本行数。默认 8192 行，最小 1 行，最大 1,000,000 行。设置写入 SQLite，Panel 重启后继续生效；保存成功后，后端现有会话和前端当前控制台立即裁剪到新上限。

## 方案

沿用项目已有的 `system_settings` 键值存储，不新增表或环境变量。增加 `console_history_lines` 键和对应的 Store 读写方法：缺失键返回默认值，保存前校验整数边界，非法值不覆盖已有值。

后端 `consoleHub` 保存当前行数上限。创建 Hub 时以 8192 初始化，并在 Server 构造阶段从 Store 读取已保存值。`PUT /api/settings/console` 保存成功后调用 Hub 更新方法；更新方法在同一互斥锁下替换上限并裁剪所有已存在会话历史，后续输出使用新上限。后端原有 1 MiB 字节上限继续先行约束实际历史大小。

前端 App 在认证成功后加载该设置，Terminal 以当前值调用已有缓冲裁剪函数。SettingsPage 使用同一状态的更新回调；保存成功回调 App，Terminal 通过 prop 变化立即裁剪当前文本，不重建 WebSocket。系统设置页面仍可独立加载设置，以覆盖测试注入模式和直接进入设置页的场景。

## API 契约

```text
GET /api/settings/console
200 {"history_lines": 8192}

PUT /api/settings/console
request  {"history_lines": 1000000}
success  {"history_lines": 1000000}
invalid  422 with an error object; stored value is unchanged
```

请求必须是唯一 JSON 对象，不接受未知字段、字符串、浮点数、零、负数或超过 1,000,000 的数值。接口位于现有认证和设置路由组中。

## 数据流与即时生效

```text
系统设置表单
  └─ PUT /api/settings/console
       ├─ Store 校验并保存
       ├─ consoleHub 更新上限并裁剪现有会话历史
       └─ 返回规范化保存值
              └─ App 更新共享行数状态
                     └─ 当前 Terminal 裁剪显示文本
```

新 WebSocket 订阅读取已按最新上限裁剪的历史；现有 WebSocket 不重连，继续接收实时输出但按新上限裁剪。

## 错误处理与兼容性

- Store 读取到损坏或越界的已保存值时返回错误；服务启动时保留安全默认值，设置 GET 返回 500，管理员可以通过修复/删除该设置恢复默认。
- API 请求格式或范围错误返回 422，不修改保存值或会话上限。
- 未配置该键的旧数据库按 8192 处理，不需要数据迁移。
- TXT 导出仍导出当前前端缓存，按钮和文件命名行为不变。

## 测试验收

- Store 默认、保存、重开持久化以及边界/非法值不覆盖已有值。
- API 默认读取、合法更新、越界/错误 JSON 拒绝与已保存值不变。
- 后端缓存使用自定义上限追加并在 Hub 更新后裁剪已有会话。
- 前端设置表单显示 8192、min/max 属性、保存请求和失败恢复；当前打开 Terminal 保存后立即只保留新上限文本。
- 运行相关 Go/Vitest 测试、Go 全量测试、前端全量测试和生产构建。
