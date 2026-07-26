# GitHub 插件源与 VPK 上传后重装实施计划

## 范围

实现已批准的方案：游戏实例绑定 GitHub 源仓库；部署时解析该源当前唯一最新插件包；GitHub 源新包入库后删除旧包；VPK 上传完成后为所有实例排队一次等待空服后的插件重装。

## 基线与兼容边界

- 项目术语以 `CONTEXT.md` 为准，实例仍保留期望状态/实际状态语义。
- 现有按 `SelectedPackageID` 固定包的接口和数据需兼容已有实例；迁移后旧固定包配置仍可读取。
- 现有 `internal/vpkrestart` 负责实例空服等待与重启协调，不新增第二套等待机制。
- 不改变 GitHub 源 CRUD 的公共字段语义；源仓库字符串作为插件包归属键。

## 文件职责

- `internal/domain/models.go`：增加实例 GitHub 源选择字段。
- `internal/content/packages.go`：按源取最新包、源包替换/清理。
- `internal/content/packages_test.go`：覆盖源包唯一保留与最新查询。
- `internal/updates/*.go`：把部署/重装的包解析从固定 ID 扩展为源优先、旧 ID 兼容。
- `internal/httpapi/server.go` 及测试：实例配置输入、上传完成触发全实例排队。
- 持久化 store/migration 文件：增加实例字段并保持旧数据迁移。
- `web/src/api/client.ts`、`web/src/app/InstanceConfigModal.tsx`、相关测试：显示和提交 GitHub 源。

## 原子步骤

1. 为包管理器写源包保留与最新查询失败测试。
2. 为实例配置写 GitHub 源字段 JSON/API 回归测试。
3. 为上传完成写全实例 VPK 重装排队测试。
4. 实现包管理器源归属、删除旧包和最新解析。
5. 实现部署/重装源优先与旧固定 ID 兼容。
6. 实现实例持久化、校验、迁移和上传回调。
7. 接入前端源选择并更新类型映射。
8. 运行 Go/前端定向测试及全量测试。

## 风险与回滚

- 删除旧 GitHub 包前必须完成新包元数据落盘；删除失败应保留新包并返回可诊断错误。
- 正在使用旧版本的运行实例不直接中断；仅在后续部署或 VPK 上传后的空服重装时切换最新包。
- 未绑定 GitHub 源的旧实例继续使用原 `SelectedPackageID`。
