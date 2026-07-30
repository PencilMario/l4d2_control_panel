# 未引用 GitHub 插件包清理设计

## 目标

扩展现有 `cleanup` 计划任务，清理本地内容仓库中不再被任何游戏实例引用、且不是对应 GitHub 仓库最新版本的具体插件包，避免历史 Release 包无限占用磁盘。

本功能只管理 Panel 本地的 `packages/releases/`，不会删除或修改远端 GitHub Releases。

## 术语与判定

- **GitHub Release 插件包**：`PackageVersion.SourceRepository` 非空的具体插件包。
- **常规插件包**：`SourceRepository` 为空的手动上传插件包。
- **引用**：任一游戏实例的 `SelectedPackageID` 或 `PackageVersion` 等于该具体包 ID。前者保护固定包选择，后者保护最后一次成功部署的版本。
- **仓库最新版**：同一 `SourceRepository` 下 `CreatedAt` 最大的包。时间相同时使用包 ID 作稳定决胜，确保每次扫描结果一致。
- **可清理包**：GitHub Release 插件包，且既未被引用，也不是其仓库最新版。

所有常规插件包均不在本功能的清理范围内，即使没有实例引用。

## 架构与职责

清理编排仍由 `internal/maintenance` 持有。它从实例仓库读取全部实例，构造由 `SelectedPackageID` 和 `PackageVersion` 组成的受保护 ID 集合，再把集合交给 `internal/content.PackageManager`。

`PackageManager` 持有包文件布局与元数据解析职责：列出具体包、按仓库确定最新版、筛选可清理项并删除对应文件。实例引用的含义不下沉到 `PackageManager`；调用方只传入已经解析好的受保护 ID 集合。

`cmd/panel` 在组装维护服务时注入实例读取能力与现有 `PackageManager`。计划任务类型、Cron 配置和 HTTP API 保持不变。

## 清理流程

1. 按现有逻辑清理超过保留期的实例备份和上传临时文件。
2. 读取全部实例；读取失败时返回错误，不执行插件包删除。
3. 收集非空的 `SelectedPackageID` 与 `PackageVersion`。
4. 列出所有插件包，并按 `SourceRepository` 计算各仓库最新版。
5. 跳过常规插件包、受保护包和各仓库最新版。
6. 删除其余 GitHub Release 插件包的归档与元数据。
7. 在任务日志中逐项记录包 ID、仓库、版本、删除结果和释放字节，最终记录文件清理与插件包清理汇总。

GitHub 历史包不受 `retention_days` 影响；只要满足可清理判定，就会在下一次 `cleanup` 中删除。

## 删除一致性与错误处理

一个具体包由 `<id>.zip` 和 `<id>.json` 组成。删除时先删除归档，再删除元数据，使中途失败的包仍可由元数据在下一次扫描中被发现并重试。若归档不存在，按已删除处理；若其他归档删除错误发生，则保留元数据并报告错误。归档删除成功但元数据删除失败时，报告错误并保留可观察的损坏元数据，下一次清理可重试元数据删除。

单个包删除失败不阻止扫描其他候选包。任务结束时汇总所有删除错误并返回组合错误，使后台任务显示失败而不是误报完整成功。成功计数只包含归档与元数据均已删除的包；释放空间按实际成功删除的归档大小统计。

上下文取消后停止继续删除，并返回取消错误。

## 日志与可观测性

每个候选包记录安全的逻辑字段：包 ID、`SourceRepository`、版本、归档文件名与大小。不得记录数据根目录、GitHub Token、下载 URL 或其他凭据。

最终汇总至少包含：扫描包数、保留最新版数、保留引用数、删除包数、释放字节数和失败数。现有备份及临时文件清理日志字段继续保留。

## 测试标准

自动化测试必须证明：

- 同一 GitHub 仓库只保护按规则确定的最新版。
- `SelectedPackageID` 引用的旧包保留。
- `PackageVersion` 引用的旧包保留。
- 未引用且非最新版的 GitHub 包，其 `.zip` 与 `.json` 均被删除。
- 不同 GitHub 仓库各自保留一个最新版。
- 所有常规插件包均保留。
- 实例读取失败时不删除任何插件包。
- 单包删除失败会继续处理其他候选项、记录失败并令任务返回错误。
- 取消上下文会停止后续删除。
- 日志不泄露本地绝对根路径。
- 原有备份与上传临时文件保留期清理测试继续通过。

## 兼容边界与非目标

- 不改变计划任务类型、计划任务 payload、HTTP API 或前端操作流程。
- 不改变 GitHub Release 同步与实例完整更新行为。
- 不清理任何常规插件包。
- 不删除远端 GitHub Releases 或其附件。
- 不提供版本锁定、Release 回退或新的保留数量/保留天数设置。
- 不依赖实例的 `PackageSourceID` 判断具体包引用；具体已部署包以 `PackageVersion` 为准。

## 设计输入

**TaskIntentDraft**：通过现有清理任务删除所有未引用且不是其 GitHub 仓库最新版的本地 Release 插件包，同时保护常规插件包。

**BaselineReadSetHint**：`CONTEXT.md` 定义插件来源与已部署插件包的区别；`internal/content/packages.go` 定义具体包元数据和本地布局；`internal/maintenance/manager.go` 定义现有清理所有权；`internal/domain/models.go` 与实例存储定义引用字段。

**ImpactStatementDraft**：影响维护编排、内容包文件操作、实例只读查询及启动组装。必须保持现有计划任务/API 兼容，且任何引用解析失败都应阻止插件包删除。
