# Task Intent

## Requested outcome

在崩溃报告详情的前端 Stackwalk 区域，仅显示按 Thread 分组的调用帧，并允许切换不同 Thread；不展示 minidump_stackwalk 的普通日志。最近上传列表的每条报告底部显示崩溃线程顶部调用，替代当前签名或哈希标识。

## Scope

- 修改前端 Stackwalk 解析、Thread 切换和列表摘要展示。
- 保留 Stackwalk 原始下载文本、DMP、后端接口和 AI 输入不变。
- 兼容没有 Thread 标题的紧凑 Stackwalk 文本。

## Non-goals

- 不修改后端、数据库、Stackwalk 工具输出或原始 DMP。
- 不改变 Stackwalk 下载按钮和 AI 分析行为。

## Acceptance criteria

- `Thread N` / `Thread N (crashed)` 被识别并可切换。
- 每个 Thread 只渲染调用帧；`Found by` 作为帧的附属信息保留，普通日志不渲染。
- 最近上传列表显示崩溃线程第一帧的模块、符号和偏移；没有解析帧时显示稳定的中文空状态。
- 现有短格式 Stackwalk 没有 Thread 标题时仍能显示为默认线程。
