# A2S 实例防御日志验证记录

## 已验证

- Helper 使用固定 NFLOG group `100` 接收 IPv4/UDP A2S drop 样本。
- Panel 将样本按游戏端口和 SourceTV 端口写入 `instances/<id>/logs/game/a2s_protect.log`。
- 文件继续由现有游戏日志树、预览、下载、保留天数和大小限制管理。
- 设置页显示 `当前封禁 IP`，其值来自 `xt_recent` 条目数量；`黑名单丢弃` 是命中黑名单后丢弃的包数，不是 IP 数量。
- 端口总限速计数未展示，因为当前策略没有生成对应的总限速规则。

## 测试证据

- `go test ./internal/a2sdefense ./cmd/a2s-defense-helper -count=1`
- `go test ./internal/httpapi -run TestGameLogsHTTPContract -count=1`
- `npm test -- --run A2SDefenseSettings.test.tsx`
- `npx playwright test -g "game log browser shows sampled A2S defense log" --project=desktop`
- `npx playwright test -g "game log browser shows sampled A2S defense log" --project=mobile`

## 语义边界

NFLOG 和事件环是采样、短暂的运行证据，不是完整审计或包捕获。Helper 重启、事件环覆盖、Panel 停止或文件写入失败都不会改变终端防火墙 DROP 行为。
