# Accelerator Crash Report Receiver Design

- 状态：已批准（2026-08-09 对话确认）
- 范围：在现有 Go Panel 内提供 Accelerator 兼容接收端
- 默认保留：90 天，可配置

## 1. 目标

保留官方 SourceMod Accelerator 扩展，通过 Panel 自托管接收崩溃报告，不依赖 Throttle。首版必须能接收 Accelerator 的预提交请求、minidump 和 metadata，并让已认证的 Panel 管理员查看报告信息和下载原始文件。

## 2. 兼容边界

公开 HTTP 端点为：

- `POST /submit`
- `POST /symbols/submit`
- `POST /binary/submit`

`/submit` 支持两种请求：

1. 预提交请求携带 `CrashSignature`、`UserID`、`ExtensionVersion` 和 `ServerID` 等普通字段。Panel 返回 `Y|<N...>|<token>`，其中每个模块都返回 `N`，表示接收 minidump 但不要求上传符号或模块二进制。
2. 上传请求使用 `multipart/form-data`，接受 `upload_file_minidump`、`upload_file_metadata`、`GameDirectory`、`ExtensionVersion`、`ServerID`、`UserID` 和 `PresubmitToken`。成功响应为文本 `OK|<crash-id>`。

`/symbols/submit` 只接收 Breakpad symbol 文本并保存到受控符号目录；由于预提交默认返回 `N`，官方 Accelerator 不会自动触发该端点。`/binary/submit` 明确拒绝，避免服务器被动接收任意游戏模块二进制。

接收端使用 `L4D2_PANEL_CRASH_REPORT_TOKEN` 作为共享 token。Token 通过 Accelerator URL 的 query 参数发送，比较使用常量时间比较；未配置 token 时接收端不可用。服务端不信任 `UserID` 作为认证身份。

## 3. 存储与保留

崩溃报告与实例 Overlay、游戏日志解耦，存储在：

```text
<data-root>/panel/crash-dumps/<crash-id>/
|-- report.json
|-- minidump.dmp
`-- metadata.txt
```

`crash-id` 使用内容哈希生成，重复上传同一个 minidump 更新同一报告而不生成无限副本。manifest 保存接收时间、文件大小、SHA-256、ServerID、GameDirectory、ExtensionVersion、UserID、CrashSignature 和 PresubmitToken 关联信息。报告目录使用受限权限，下载只经过已认证 Panel API。

默认保留 90 天，通过 `L4D2_PANEL_CRASH_RETENTION_DAYS` 配置，要求为正整数。Panel 启动时清理过期完成报告，并每天执行一次同样的清理；预提交 token 只保留 24 小时。清理不跟随符号目录，也不删除正在写入的报告。

请求体、minidump、metadata 和 symbol 文件分别有硬上限。文件名只用于日志之外的字段，不参与路径拼接；所有落盘路径由服务器生成。

## 4. Panel API

在现有认证路由内新增：

- `GET /api/crash-reports`：按接收时间倒序列出报告摘要。
- `GET /api/crash-reports/{id}`：读取 manifest 和可用文件信息。
- `GET /api/crash-reports/{id}/download?file=minidump|metadata`：下载原始文件。

首版不新增前端页面，API 先作为稳定面板集成边界；崩溃解析、线程栈展示、Crash Signature 聚合和删除操作留到后续切片。

## 5. 安全与故障处理

- 公开接收端只允许 `POST`，强制 token、请求体上限和 multipart 文件上限。
- 预提交失败不得创建大文件；上传失败删除临时文件，不留下半成品报告。
- 元数据按原文保存但不在未认证接口返回；日志不得记录 token、命令行或文件内容。
- 重复上传保持幂等；同一报告的新 metadata 原子替换 manifest 关联文件。
- 磁盘或解析错误返回 5xx，让 Accelerator 保留本地 dump 以便重试。
- 未安装 `minidump_stackwalk` 不阻止接收；首版只保存原始数据和 Accelerator 提供的 CrashSignature。

## 6. 测试与验收

- 单元测试覆盖预提交响应格式、模块数量、token、multipart 字段、大小限制、去重、原子写入、路径安全和 90 天清理。
- HTTP 测试覆盖公开接收端的未认证 token 边界以及认证后的列表、详情、下载。
- 使用 `D:\Windows\Download\crash_rmvtruf2n5tk.dmp` 做一次真实 multipart 上传，验证返回 crash ID、文件哈希和 Panel 下载内容一致。
- 通过 Go 测试、`go vet`、Compose 配置检查和 `git diff --check` 验证部署边界。

## 7. 设计输入与影响边界

### TaskIntentDraft

目标是以最小新增运行时组件兼容 Accelerator 上传协议，支持管理员诊断，同时避免 Throttle、任意二进制上传和游戏容器访问 Panel 崩溃存储。

### BaselineReadSetHint

设计参考 `CONTEXT.md`、`docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`、`internal/httpapi/server.go`、`internal/store`、`internal/config/config.go`、`cmd/panel/main.go`、`docker-compose.yml` 以及现有游戏日志持久化边界。官方协议字段与响应格式参考 `asherkin/accelerator` 的 `extension/extension.cpp`。

### ImpactStatementDraft

新增 `internal/crashreports` 文件存储与协议处理、Panel 公开接收路由、认证查询 API、配置项和周期清理。现有实例生命周期、Overlay、游戏日志 API、认证会话和 Docker 游戏挂载保持不变。首版不引入新的容器能力、网络服务、数据库迁移或前端页面。

