# Accelerator 管理、崩溃在线查看与 AI 分析设计

- 状态：待用户审核
- 日期：2026-08-10
- 基础设计：[2026-08-09 Accelerator 接收端设计](2026-08-09-accelerator-crash-report-receiver-design.md)

## 1. 目标与边界

在现有 Go Panel 的 Accelerator 兼容接收端之上，增加以下能力：

1. 在实例设置中启用或禁用 Accelerator，并由 Panel 独立管理安装文件。
2. 使用可配置的 HTTPS 下载 URL 获取 Accelerator 包，不锁定官方仓库或版本；可选使用已有 GitHub Release 代理加速 GitHub 下载。
3. 安装时修改 SourceMod `addons/sourcemod/configs/core.cfg`，让预提交、符号和二进制上传都指向本 Panel 的本地地址。
4. 插件包安装、重装、更新和容器重建时，重新确保已启用实例的 Accelerator 与配置存在。
5. 完整接收 Accelerator 的 dump、metadata、Breakpad symbol 和模块二进制上传协议。
6. 在面板中查看崩溃报告的 metadata、模块、stackwalk 和 AI 分析结果，并下载原始文件。
7. 通过可配置的 OpenAI-compatible 接口进行异步 AI 分析；原始 dump、模块二进制和未脱敏 metadata 永远不发送给 AI。
8. 内置可确认许可的 SourceMod、Metamod 符号；不预置或分发 L4D2/Valve 游戏符号。

保留现有约束：公开 Accelerator 路径只接受 loopback 来源、共享 token 和本项目 managed instance 的 `server-id.txt`；报告默认保留 90 天并可配置；不依赖 Throttle。

### 非目标

- 不把 Accelerator 混入常规插件包的所有权或版本生命周期。
- 不提供 Valve/L4D2 二进制或符号的预置下载。
- 不把 raw `.dmp`、模块二进制或原始 metadata 发送到 AI 服务。
- 不在浏览器中直接渲染二进制 dump；原始 dump 只提供认证下载。
- 不为修改下载 URL 自动重启所有实例；新来源在下次启用、重装或更新时生效。
- 不保留 Throttle fallback 或把公开请求转发给 Throttle。

## 2. 已确认的产品决策

### 2.1 Accelerator 来源

Accelerator 来源是全局系统设置，实例只保存能力开关：

- `download_url`：直接 HTTPS 下载地址，必填才允许启用新实例的 Accelerator。
- `use_github_proxy`：是否对 GitHub Release URL 使用现有全局 GitHub 下载加速地址。仅当 URL 的主机是 GitHub 时生效，其他地址保持原 URL。
- Panel 不依赖仓库名或版本号；fork 只需提供自己的包 URL。

官方默认值可以指向当前公开 Linux 构建地址，但该值必须可由管理员修改或清空。下载结果按 SHA-256 缓存，下载临时文件和解压目录不能直接作为实例文件使用。

安装器只允许归档中的 SourceMod 相关路径，拒绝绝对路径、路径穿越、符号链接和未知目标路径。安装前验证归档包含 Accelerator autoload、扩展二进制和 gamedata；缺失或架构不匹配时任务失败且不替换旧安装。

### 2.2 实例开关与生命周期

实例配置增加：

- `accelerator_enabled`：是否由 Panel 管理 Accelerator。
- `auto_crash_analysis`：收到该实例的新报告后是否自动排队 AI 分析。

保存实例设置时：

- 实例停止或未安装：只执行安装/卸载和配置补丁，不改变期望运行状态。
- 实例运行中：创建现有异步重配置任务，按停止、部署、配置、重建或重新创建容器、恢复运行的流程执行。
- 禁用时移除 Panel manifest 中仍由 Panel 拥有的文件，并恢复 Panel 之前修改的配置值；发现文件被外部修改时不强制删除，任务报告冲突。

所有选择的插件包完整安装、GitHub 源插件重装、插件更新和容器重建任务，在其常规部署完成后调用 Accelerator `Ensure`。启用能力的实例若无法下载、校验、解压、部署或配置 Accelerator，任务失败；不会静默留下“已启用但未安装”的状态。

### 2.3 SourceMod 配置补丁

上游 Accelerator 读取 SourceMod Core 配置，不读取独立的 `accelerator.cfg`。Panel 对 `addons/sourcemod/configs/core.cfg` 做 KeyValues 定点补丁，只修改以下键：

```text
MinidumpUrl             http://127.0.0.1:<panel-port>/submit?token=<token>
MinidumpSymbolUrl      http://127.0.0.1:<panel-port>/symbols/submit?token=<token>
MinidumpBinaryUrl      http://127.0.0.1:<panel-port>/binary/submit?token=<token>
MinidumpPresubmit      yes
MinidumpSymbolUpload   3
MinidumpBinaryUpload   yes
```

Panel 必须保留其他 Core 配置和格式无关的用户键值。每个实例保存 Accelerator manifest，记录修改前值、修改后值和修改后文件 hash：

- 重装时只更新这些键，并重新记录 hash。
- 禁用时仅当值仍是 Panel 写入的值时恢复旧值。
- 如果管理员或插件修改过同一键，禁用不覆盖新值，而是保留该值并报告冲突。
- token 为空时不能生成有效安装配置，启用任务返回明确错误；不生成指向 Throttle 的默认地址。

配置中的 `127.0.0.1` 依赖现有 Panel 和游戏容器的 host network。公开 receiver 仍检查实际 TCP 来源和 managed instance，不能因为 URL 是 loopback 就跳过授权。

### 2.4 完整 Accelerator 上传协议

保留并扩展三个公开端点：

- `POST /submit`
- `POST /symbols/submit`
- `POST /binary/submit`

三者都要求共享 token、本机来源、合法请求大小和能匹配 `server-id.txt` 的 `ServerID`。上传者提供的 `UserID`、文件名和 `code_file` 只作为元数据，不能用于授权或路径拼接。

预提交请求中的 `CrashSignature` 使用 Accelerator v2 的模块记录解析出平台、架构、模块 debug identifier 和模块列表。每个模块返回一个决策字符：

| 条件 | 决策 |
| --- | --- |
| 已有内置或已上传的匹配 SourceMod/Metamod symbol | `N` |
| Linux 模块缺少 symbol | `Y`，请求 symbol 文件 |
| Windows 模块缺少二进制且二进制上传允许 | `U`，请求模块二进制 |
| 已有对应 artifact 或无法识别的模块 | `N` |

`Y`、`U`、`N` 是 Accelerator 兼容协议的真实语义；不能把所有模块无条件标为 `U`，因为每个模块只有一个决策字符。`/symbols/submit` 和 `/binary/submit` 都必须独立可用，既能接收 Accelerator 按预提交触发的请求，也能接收人工或其他兼容客户端的合法请求。

符号和二进制都按内容寻址保存。symbol manifest 至少记录 debug identifier、code identifier、平台、架构、来源实例、接收时间和内容 hash。二进制 manifest 记录相同的标识和脱敏 basename；绝不以客户端路径创建文件。

### 2.5 存储与保留

Panel 数据根目录使用以下布局：

```text
panel/
|-- accelerator-cache/
|   |-- <archive-sha256>.zip
|   `-- <archive-sha256>/...
|-- crash-dumps/
|   |-- reports/<crash-id>/
|   |   |-- report.json
|   |   |-- minidump.dmp
|   |   |-- metadata.txt
|   |   |-- stackwalk.txt       # 成功生成后
|   |   `-- ai-analysis.md      # 成功生成后
|   |-- symbols/
|   |   |-- builtin/sourcemod/...
|   |   |-- builtin/metamod/...
|   |   `-- uploaded/...
|   `-- binaries/...
`-- ...

instances/<id>/
|-- accelerator-manifest.json
`-- game/left4dead2/...          # Panel 管理的安装目标
```

报告目录仍由 minidump SHA-256 标识并幂等更新。报告 manifest 新增实例关联、模块 artifact 引用、stackwalk 状态和 AI 状态。报告目录及其派生文件默认保留 90 天，使用现有 retention 配置清理；清理必须包含报告引用的二进制对象。内置符号不因报告过期删除；上传符号和未引用二进制采用内容寻址，只有无存活报告引用且超过清理宽限期时才回收。

所有接收内容使用硬上限、临时文件、原子 rename 和受限权限。二进制上传使用独立上限，不得通过一个请求绕过 minidump 或总请求限制。建议初始上限为单个二进制 256 MiB、单次请求 512 MiB，实际值必须在配置和测试中统一。

### 2.6 Stackwalk

Panel 在服务端运行本地 `minidump_stackwalk`，不调用外部崩溃服务：

1. 报告接收成功后立即返回 Accelerator 响应，stackwalk 放入异步分析队列。
2. worker 读取 Panel 内的 dump、symbol 和 binary artifact，在超时和输出上限内运行 stackwalk。
3. 成功结果原子写入 `stackwalk.txt`，同时保存工具版本、符号覆盖摘要和生成时间。
4. 缺少工具、匹配符号或解析失败不影响报告保存；详情页展示状态、错误摘要和原始文件下载。
5. Panel 镜像提供默认工具路径，并允许受控配置覆盖路径；工具进程以 Panel 用户运行，不继承请求中的 shell 参数。

内置符号只包含经过许可确认的 SourceMod 和 Metamod 版本。实现阶段从公开运行时包或公开构建产物生成/取得 Breakpad symbol，并在仓库中保留来源 URL、许可证说明和 SHA-256 manifest。未找到匹配符号时不伪造函数名，也不把 L4D2/Valve 文件打包进 Panel。

### 2.7 AI 分析

AI 配置为全局设置：

- OpenAI-compatible endpoint。
- model 名称。
- API key 存在现有 encrypted secrets，API 返回只显示是否已配置。

endpoint 支持 OpenAI、Ollama、vLLM 等兼容实现。允许本机 HTTP endpoint；非 loopback 的 HTTP 地址直接拒绝，必须使用 HTTPS。请求使用固定超时、最大响应大小和异步重试上限。

发送给 AI 的内容只包括 Panel 本地生成的脱敏结构：

- Crash Signature 的平台、架构、错误类型、模块名和 symbol coverage。
- 脱敏后的 stackwalk 文本中的函数、模块、偏移和线程结构。
- 从 metadata 提取并脱敏后的环境摘要。
- Accelerator、SourceMod 和游戏运行时的版本字段。

原始 metadata、命令行、绝对路径、IP、ServerID、UserID、token、完整 stackwalk、dump、binary 和 symbol 文件不得原样出站。发送前对 metadata 和 stackwalk 做结构化脱敏并生成输入 hash，manifest 保存 hash、模型、开始/结束时间、状态和错误，不保存 API key。模型输出按不可信文本展示，不作为 Panel 操作指令执行。

自动策略：

- 默认只在详情页手动点击“AI 分析”。
- 实例 `auto_crash_analysis` 开启后，新报告自动排队分析。
- 没有 AI endpoint/model 时不反复重试，详情显示“AI 未配置”；手动配置后可以重新触发。
- AI 失败不影响 dump、metadata、stackwalk 和下载。

### 2.8 Panel API 与前端

认证 API 保留现有报告接口并扩展：

- `GET /api/crash-reports`：时间倒序列表，支持实例、Crash Signature、分析状态筛选。
- `GET /api/crash-reports/{id}`：manifest、模块和分析状态。
- `GET /api/crash-reports/{id}/download?file=minidump|metadata|stackwalk|ai|binary`：认证下载；当 `file=binary` 时必须同时提供 `artifact=<id>`，且该 ID 必须来自报告 manifest 中的 artifact。
- `POST /api/crash-reports/{id}/analyze`：排队或重试 stackwalk/AI，返回异步 Job。
- `GET/PUT /api/settings/accelerator`：下载 URL 和 GitHub 代理开关。
- `GET/PUT /api/settings/crash-analysis`：endpoint、model 和配置状态；API key 使用独立敏感字段更新，不回显明文。

实例创建和更新 API 的请求/响应增加 `accelerator_enabled` 与 `auto_crash_analysis`。新增全局“崩溃报告”页面：

- 列表显示接收时间、实例、Crash Signature、符号覆盖和 stackwalk/AI 状态。
- 详情按区块显示概要、metadata、模块、stackwalk、AI 结果和下载操作。
- 实例详情只显示最近报告并链接到全局列表的实例筛选。
- loading、空状态、解析失败、AI 未配置、权限错误和重试状态都必须可见。
- 实例设置中加入 Accelerator 和自动 AI 开关；保存时复用现有异步 Job 状态提示。

## 3. 架构选择

### 方案 A：把 Accelerator 当作普通插件包内容

实现简单，但普通包更新会覆盖或删除扩展，无法区分用户插件和 Panel 能力，也无法可靠恢复 `core.cfg`。不采用。

### 方案 B：Panel 独立 capability manager

新增 Accelerator manager 管理下载缓存、归档白名单、实例 manifest、文件安装、Core 配置补丁和卸载。插件部署、实例启停和容器重建通过窄接口调用 `Ensure`，崩溃 receiver/stackwalk/AI 保持独立模块。推荐此方案，因为所有权清楚，能复用现有异步任务、私有文件应用和 host network 边界。

### 方案 C：为每个实例增加独立 sidecar

可隔离安装和解析工具，但会增加容器、网络、卷、版本同步和故障面，也不能避免 SourceMod Core 配置需要修改。不采用。

## 4. 组件与数据流

```text
实例设置保存
    -> reconfigure Job
    -> Accelerator manager 下载/校验/解压
    -> 安装文件 + patch core.cfg + manifest
    -> lifecycle restart/rebuild（运行中实例）

Accelerator
    -> /submit 预提交
    <- Y|N/Y/U decisions|token
    -> /submit multipart dump + metadata
    -> /symbols/submit 或 /binary/submit
    -> crashreports Manager 内容寻址存储
    -> analysis queue
        -> minidump_stackwalk + symbols/binaries
        -> 脱敏摘要 + stackwalk
        -> OpenAI-compatible AI（不带 raw dump）
    -> 认证 API / 全局崩溃页面
```

组件边界：

- `internal/crashreports`：协议、认证前置检查、报告和 artifact 存储、保留清理。
- `internal/accelerator`：下载、归档验证、安装 manifest、Core 配置补丁和 `Ensure`/卸载。
- `internal/crashanalysis`：stackwalk、符号覆盖、脱敏摘要、AI HTTP 客户端和异步状态。
- `internal/store`：实例开关、全局非敏感设置和分析索引所需的 SQLite migration。
- `internal/httpapi`：认证 API、设置 API、实例输入和异步 Job 入口。
- `cmd/panel`：组装 manager、启动 worker、优雅停止和周期清理。
- `web/src/app`：崩溃报告页面、实例配置控件和全局设置控件。

## 5. 错误处理与安全不变量

- 下载 URL 必须是 HTTPS；GitHub 代理只改变下载请求，不把 GitHub token 转发给代理。
- 下载、解压、Core patch 或 manifest 写入失败时，旧安装保持可恢复；禁止半安装状态被标记为成功。
- 外部修改 Panel 管理文件时不静默覆盖或删除，返回冲突并记录任务事件。
- 上传请求不信任 `X-Forwarded-For`，不信任 `UserID`，不信任客户端文件名和模块路径。
- `/binary/submit` 不再拒绝合法二进制，但只接受 managed instance、共享 token、loopback 和硬上限内的模块文件；它不是任意文件上传接口。
- 原始 dump、binary、symbol、metadata 只能通过认证 API 访问；公开 receiver 只返回 Accelerator 所需的文本响应。
- AI 输入必须经过结构化脱敏；测试必须证明 dump 的唯一字节序列、binary 内容和 token 不出现在请求 body。
- retention 清理只触及 Panel 自己的 crash report/artifact 根，不触及实例游戏目录、日志或用户私有文件。
- 任何 worker 重启后都能从 manifest 的 queued/running 状态恢复为可重试状态，不丢失已接收报告。

## 6. 验证与验收

### 后端与协议

- 预提交解析 Linux/Windows、模块 debug identifier、内置 symbol 命中和 `Y/U/N` 响应。
- `/symbols/submit` 和 `/binary/submit` 覆盖 token、loopback、managed instance、字段、大小、重复内容、路径安全和原子写入。
- 公开端点不会接受其他实例或不受管理的 `ServerID`，也不会接受 body 中的任意路径。
- 90 天清理同时覆盖报告、metadata、stackwalk、AI 结果和不再引用的二进制；内置符号不被删除。

### Accelerator 安装

- 假下载器验证 URL、GitHub 代理开关、SHA-256 去重和缓存。
- 归档测试覆盖合法文件、未知路径、符号链接、路径穿越、缺失扩展和大小限制。
- Core.cfg 测试验证只修改指定键、保留其他键、重复 patch 幂等、禁用恢复和外部修改冲突。
- 实例测试验证启用、禁用、运行中重启、停止实例不启动，以及插件重装/更新/容器重建后 `Ensure` 再次执行。
- 真实 Accelerator 包安装后检查 `core.cfg` 三个 URL 均为 loopback Panel 地址，`MinidumpBinaryUpload=yes`，且没有 Throttle URL。

### Stackwalk、AI 和前端

- fake stackwalk 工具覆盖成功、超时、非零退出、超大输出和缺失符号。
- 脱敏测试覆盖 metadata 中的路径、IP、ServerID、UserID、token 和命令行。
- fake OpenAI-compatible endpoint 验证请求格式、模型、超时、重试、保存结果以及 raw dump/binary 不出站。
- HTTP API 测试覆盖认证、列表筛选、详情、四类下载、手动分析和配置错误。
- React/Vitest 测试覆盖全局列表、筛选、详情状态、手动分析、自动分析标识、空/错误状态和实例设置保存。
- 使用 `D:\Windows\Download\crash_rmvtruf2n5tk.dmp` 做真实接收、去重、下载和 stackwalk 输入验证；不把 dump 复制进仓库。
- 完成前运行 `go test -p 1 ./... -count=1`、`go vet ./...`、前端测试、`git diff --check` 和 `docker compose config --quiet`；线上只做受控测试，不自动重建或重启安可服。

## 7. 设计输入与影响草稿

### TaskIntentDraft

把现有 Accelerator receiver 扩展为 Panel 管理的完整能力：实例自动安装和配置、完整上传协议、在线诊断和可选 AI 分析，同时维持本机来源、managed instance、90 天保留和 raw dump 不出 Panel 的边界。

### BaselineReadSetHint

本设计参考 `CONTEXT.md`、现有 receiver spec、`internal/crashreports`、`internal/httpapi/server.go`、`internal/store`、`internal/content`、`internal/updates`、`internal/lifecycle`、`internal/secrets`、`cmd/panel/main.go`、`docker-compose.yml`、`web/src/app/App.tsx`、`InstanceConfigModal.tsx` 以及 Accelerator 上游 `extension.cpp`。外部调查确认官方构建包来自 `builds.limetech.io`，上游通过 `MinidumpUrl`、`MinidumpSymbolUrl` 和 `MinidumpBinaryUrl` 读取 Core 配置。

### ImpactStatementDraft

受影响层：SQLite/domain 实例契约、系统设置和 secrets、文件存储、上传协议、实例生命周期和插件更新任务、Panel HTTP API、React 页面和 Panel Docker 镜像工具。必须保持的兼容边界：现有 `/api` 会话认证、host network、overlay/private 文件安全、普通插件包生命周期、游戏日志和 receiver 的 loopback + managed instance 限制。主要风险是扩展文件所有权、Core 配置冲突、二进制磁盘增长、stackwalk 进程隔离和 AI 脱敏遗漏。
