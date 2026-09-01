# 游戏实例控制台缓存上限实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 将游戏实例控制台后端历史和前端显示缓存的行数上限从当前 1000 行统一提高到 8192 行。

**Architecture:** 沿用现有的双层有界缓存：`internal/httpapi` 负责实例会话历史，React 前端负责当前终端文本。只调整两处已有行数常量；后端 1 MiB 字节上限和 WebSocket 协议保持不变。

**Tech Stack:** Go `testing`、React/TypeScript、Vitest、Vite。

**Baseline / Authority Refs:** `CONTEXT.md`；`docs/aegis/work/2026-08-16-instance-console-history/30-plan.md`；`internal/httpapi/console.go`；`internal/httpapi/console_test.go`；`web/src/app/consoleBuffer.ts`；`web/src/app/consoleBuffer.test.ts`。

**Compatibility Boundary:** 保持 `/api/instances/{id}/console` WebSocket 地址、命令帧类型、实时订阅、实例隔离、后端 1 MiB 字节上限和 Panel 重启后历史清空行为不变。只扩大行数容量，不修改其他日志/性能缓存。

**Verification:** 先运行新增的 Go/Vitest 回归测试确认旧实现失败；修改常量后运行控制台目标测试、Go 全量测试、前端测试和前端构建。

---

### Task 1: 为 8192 行默认行为补充失败测试

**Files:**
- Modify: `internal/httpapi/console_test.go`
- Modify: `web/src/app/consoleBuffer.test.ts`

**Why this task exists:** 防止只修改一个层级，或把测试继续固定在旧的 1000 行行为上；测试必须验证用户重新打开控制台和当前页面显示都能保留最新 8192 行。

**Impact / Compatibility:** 测试调用现有真实缓存函数，不改变生产接口；显式传入自定义 `maxLines` 的前端行为仍由现有测试覆盖。

**Verification:**

- Go：新增一个 8193 行跨帧历史测试，断言保留最新 8192 行。
- Vitest：新增一个不传 `maxLines` 的 8193 行测试，断言默认保留最新 8192 行。
- 两个新增测试在常量仍为 1000 时必须失败，并且失败原因是实际保留行数不足。

- [ ] **Step 1: 写 Go 失败测试**

在 `internal/httpapi/console_test.go` 增加以下测试：

```go
func TestAppendConsoleHistoryKeepsNewest8192LinesAcrossFrames(t *testing.T) {
	first := strings.Builder{}
	for index := 1; index <= 7000; index++ {
		_, _ = first.WriteString("old-" + strconv.Itoa(index) + "\n")
	}
	second := strings.Builder{}
	for index := 1; index <= 1193; index++ {
		_, _ = second.WriteString("new-" + strconv.Itoa(index) + "\n")
	}

	history := appendConsoleHistory(nil, []byte(first.String()))
	history = appendConsoleHistory(history, []byte(second.String()))
	lines := strings.Split(strings.TrimSuffix(string(history), "\n"), "\n")
	if len(lines) != 8192 {
		t.Fatalf("lines=%d, want 8192", len(lines))
	}
	if lines[0] != "old-2" || lines[len(lines)-1] != "new-1193" {
		t.Fatalf("history endpoints=%q...%q", lines[0], lines[len(lines)-1])
	}
}
```

- [ ] **Step 2: 写前端失败测试**

在 `web/src/app/consoleBuffer.test.ts` 增加以下测试：

```ts
it("keeps the newest 8192 lines with the default limit", () => {
  const incoming = Array.from({ length: 8193 }, (_, index) => `line-${index + 1}`).join("\n");

  const output = appendConsoleOutput("", incoming);

  expect(output.split("\n")).toHaveLength(8192);
  expect(output.startsWith("line-2\n")).toBe(true);
  expect(output.endsWith("line-8193")).toBe(true);
});
```

- [ ] **Step 3: 运行新增测试确认 RED**

运行：

```sh
go test ./internal/httpapi -run TestAppendConsoleHistoryKeepsNewest8192LinesAcrossFrames -count=1
npm --prefix web test -- --run src/app/consoleBuffer.test.ts -t "keeps the newest 8192 lines"
```

预期：两个命令都失败；Go 实际行数为 1000，Vitest 实际输出为 1000 行。若出现编译错误或测试未被发现，先修正测试命令/测试本身，再重新确认该行为失败。

### Task 2: 将两层生产缓存上限改为 8192

**Files:**
- Modify: `internal/httpapi/console.go:12-14`
- Modify: `web/src/app/consoleBuffer.ts:1`

**Why this task exists:** 后端历史快照和前端终端文本必须使用相同目标容量，否则任一层都会继续截断历史。

**Impact / Compatibility:** 只将 `maxConsoleHistoryLines` 与 `NATIVE_CONSOLE_MAX_LINES` 从 1000 改为 8192；`maxConsoleHistoryBytes`、函数签名、WebSocket 帧流程和订阅生命周期不变。

**Repair Track:**

- 根因：后端和前端各自拥有独立的 1000 行截断常量，用户可见容量由较小/较早截断的一层决定。
- 规范 owner：继续由后端 `appendConsoleHistory` 和前端 `appendConsoleOutput` 分别负责各自缓存边界。
- 最小修复：仅同步两个已有常量的目标值。

**Retirement Track:**

- 旧的 1000 行默认边界在两个常量中退出。
- 不保留旧值 fallback，不迁移历史数据；显式传入自定义 `maxLines` 的通用前端函数能力继续保留。
- 如未来需要管理员可配置容量，再单独设计配置契约，不在本任务中引入。

- [ ] **Step 1: 修改后端常量**

将 `internal/httpapi/console.go` 中的：

```go
maxConsoleHistoryLines = 1000
```

改为：

```go
maxConsoleHistoryLines = 8192
```

- [ ] **Step 2: 修改前端常量**

将 `web/src/app/consoleBuffer.ts` 中的：

```ts
export const NATIVE_CONSOLE_MAX_LINES = 1000;
```

改为：

```ts
export const NATIVE_CONSOLE_MAX_LINES = 8192;
```

- [ ] **Step 3: 运行 Task 1 测试确认 GREEN**

运行：

```sh
go test ./internal/httpapi -run TestAppendConsoleHistoryKeepsNewest8192LinesAcrossFrames -count=1
npm --prefix web test -- --run src/app/consoleBuffer.test.ts -t "keeps the newest 8192 lines"
```

预期：两个命令均通过。

### Task 3: 回归验证与变更审查

**Files:**
- No additional production files.

**Why this task exists:** 控制台缓存是用户可见行为，必须确认现有截断边界、控制台后端包和前端构建没有回归。

**Impact / Compatibility:** 只验证本任务触及的后端/前端边界；不把当前根目录已有的 `internal/accelerator/*` 改动或未跟踪日志导出纳入本次变更。

**Verification:**

- [ ] **Step 1: 运行后端控制台测试**

运行：`go test ./internal/httpapi -run Console -count=1`

预期：控制台相关测试全部通过。

- [ ] **Step 2: 运行 Go 全量回归**

运行：`go test ./...`

预期：所有 Go 包测试通过。

- [ ] **Step 3: 运行前端单元测试**

运行：`npm --prefix web test -- --run src/app/consoleBuffer.test.ts`

预期：控制台缓存测试全部通过。

- [ ] **Step 4: 运行前端生产构建**

运行：`npm --prefix web run build`

预期：Vite 构建退出码为 0。

- [ ] **Step 5: 检查差异边界**

运行：`git diff --check` 与 `git status --short`

预期：无空白错误；变更仅包含本任务的文档、两个生产常量和两组测试，未触碰用户原有改动。
