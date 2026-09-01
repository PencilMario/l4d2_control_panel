# 游戏实例控制台文本导出实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 在实例控制台页面把当前最多 8192 行的显示文本导出为本地 UTF-8 `.txt` 文件。

**Architecture:** 前端 `Terminal` 组件从已有 `output` 状态创建 Blob，并通过临时下载链接触发浏览器下载。文件名清理与下载生命周期放在独立的 `consoleExport.ts` 小模块中，组件只负责调用事件处理函数；后端协议和缓存不变。

**Tech Stack:** React、TypeScript、Vitest、Testing Library、浏览器 Blob/Object URL API。

**Baseline / Authority Refs:** `CONTEXT.md`；`docs/aegis/work/2026-09-01-console-history-cap/20-spec.md`；`web/src/app/App.tsx`；`web/src/app/App.test.tsx`；`web/src/app/CrashReportsPage.tsx`；`web/src/styles/app.css`。

**Compatibility Boundary:** 保持 `/api/instances/{id}/console` WebSocket、控制台输出/跟随/清空/命令发送行为、现有工具栏样式和后端 1 MiB 字节上限不变。导出是纯本地副作用，不新增 API 或持久化数据。

**Verification:** 先增加 UI 导出失败测试并确认旧代码失败；实现下载模块与按钮；再运行目标测试、前端全量测试、Go 全量测试和前端构建。

---

### Task 1: 添加控制台导出失败测试

**Files:**
- Modify: `web/src/app/App.test.tsx`

**Why this task exists:** 用用户实际操作路径保护导出按钮状态、Blob 内容、文件名和 Object URL 清理行为。

**Impact / Compatibility:** 测试使用现有 Fake WebSocket 和实例控制台，不改变生产协议；测试仅模拟浏览器下载 API。

**Verification:** 在尚无导出按钮的当前代码上，目标测试应因找不到“导出控制台文本”按钮而失败，而不是因测试环境初始化错误失败。

- [ ] **Step 1: 扩展现有控制台测试**

在打开控制台并取得 `output` 后，先断言按钮初始禁用；收到 `ready\n` 后断言按钮启用。为浏览器下载 API 建立 spies，然后点击按钮并断言 Blob 文本、文件名、链接点击和 URL 释放：

```tsx
const exportButton = screen.getByRole("button", { name: "导出控制台文本" });
expect(exportButton).toBeDisabled();
act(() => sockets[0].onmessage?.({ data: "ready\n" } as MessageEvent));
expect(exportButton).toBeEnabled();

const createObjectURL = vi.fn(() => "blob:console-export");
const revokeObjectURL = vi.fn();
vi.stubGlobal("URL", { createObjectURL, revokeObjectURL });
const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
const appendChild = vi.spyOn(document.body, "appendChild");

await userEvent.click(exportButton);

const [blob] = createObjectURL.mock.calls[0] as [Blob];
expect(await blob.text()).toBe("ready\n");
const anchor = appendChild.mock.calls.at(-1)?.[0] as HTMLAnchorElement;
expect(anchor.download).toBe("深夜战役-console.txt");
expect(anchor.href).toBe("blob:console-export");
expect(click).toHaveBeenCalledOnce();
expect(revokeObjectURL).toHaveBeenCalledWith("blob:console-export");
```

- [ ] **Step 2: Run the target test to verify RED**

Run: `npm --prefix web test -- --run src/app/App.test.tsx -t "exports console text"`

Expected: FAIL because the current `Terminal` has no button named `导出控制台文本`. If the test errors before the assertion, correct only the test setup and rerun until it fails for the missing behavior.

### Task 2: Implement local TXT download

**Files:**
- Create: `web/src/app/consoleExport.ts`
- Modify: `web/src/app/App.tsx`

**Why this task exists:** 提供可复用且可测试的文件名清理和下载生命周期，避免把临时 DOM/URL 管理散落在控制台渲染逻辑中。

**Impact / Compatibility:** 新模块只调用浏览器本地 API；没有网络请求、服务端状态或控制台 WebSocket 变化。按钮在 `output` 为空时保持禁用。

**Repair Track:**

- 根因：控制台只有显示和清空操作，没有把现有 `output` 状态转换为用户可保存文件的路径。
- 规范 owner：`Terminal` 持有导出事件与当前文本；`consoleExport.ts` 负责文件名和 Blob/临时链接生命周期。
- 最小修复：增加一个本地下载 helper、在标题栏增加一个禁用条件明确的按钮，并复用已有 `Download` 图标。

**Retirement Track:**

- 不保留旧的“复制到服务器/新增导出 API”路径，因为本功能不需要服务器参与。
- 不改变“清空显示”行为；导出只读取当前状态，不清空或截断额外内容。
- 若未来需要导出后端完整历史，应另行设计服务器下载接口，不把本地导出 helper 扩展成第二种数据 owner。

- [ ] **Step 1: 添加文件名清理与下载 helper**

创建 `web/src/app/consoleExport.ts`：

```ts
export function consoleDownloadFilename(instanceName: string) {
  const safeName = instanceName
    .replace(/[<>:"/\\|?*\u0000-\u001f]/g, "_")
    .replace(/[. ]+$/g, "")
    .trim();
  return `${safeName || "game-instance"}-console.txt`;
}

export function downloadConsoleText(text: string, instanceName: string) {
  const blob = new Blob([text], { type: "text/plain;charset=utf-8" });
  const href = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = href;
  anchor.download = consoleDownloadFilename(instanceName);
  anchor.style.display = "none";
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(href);
}
```

- [ ] **Step 2: 添加控制台标题栏按钮**

在 `web/src/app/App.tsx` 引入 `downloadConsoleText`，在 `Terminal` 的 `.terminal-head-actions` 中保留原有清空按钮，并增加：

```tsx
<button
  type="button"
  title="导出控制台文本"
  aria-label="导出控制台文本"
  disabled={!output}
  onClick={() => downloadConsoleText(output, instance.name)}
>
  <Download />导出 TXT
</button>
```

按钮放在清空按钮之后，确保现有移动端 `button:first-child` 规则仍优先折叠清空操作；现有 `.terminal-head-actions button` 样式自动覆盖新按钮。

- [ ] **Step 3: Run the target test to verify GREEN**

Run: `npm --prefix web test -- --run src/app/App.test.tsx -t "exports console text"`

Expected: PASS; the test observes `ready\n` in the Blob, `深夜战役-console.txt` as the filename, a link click, and Object URL revocation.

### Task 3: Add focused filename boundary tests and run regressions

**Files:**
- Create: `web/src/app/consoleExport.test.ts`
- No additional production changes unless verification exposes a defect.

**Why this task exists:** 文件名来自用户配置的实例名，非法字符和空名称必须保持可下载且不污染路径；同时要确认新增按钮没有破坏既有控制台流程。

**Impact / Compatibility:** 只测试 helper 的纯文件名规则和现有前端/Go 回归，不改变其他文件下载功能。

**Verification:**

- [ ] **Step 1: Add filename boundary tests**

创建以下测试：

```ts
import { describe, expect, it } from "vitest";
import { consoleDownloadFilename } from "./consoleExport";

describe("consoleDownloadFilename", () => {
  it("replaces illegal filename characters and trims trailing separators", () => {
    expect(consoleDownloadFilename('night:/raid?. ')).toBe("night__raid_-console.txt");
  });

  it("uses a safe fallback for an empty name", () => {
    expect(consoleDownloadFilename("...   ")).toBe("game-instance-console.txt");
  });
});
```

- [ ] **Step 2: Run frontend regressions**

Run: `npm --prefix web test -- --run src/app/App.test.tsx src/app/consoleExport.test.ts src/app/consoleBuffer.test.ts` and then `npm --prefix web test -- --run`.

Expected: targeted tests and all frontend tests pass.

- [ ] **Step 3: Run backend and build regressions**

Run: `go test ./...` and `npm --prefix web run build`.

Expected: all Go packages pass and the production build exits with code 0. Restore any generated `web/public/vpk-cleaner.wasm` diff before final review.

- [ ] **Step 4: Check final diff boundary**

Run: `git diff --check`, `git status --short`, and `git diff --stat`.

Expected: only the approved 8192-line cache changes, export implementation/tests, and task records are present; no generated build artifact or unrelated user change is included.
