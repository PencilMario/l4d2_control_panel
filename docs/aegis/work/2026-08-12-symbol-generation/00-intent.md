# Task Intent

## Requested outcome

为 Accelerator 崩溃分析补齐本地 Breakpad `dump_syms` 工具和自动符号索引：扫描共享游戏 release 与实例 overlay 中实际存在的 ELF 模块，生成有效 `.sym`，按 MODULE debug identity 去重并写入 Panel 的本地符号库。

## Scope

- Panel 镜像构建 `dump_syms` 并提供可配置路径。
- 新增 ELF 扫描、命令执行、MODULE 解析、失败隔离和定时索引。
- 生成符号通过 `crashreports.Manager` 内容寻址保存，并标记为持久生成 artifact。
- 保留现有 Accelerator symbol 上传协议和 SourceMod/Metamod 公共内置符号。

## Non-goals

- 不把 Valve、Steam、L4D2 或实例生成符号提交到仓库公共内置资产。
- 不引入 Throttle 或外部符号服务。
- 不将客户端路径作为服务器落盘路径。
