# Atomic Tasks

- [ ] Store：新增 `console_history_lines` 整数设置，默认 8192，范围 1～1,000,000。
- [ ] HTTP：新增认证的 `/api/settings/console` GET/PUT，严格 JSON 和边界校验。
- [ ] 后端：Hub 动态读取行数，保存时裁剪现有会话，后续输出使用新值。
- [ ] 前端：App/SettingsPage/Terminal 共享设置，保存后当前显示立即裁剪且不重连 WebSocket。
- [ ] 回归：Go/Vitest/构建通过，TXT 导出及既有控制台协议保持通过。
- [ ] 部署：按 SSH 目标可用性更新安可实例并健康检查；目标未知时保留为明确 blocker。
