# 新建实例 SRCDS 默认启动项设计

状态：待书面复核
日期：2026-07-15

## 1. 目标

调整新建实例表单的 SRCDS 默认启动配置，使新实例默认以对抗模式、32 人上限启动，并预填没有独立表单选项的通用启动参数。

本设计仅改变后续新建实例的表单默认值。已有实例继续使用数据库中保存的启动配置，不自动补齐或迁移任何参数。

## 2. 参数归属

继续由 Panel 的现有结构化选项管理：

- `-game left4dead2`：Panel 固定值。
- `-port <game-port>`：游戏端口，默认 `27015`；用户要求中的 `$PORT` 对应此字段。
- `-tickrate <tickrate>`：默认 `100`。
- `+map <start-map>`：默认 `c2m1_highway`。
- `+mp_gamemode <game-mode>`：默认 `versus`。
- `-maxplayers <max-players>`：默认 `32`。

以下没有独立选项的参数写入“额外 SRCDS 启动项”的新建默认值：

```text
-sv_lan 0 -ip 0.0.0.0 +sv_clockcorrection_msecs 25 -timeout 10 +sv_setmax 32 +servercfgfile server.cfg
```

现有固定 `-console` 参数继续保留。额外启动项仍由现有解析器验证并作为 argv 追加，不经 Shell 执行。

## 3. 新建实例预览

在默认游戏端口下，新建表单的启动指令预览应为：

```text
./srcds_run -game left4dead2 -console -port 27015 -tickrate 100 +map c2m1_highway +mp_gamemode versus -maxplayers 32 -sv_lan 0 -ip 0.0.0.0 +sv_clockcorrection_msecs 25 -timeout 10 +sv_setmax 32 +servercfgfile server.cfg
```

用户修改游戏端口、地图、模式、Tickrate、玩家上限或额外启动项后，预览继续按现有规则实时更新。

## 4. 方案选择

采用仅修改 React 新建表单默认值的方案。该位置是当前新建实例默认值的唯一所有者，提交后这些值按现有 API 契约持久化。

不采用 API 缺省注入，因为它会改变非浏览器客户端的请求语义；不迁移已有实例，因为这会导致既有服务器重启后行为变化。

## 5. 测试与验收

- 组件测试证明新建表单显示 `versus`、`100`、`32` 和指定额外启动项。
- 组件测试证明默认启动指令预览与本设计一致。
- 编辑已有实例时继续回填该实例保存的值，不套用新建默认值。
- 运行现有前端测试和构建，确认启动配置编辑与预览没有回归。

## 6. 兼容边界与非目标

- 不修改已有实例记录，包括 `extra_args` 为空的实例。
- 不修改 API、数据库、Docker 环境变量或 Supervisor 的缺省值。
- 不改变 Panel 管理参数与额外启动项的冲突校验。
- 不新增启动项表单控件。

## 7. 工作草案摘要

`TaskIntentDraft`：把用户给出的启动串映射到现有结构化字段和额外启动项，仅作为新建实例默认值。

`BaselineReadSetHint`：`InstanceConfigModal.tsx` 是新建默认值与预览所有者；现有实例值由 API 持久化并在编辑时直接回填；`internal/srcds` 和 Supervisor 定义运行时参数顺序与冲突边界。

`ImpactStatementDraft`：影响新建实例的前端初始状态、预览和提交数据。已有实例、后端契约和运行时兼容路径保持不变。
