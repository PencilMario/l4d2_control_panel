# 实例启动配置与插件包设计

状态：已批准  
日期：2026-07-15

## 1. 目标

每个 L4D2 实例独立拥有一套可编辑的 SRCDS 启动配置和一个选定的插件包。管理员在创建实例和编辑已有实例时都能配置这些值，并在提交前预览最终的 `srcds_run` 启动指令。

首次启动不得先运行纯净 SRCDS。Panel 必须先安装游戏、部署该实例选定的插件包、应用私有覆盖层，再创建并启动游戏容器。

## 2. 现状与基线

本设计受以下现有契约约束：

- `Instance.ExtraArgs` 已持久化为 `instances.extra_args`，Docker 容器通过 `SRCDS_EXTRA_ARGS` 把它交给 Supervisor。
- Supervisor 当前使用 `shlex.split` 将额外启动项拆成 argv，并追加在 Panel 管理的参数之后。
- 更新接口会在插件包成功部署后把包 ID 写入 `Instance.PackageVersion`。
- 插件包文件存放在共享仓库，但部署目标始终是 `instances/<id>/game/`，因此实际内容所有权是实例级的。
- 修改容器环境中的启动配置需要重建游戏容器；持久化的游戏、私有覆盖和备份目录不得被删除。
- 现有插件包流水线负责归档检查、完整/热更新、私有覆盖、日志化事务和回滚，首次部署不得绕过它。

参考基线：

- `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`
- `internal/domain/models.go`
- `internal/httpapi/server.go`
- `internal/lifecycle/service.go`
- `internal/docker/lifecycle.go`
- `internal/updates/pipeline.go`
- `runtime/supervisor.py`
- `web/src/app/App.tsx`

## 3. 术语与状态

- **选定插件包**：管理员希望该实例使用的插件包 ID。创建实例时写入，重配置时可以改变。
- **已部署插件包**：最后一次成功提交到实例游戏目录的插件包 ID。部署失败时不得提前改变。
- **启动配置**：Panel 管理的结构化参数加上管理员提供的额外启动项。
- **启动指令预览**：根据当前表单值渲染的最终 `srcds_run` 指令，参数顺序与 Supervisor 一致。

`PackageVersion` 继续表示已部署插件包，以保持现有更新语义。实例新增 `SelectedPackageID`，API 使用 `package_id` 表示选定插件包，并使用 `applied_package_id` 暴露已部署插件包。数据库采用加法迁移；已有实例的 `SelectedPackageID` 从 `package_version` 回填。

## 4. 实例配置界面

创建实例和编辑实例复用同一套表单组件，避免字段、默认值、预览和提交契约漂移。

表单包含：

- 名称。
- 游戏端口。
- 可选 SourceTV 端口。
- 插件监听端口列表。
- 启动地图。
- 游戏模式。
- Tickrate。
- 最大玩家数。
- 实例插件包。
- 额外 SRCDS 启动项。

游戏模式和插件包使用选项控件。端口、地图、Tickrate、玩家数和额外启动项以实例当前值或新建默认值填充，并允许编辑。

以下启动前缀由运行环境固定，不允许通过表单删除或替换：

```text
./srcds_run -game left4dead2 -console
```

其余结构化参数均可编辑。最终参数顺序为：

```text
./srcds_run
-game left4dead2
-console
-port <game-port>
-tickrate <tickrate>
+map <start-map>
+mp_gamemode <game-mode>
-maxplayers <max-players>
[+tv_enable 1 +tv_port <sourcetv-port>]
<extra-args...>
```

插件监听端口是 Panel 与插件间的实例声明，通过 `L4D2_PLUGIN_PORTS` 传入容器，不伪装成 SRCDS 原生命令行参数，因此不出现在启动指令预览中。

每张实例卡片提供明确命名的配置按钮。卡片显示选定插件包；当选定包与已部署包不同，显示待应用状态。运行中实例保存会进入重配置任务，按钮文案明确表示会重建或更新实例。

## 5. 启动项解析与预览

额外启动项是 argv 的追加部分，不通过 Shell 执行，不支持管道、重定向、命令替换或环境变量展开。

API 在创建和更新时验证额外启动项能按 Supervisor 接受的 shell-like quoting 规则拆分。空字符串表示没有额外参数。格式错误返回 `422 invalid_instance`，不得等到容器启动后才失败。

预览随表单变化实时更新，并保留管理员输入中的引号语义。提交后的容器环境与预览使用相同的结构化值和额外参数顺序。重复提供 Panel 已管理的参数不会被静默改写；实现应拒绝会使端口、地图、模式、Tickrate、玩家数或 SourceTV 声明与 Panel 状态失真的冲突参数。

## 6. 插件包选择

新建实例必须选择仓库中已经存在的插件包。没有可用插件包时，实例创建不可提交。API 必须验证 `package_id` 存在，不能只信任浏览器选项。

每个实例独立存储 `SelectedPackageID`。多个实例可以选择同一插件包，也可以分别选择不同插件包。切换一个实例的插件包不得修改其他实例的选择、游戏目录、清单或运行状态。

插件包被选中不等于已经部署：

- 新建实例：`SelectedPackageID` 有值，`PackageVersion` 为空。
- 首次部署成功：两者相同。
- 已有实例切换包：先更新 `SelectedPackageID`，成功提交部署后才更新 `PackageVersion`。
- 部署失败：保留新的选择以便重试，`PackageVersion` 仍指向最后成功部署的包。

## 7. 首次启动编排

首次启动由 Panel 的持久 Job 执行以下顺序：

1. 重新读取实例和选定插件包，验证包仍存在。
2. 检查数据目录空间、声明端口和同实例维护容器占用。
3. 创建实例持久目录并将实际状态标记为 `installing`。
4. 使用受限维护容器执行匿名 SteamCMD 首装序列：先 Windows 平台引导 App 222860，再切换 Linux 平台完成首装；首装阶段不执行 `validate`，只有后续更新或完整性验证才使用 `validate`。
5. 使用现有插件包 Pipeline 对实例执行完整部署并应用私有覆盖层。
6. 提交部署事务并把 `PackageVersion` 更新为 `SelectedPackageID`。
7. 根据最新启动配置创建 Host 网络游戏容器。
8. 启动 SRCDS，完成健康检查后标记实例为 `running`。

游戏容器不得承担插件归档解压职责，也不得在第 5 步完成前启动 SRCDS。SteamCMD 首装维护容器继续使用固定命令、受限标签、非特权用户和仅游戏目录挂载。

失败处理：

- SteamCMD 失败时不创建游戏容器，实例标记为 `faulted`，选定包保留。
- 插件部署失败时使用现有事务回滚，不更新 `PackageVersion`，也不创建游戏容器。
- 游戏容器创建、启动或健康检查失败时保留已安装游戏和已提交插件内容，实例标记为 `faulted`，允许重试启动。
- 中断后由现有 Job 恢复和插件部署日志恢复机制保留可诊断状态；遗留维护容器仍按当前认领规则处理。

## 8. 已有实例重配置

更新请求同时接受启动配置和 `package_id`。后端在写入前比较旧配置，生成一个持久的实例重配置 Job：

- 只有显示名称改变：直接保存，不重建。
- 容器启动配置改变：保留数据目录并重建游戏容器。
- 插件包改变：执行完整插件更新；热更新仍保留在内容仓库页面作为管理员显式操作，不用于配置切换。
- 启动配置和插件包同时改变：在同一 Job 中顺序执行所需操作，禁止创建并发的重建与插件更新 Job。

重配置必须保留实例原有的期望运行状态。原本运行的实例在成功后恢复运行；原本停止的实例保持停止。插件部署成功前不更新 `PackageVersion`。任务失败时保留诊断记录，且不得把未成功部署的包标记为已应用。

## 9. API 契约

创建和更新实例请求增加：

```json
{
  "extra_args": "-strictportbind +sv_lan 0",
  "package_id": "package-uuid"
}
```

实例响应增加稳定的小写字段：

```json
{
  "extra_args": "-strictportbind +sv_lan 0",
  "package_id": "selected-package-uuid",
  "applied_package_id": "deployed-package-uuid"
}
```

创建成功仍返回 `201`。无运行时变更的实例更新返回 `200`；需要容器重建或插件切换时返回 `202` Job。严格 JSON 解码继续拒绝未知字段。

## 10. 测试与验收

后端测试覆盖：

- 创建实例接受、验证并持久化额外启动项和选定插件包。
- 不存在的插件包、无效启动项 quoting 和冲突的 Panel 管理参数被拒绝。
- 数据库迁移为已有实例回填选定插件包，且不同实例可保存不同包。
- Docker 容器环境包含实例自己的额外启动项；Supervisor 生成的 argv 顺序与预览契约一致。
- 首次启动严格执行 SteamCMD、插件部署、私有覆盖、容器创建和 SRCDS 启动顺序。
- 首装或插件部署失败时不启动 SRCDS，且已部署包字段不被提前更新。
- 已有实例切换插件包只修改目标实例，并保持原期望运行状态。
- 同一实例的重配置不能与维护写入并发执行。

前端测试覆盖：

- 新建表单加载插件包，提交 `extra_args` 和 `package_id`。
- 编辑表单回填所有默认值、当前启动项和该实例的插件包。
- 修改字段会实时更新完整启动指令预览。
- 保存已有实例能处理 `200` 实例响应和 `202` Job 响应。
- 卡片显示插件包及待应用状态。

浏览器主路径验收：上传至少两个插件包，分别创建两个实例并选择不同包，配置不同额外启动项，确认预览、持久化、首次启动 Job 和刷新后的实例归属均正确。

## 11. 兼容边界与非目标

兼容边界：

- 保留现有实例数据目录、插件部署清单和 `package_version` 数据。
- 已有但尚未部署插件包的实例记录继续可读、可编辑；再次启动前必须选择一个有效插件包。
- 空 `extra_args` 的实例保持现有 SRCDS 启动行为。
- 现有内容仓库热更新和完整更新入口继续可用。
- 不允许客户端指定 Docker Exec、容器入口点或任意 Shell 命令。

非目标：

- 不支持为同一实例同时叠加多个插件包。
- 不自动把插件端口转换成任意 SRCDS 参数。
- 不提供任意运行镜像、Docker 命令或 Supervisor 固定前缀编辑。
- 不在本任务中重设计插件仓库上传、GitHub Release 获取或计划任务模型。

## 12. 工作草案摘要

`TaskIntentDraft`：为创建和已有实例提供独立、可预览的启动配置，并把实例插件包选择接入首次安装与后续重配置。

`BaselineReadSetHint`：实例模型与 SQLite、HTTP 创建/更新、Docker 容器规格、生命周期、SteamCMD 维护容器、插件 Pipeline、Supervisor 和 React 实例表单共同拥有该行为。

`ImpactStatementDraft`：改动跨越 API、持久化、生命周期、Docker 维护任务、更新协调器和实例 UI。必须保持 Host 网络、持久目录、固定 PTY/Supervisor、安全归档部署、回滚和期望运行状态不变。
