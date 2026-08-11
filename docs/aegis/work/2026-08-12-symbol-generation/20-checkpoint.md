# Todo Checkpoint

## Current state

- Active slice: 自动生成和持久化本地 ELF Breakpad 符号。
- Completed: 基线读取、worktree 隔离、TDD red/green、生成器、Manager 持久化、Panel/Compose/配置接入、文档更新。
- Remaining: staged diff review、代码审查、安可镜像构建与实际模块验证、合并主分支。
- Blocked on: 本机没有 Docker CLI；全量 Go 测试存在多包 Windows `TempDir` 清理抖动，受影响测试单独重跑可通过。

## Drift check

- Scope: remains within local symbol generation and crash analysis storage.
- Compatibility: legacy Accelerator symbol uploads remain accepted, including MODULE records without debug file.
- Retirement: no Throttle path or new public symbol source was added.
- Decision: continue to remote build and integration verification.
