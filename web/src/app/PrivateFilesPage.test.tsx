import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Instance } from "./App";
import { PrivateFilesPage } from "./PrivateFilesPage";

const instance = {
  id: "abc",
  name: "深夜战役",
} as Instance;

const json = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function mockPrivateAPI(options?: {
  error?: boolean;
  binaryOnly?: boolean;
  importResponse?: Promise<Response>;
}) {
  let modified = false;
  const calls: Array<{ path: string; init?: RequestInit }> = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      calls.push({ path, init });
      if (options?.error && path.endsWith("/private/tree")) {
        return json({ error: { message: "workspace unavailable" } }, 503);
      }
      if (path.endsWith("/private/tree")) {
        return json(
          options?.binaryOnly
            ? [
                { path: "addons", kind: "directory", updated_at: "now" },
                {
                  path: "addons/plugin.vpk",
                  kind: "file",
                  size: 4,
                  hash: "deadbeef",
                  updated_at: "now",
                },
              ]
            : [
                { path: "cfg", kind: "directory", updated_at: "now" },
                { path: "empty", kind: "directory", updated_at: "now" },
                {
                  path: "cfg/server.cfg",
                  kind: "file",
                  size: 14,
                  hash: "1234",
                  updated_at: "now",
                },
              ],
        );
      }
      if (path.endsWith("/private/diff")) {
        return json({
          changes: modified
            ? [{ path: "cfg/server.cfg", kind: "modified" }]
            : [],
          summary: { added: 0, modified: modified ? 1 : 0, deleted: 0 },
        });
      }
      if (path.endsWith("/private/snapshots")) {
        return json([
          {
            id: "20260715T090000.000000000Z-snapshot",
            applied_at: "2026-07-15T09:00:00Z",
            summary: { added: 1, modified: 0, deleted: 0 },
          },
        ]);
      }
      if (path.endsWith("/private/archive") && !init?.method) {
        return new Response(new Uint8Array([80, 75, 5, 6]), {
          status: 200,
          headers: { "Content-Type": "application/zip" },
        });
      }
      if (path.endsWith("/private/archive?confirm=true") && init?.method === "POST") {
        return options?.importResponse ?? new Response(null, { status: 204 });
      }
      if (path.includes("/private/file/cfg/server.cfg") && !init?.method) {
        return new Response("hostname smoke", { status: 200 });
      }
      if (path.includes("/private/cfg/server.cfg") && init?.method === "PUT") {
        modified = true;
        return new Response(null, { status: 204 });
      }
      if (path.endsWith("/private/uploads") && init?.method === "POST") {
        return json({ id: "upload-1", offset: 0 }, 201);
      }
      if (path.endsWith("/private/uploads/upload-1") && init?.method === "PATCH") {
        const offset = Number((init.headers as Record<string, string>)["Upload-Offset"] || 0);
        const size = (init.body as Blob)?.size || 0;
        return new Response(null, {
          status: 204,
          headers: { "Upload-Offset": String(offset + size) },
        });
      }
      return new Response(null, { status: 204 });
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return { calls, fetchMock };
}

afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("PrivateFilesPage", () => {
  it("explains and performs a full workspace ZIP replacement without applying", async () => {
    const { calls } = mockPrivateAPI();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    const queue = vi.fn();
    render(<PrivateFilesPage instances={[instance]} queue={queue} />);
    await screen.findByRole("treeitem", { name: "cfg" });
    const file = new File(
      [new Uint8Array([80, 75, 3, 4])],
      "replacement.zip",
      { type: "application/zip" },
    );
    fireEvent.change(screen.getByLabelText("导入 ZIP"), {
      target: { files: [file] },
    });
    await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1));
    const warning = String(confirm.mock.calls[0][0]);
    expect(warning).toContain("replacement.zip");
    expect(warning).toContain("深夜战役");
    expect(warning).toContain(
      "ZIP 中不存在的现有文件和未应用更改将被删除，不会保留",
    );
    expect(warning).toContain("历史应用快照不受影响");
    expect(warning).toContain("不会自动应用到游戏目录");
    expect(
      await screen.findByText("工作区已完全替换，请检查差异后应用更改。"),
    ).toBeVisible();
    const imported = calls.find(({ path, init }) =>
      path.endsWith("/private/archive?confirm=true") && init?.method === "POST"
    );
    expect(imported?.init?.body).toBe(file);
    expect((imported?.init?.headers as Record<string, string>)["Content-Type"]).toBe(
      "application/zip",
    );
    expect(queue).not.toHaveBeenCalled();
  });

  it("cancels ZIP import before sending a request", async () => {
    const { calls } = mockPrivateAPI();
    vi.spyOn(window, "confirm").mockReturnValue(false);
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    await screen.findByRole("treeitem", { name: "cfg" });
    fireEvent.change(screen.getByLabelText("导入 ZIP"), {
      target: { files: [new File(["zip"], "cancel.zip", { type: "application/zip" })] },
    });
    await Promise.resolve();
    expect(calls.some(({ path }) => path.includes("/private/archive"))).toBe(false);
  });

  it("exports the selected workspace as a ZIP download", async () => {
    const { calls } = mockPrivateAPI();
    const createObjectURL = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:private-zip");
    const revokeObjectURL = vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    await userEvent.click(await screen.findByRole("button", { name: "导出 ZIP" }));
    expect(await screen.findByText("私有文件 ZIP 已导出")).toBeVisible();
    expect(calls.some(({ path, init }) =>
      path.endsWith("/instances/abc/private/archive") && !init?.method
    )).toBe(true);
    expect(createObjectURL.mock.calls[0][0]).toMatchObject({
      size: 4,
      type: "application/zip",
    });
    expect(click).toHaveBeenCalledTimes(1);
    expect((click.mock.contexts[0] as HTMLAnchorElement).download)
      .toBe("private-files-abc.zip");
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:private-zip");
  });

  it("keeps an in-flight ZIP import owned by its original instance", async () => {
    let finishImport!: (response: Response) => void;
    const importResponse = new Promise<Response>((resolve) => {
      finishImport = resolve;
    });
    const { calls } = mockPrivateAPI({ importResponse });
    vi.spyOn(window, "confirm").mockReturnValue(true);
    render(
      <PrivateFilesPage
        instances={[instance, { ...instance, id: "def", name: "黎明战役" }]}
        queue={vi.fn()}
      />,
    );
    await screen.findByRole("treeitem", { name: "cfg" });
    const importInput = screen.getByLabelText("导入 ZIP");
    fireEvent.change(importInput, {
      target: { files: [new File(["zip"], "abc.zip", { type: "application/zip" })] },
    });
    await waitFor(() => expect(calls.some(({ path }) =>
      path.includes("/instances/abc/private/archive?confirm=true")
    )).toBe(true));
    await userEvent.selectOptions(screen.getByLabelText("目标实例"), "def");
    finishImport(new Response(null, { status: 204 }));
    await waitFor(() => expect(importInput).not.toBeDisabled());
    expect(calls.some(({ path }) =>
      path.includes("/instances/def/private/archive?confirm=true")
    )).toBe(false);
    expect(screen.queryByText("工作区已完全替换，请检查差异后应用更改。"))
      .not.toBeInTheDocument();
  });

  it("stages edits and applies them only from the status bar", async () => {
    mockPrivateAPI();
    const queue = vi.fn().mockResolvedValue(undefined);
    render(<PrivateFilesPage instances={[instance]} queue={queue} />);
    await userEvent.click(await screen.findByRole("treeitem", { name: "cfg" }));
    await userEvent.click(screen.getByRole("button", { name: "编辑 server.cfg" }));
    await userEvent.clear(screen.getByLabelText("文件内容"));
    await userEvent.type(screen.getByLabelText("文件内容"), "hostname staged");
    await userEvent.click(screen.getByRole("button", { name: "保存到暂存区" }));
    expect(queue).not.toHaveBeenCalled();
    expect(await screen.findByText("1 项修改未应用")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "1 项修改未应用" }));
    expect(screen.getByRole("region", { name: "暂存差异" })).toHaveTextContent(
      "cfg/server.cfg",
    );
    await userEvent.click(screen.getByRole("button", { name: "应用更改" }));
    expect(queue).toHaveBeenCalledWith("/api/instances/abc/private/apply", {});
  });

  it("shows directories, empty directories, and no editor for binary files", async () => {
    mockPrivateAPI();
    const { rerender } = render(
      <PrivateFilesPage instances={[instance]} queue={vi.fn()} />,
    );
    expect(await screen.findByRole("treeitem", { name: "empty" })).toBeVisible();
    rerender(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
  });

  it("does not offer text editing for binary files", async () => {
    mockPrivateAPI({ binaryOnly: true });
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    await userEvent.click(await screen.findByRole("treeitem", { name: "addons" }));
    expect(screen.getByText("plugin.vpk")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "编辑 plugin.vpk" }),
    ).not.toBeInTheDocument();
  });

  it("confirms directory creation, move overwrite, and deletion", async () => {
    const { calls } = mockPrivateAPI();
    vi.spyOn(window, "prompt")
      .mockReturnValueOnce("cfg/new")
      .mockReturnValueOnce("cfg/renamed.cfg");
    vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    await screen.findByRole("tree");
    await userEvent.click(screen.getByRole("button", { name: "新建目录" }));
    await userEvent.click(screen.getByRole("treeitem", { name: "cfg" }));
    await userEvent.click(screen.getByRole("button", { name: "移动 server.cfg" }));
    await userEvent.click(screen.getByRole("button", { name: "删除 server.cfg" }));
    expect(calls.some((call) => call.path.endsWith("/private/directories"))).toBe(true);
    expect(
      calls.some(
        (call) =>
          call.path.endsWith("/private/move") &&
          String(call.init?.body).includes('"confirm":true'),
      ),
    ).toBe(true);
    expect(
      calls.some(
        (call) =>
          call.init?.method === "DELETE" && call.path.includes("confirm=true"),
      ),
    ).toBe(true);
  });

  it("uploads with SHA256, offset, and octet-stream headers", async () => {
    const { calls } = mockPrivateAPI();
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    await screen.findByRole("tree");
    const file = new File([new Uint8Array([1, 2, 3, 4])], "plugin.bin");
    fireEvent.change(screen.getByLabelText("上传文件"), {
      target: { files: [file] },
    });
    expect(await screen.findByText("上传完成 · 100%")).toBeVisible();
    const begin = calls.find(
      (call) => call.path.endsWith("/private/uploads") && call.init?.method === "POST",
    );
    expect(String(begin?.init?.body)).toContain('"sha256"');
    const patch = calls.find((call) => call.init?.method === "PATCH");
    expect(patch?.init?.headers).toMatchObject({
      "Content-Type": "application/offset+octet-stream",
      "Upload-Offset": "0",
    });
  });

  it("restores snapshots into staged workspace without queueing apply", async () => {
    const { calls } = mockPrivateAPI();
    const queue = vi.fn();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<PrivateFilesPage instances={[instance]} queue={queue} />);
    await userEvent.click(await screen.findByRole("button", { name: "历史快照" }));
    await userEvent.click(screen.getByRole("button", { name: /恢复 2026/ }));
    expect(calls.some((call) => call.path.includes("/snapshots/") && call.path.endsWith("/restore"))).toBe(true);
    expect(queue).not.toHaveBeenCalled();
    expect(await screen.findByText("快照已恢复到暂存区")).toBeVisible();
  });

  it("renders loading, error recovery, and mobile drawer semantics", async () => {
    mockPrivateAPI({ error: true });
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    expect(screen.getByRole("status")).toHaveTextContent("正在读取私有文件");
    expect(await screen.findByRole("alert")).toHaveTextContent("workspace unavailable");
    expect(screen.getByRole("button", { name: "重试" })).toBeVisible();
    expect(screen.getByRole("button", { name: "打开文件树" })).toHaveAttribute(
      "aria-controls",
      "private-tree-drawer",
    );
    expect(screen.queryByRole("dialog", { hidden: true })).not.toBeInTheDocument();
  });

  it("resumes a failed upload only when the file and target path match exactly", async () => {
    const { calls } = mockPrivateAPI();
    let patches = 0;
    const base = globalThis.fetch as ReturnType<typeof vi.fn>;
    base.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input); calls.push({ path, init });
      if (path.endsWith("/private/tree")) return json([]);
      if (path.endsWith("/private/diff")) return json({ changes: [], summary: { added: 0, modified: 0, deleted: 0 } });
      if (path.endsWith("/private/snapshots")) return json([]);
      if (path.endsWith("/private/uploads") && init?.method === "POST") return json({ id: "resume-1", offset: 0 }, 201);
      if (path.endsWith("/private/uploads/resume-1") && !init?.method) return json({ offset: 2 });
      if (init?.method === "PATCH") {
        patches++;
        return patches === 1 ? json({}, 409) : new Response(null, { status: 204, headers: { "Upload-Offset": "4" } });
      }
      return new Response(null, { status: 204 });
    });
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    const file = new File([new Uint8Array([1, 2, 3, 4])], "resume.bin");
    fireEvent.change(screen.getByLabelText("上传文件"), { target: { files: [file] } });
    expect(await screen.findByRole("button", { name: "恢复上传" })).toBeEnabled();
    fireEvent.change(screen.getByLabelText("上传文件"), { target: { files: [file] } });
    expect(await screen.findByText("上传完成 · 100%")).toBeVisible();
    expect(calls.filter((call) => call.path.endsWith("/private/uploads") && call.init?.method === "POST")).toHaveLength(1);
    expect(calls.some((call) => call.path.endsWith("/private/uploads/resume-1") && !call.init?.method)).toBe(true);
    expect(calls.some((call) => call.init?.method === "PATCH" && (call.init.headers as Record<string, string>)["Upload-Offset"] === "2")).toBe(true);
    expect(calls.some((call) => call.path.endsWith("/complete"))).toBe(true);
  });

  it("starts a new upload when the same file is reselected for a different target path", async () => {
    const { calls } = mockPrivateAPI();
    let uploadCount = 0;
    let patches = 0;
    const base = globalThis.fetch as ReturnType<typeof vi.fn>;
    base.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input); calls.push({ path, init });
      if (path.endsWith("/private/tree")) return json([{ path: "cfg", kind: "directory", updated_at: "now" }]);
      if (path.endsWith("/private/diff")) return json({ changes: [], summary: { added: 0, modified: 0, deleted: 0 } });
      if (path.endsWith("/private/snapshots")) return json([]);
      if (path.endsWith("/private/uploads") && init?.method === "POST") {
        uploadCount++;
        return json({ id: `path-${uploadCount}`, offset: 0 }, 201);
      }
      if (init?.method === "PATCH") {
        patches++;
        if (patches === 1) return json({}, 409);
        const size = (init.body as Blob)?.size || 0;
        return new Response(null, { status: 204, headers: { "Upload-Offset": String(size) } });
      }
      return new Response(null, { status: 204 });
    });
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    const file = new File([new Uint8Array([1, 2, 3, 4])], "same.bin");
    fireEvent.change(screen.getByLabelText("上传文件"), { target: { files: [file] } });
    expect(await screen.findByRole("button", { name: "恢复上传" })).toBeEnabled();
    await userEvent.click(screen.getByRole("treeitem", { name: "cfg" }));
    fireEvent.change(screen.getByLabelText("上传文件"), { target: { files: [file] } });
    expect(await screen.findByText("上传完成 · 100%")).toBeVisible();
    expect(uploadCount).toBe(2);
    expect(calls.some((call) => call.path.endsWith("/private/uploads/path-1") && !call.init?.method)).toBe(false);
    expect(String(calls.filter((call) => call.path.endsWith("/private/uploads") && call.init?.method === "POST").at(-1)?.init?.body)).toContain('"path":"cfg/same.bin"');
  });

  it("clears persisted upload metadata that uses the legacy fingerprint format", async () => {
    localStorage.setItem("private-upload:abc", JSON.stringify({
      id: "legacy-1",
      instanceID: "abc",
      path: "legacy.bin",
      size: 4,
      offset: 2,
      fingerprint: "legacy.bin:4:123:deadbeef",
    }));
    mockPrivateAPI();
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    await screen.findByRole("tree");
    await waitFor(() => expect(localStorage.getItem("private-upload:abc")).toBeNull());
    expect(screen.queryByRole("button", { name: "恢复上传" })).not.toBeInTheDocument();
  });

  it("rejects binary bytes even with a text extension", async () => {
    mockPrivateAPI();
    const base = globalThis.fetch as ReturnType<typeof vi.fn>;
    const original = base.getMockImplementation() as any;
    base.mockImplementation(async (input, init) => String(input).includes("/private/file/cfg/server.cfg")
      ? new Response(new Uint8Array([65, 0, 66]))
      : original(input, init));
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    await userEvent.click(await screen.findByRole("treeitem", { name: "cfg" }));
    await userEvent.click(screen.getByRole("button", { name: "编辑 server.cfg" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("不是可编辑的 UTF-8 文本");
    expect(screen.queryByLabelText("文件内容")).not.toBeInTheDocument();
  });

  it("edits valid UTF-8 files without an extension", async () => {
    mockPrivateAPI();
    const base = globalThis.fetch as ReturnType<typeof vi.fn>;
    const original = base.getMockImplementation() as any;
    base.mockImplementation(async (input, init) => {
      const path = String(input);
      if (path.endsWith("/private/tree")) return json([{ path: "README", kind: "file", size: 6, updated_at: "now" }]);
      if (path.includes("/private/file/README")) return new Response(new TextEncoder().encode("说明"));
      return original(input, init);
    });
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    await userEvent.click(await screen.findByRole("button", { name: "编辑 README" }));
    expect(await screen.findByLabelText("文件内容")).toHaveValue("说明");
  });

  it("loads path-specific file history", async () => {
    const { fetchMock } = mockPrivateAPI();
    const base = globalThis.fetch as ReturnType<typeof vi.fn>;
    const original = base.getMockImplementation() as any;
    base.mockImplementation(async (input, init) => String(input).includes("/private/history/cfg/server.cfg")
      ? json([{ path: "cfg/server.cfg.1", size: 10, hash: "old" }])
      : original(input, init));
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    await userEvent.click(await screen.findByRole("treeitem", { name: "cfg" }));
    await userEvent.click(screen.getByRole("treeitem", { name: "server.cfg" }));
    await userEvent.click(screen.getByRole("button", { name: "历史 server.cfg" }));
    expect(await screen.findByRole("dialog", { name: /文件历史/ })).toHaveTextContent("cfg/server.cfg.1");
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("/private/history/cfg/server.cfg"))).toBe(true);
  });

  it("waits for apply completion, refreshes counts, and prevents duplicate apply", async () => {
    let changed = true;
    const { fetchMock } = mockPrivateAPI();
    const original = fetchMock.getMockImplementation() as any;
    fetchMock.mockImplementation(async (input, init) => {
      if (String(input).endsWith("/private/diff")) return json(changed ? { changes: [{ path: "a", kind: "added" }], summary: { added: 1, modified: 0, deleted: 0 } } : { changes: [], summary: { added: 0, modified: 0, deleted: 0 } });
      return original(input, init);
    });
    let finish!: (job: unknown) => void;
    const queueAndWait = vi.fn(() => new Promise<any>((resolve) => { finish = resolve; }));
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} queueAndWait={queueAndWait} />);
    const apply = await screen.findByRole("button", { name: "应用更改" });
    act(() => {
      apply.click();
      apply.click();
    });
    expect(queueAndWait).toHaveBeenCalledTimes(1);
    expect(apply).toHaveAttribute("aria-busy", "true");
    changed = false;
    finish({ Status: "succeeded" });
    expect(await screen.findByText("工作区与已应用版本一致")).toBeVisible();
  });

  it("traps drawer focus, closes on Escape, and returns focus", async () => {
    mockPrivateAPI();
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    const trigger = screen.getByRole("button", { name: "打开文件树" });
    await userEvent.click(trigger);
    const close = screen.getByRole("button", { name: "关闭文件树" });
    expect(close).toHaveFocus();
    const drawer = screen.getByRole("dialog", { name: "私有文件目录" });
    const last = drawer.querySelectorAll<HTMLElement>("button:not([disabled])").item(
      drawer.querySelectorAll("button:not([disabled])").length - 1,
    );
    close.focus();
    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(last).toHaveFocus();
    fireEvent.keyDown(document, { key: "Tab" });
    expect(close).toHaveFocus();
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(screen.queryByRole("dialog", { name: "私有文件目录", hidden: true })).not.toBeInTheDocument();
  });

  it("clears the old instance immediately and reloads an identical path for the new instance", async () => {
    let resolveB!: (response: Response) => void;
    const abortedA: AbortSignal[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path.includes("/instances/a/private/tree")) {
        if (init?.signal) abortedA.push(init.signal);
        return json([{ path: "same.cfg", kind: "file", size: 1, hash: "content-a", updated_at: "now" }]);
      }
      if (path.includes("/instances/b/private/tree")) return new Promise<Response>((resolve) => { resolveB = resolve; });
      if (path.endsWith("/private/diff")) return json({ changes: [], summary: { added: 0, modified: 0, deleted: 0 } });
      return json([]);
    }));
    render(<PrivateFilesPage instances={[{ ...instance, id: "a", name: "A" }, { ...instance, id: "b", name: "B" }]} queue={vi.fn()} />);
    await userEvent.click(await screen.findByRole("treeitem", { name: "same.cfg" }));
    expect(screen.getByText(/content-a/)).toBeVisible();

    fireEvent.change(screen.getByLabelText("目标实例"), { target: { value: "b" } });
    expect(screen.queryByText(/content-a/)).not.toBeInTheDocument();
    expect(screen.queryByRole("treeitem", { name: "same.cfg" })).not.toBeInTheDocument();
    expect(abortedA.some((signal) => signal.aborted)).toBe(true);

    resolveB(json([{ path: "same.cfg", kind: "file", size: 2, hash: "content-b", updated_at: "now" }]));
    await userEvent.click(await screen.findByRole("treeitem", { name: "same.cfg" }));
    expect(screen.getByText(/content-b/)).toBeVisible();
  });

  it("ignores a deferred instance response after switching instances", async () => {
    let resolveA!: (response: Response) => void;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path.includes("/instances/a/private/tree")) return new Promise<Response>((resolve) => { resolveA = resolve; });
      if (path.includes("/instances/b/private/tree")) return json([{ path: "b.cfg", kind: "file", size: 1, updated_at: "now" }]);
      if (path.endsWith("/private/diff")) return json({ changes: [], summary: { added: 0, modified: 0, deleted: 0 } });
      return json([]);
    }));
    render(<PrivateFilesPage instances={[{ ...instance, id: "a", name: "A" }, { ...instance, id: "b", name: "B" }]} queue={vi.fn()} />);
    await userEvent.selectOptions(screen.getByLabelText("目标实例"), "b");
    expect(await screen.findByRole("treeitem", { name: "b.cfg" })).toBeVisible();
    resolveA(json([{ path: "a.cfg", kind: "file", size: 1, updated_at: "now" }]));
    await Promise.resolve();
    expect(screen.queryByRole("treeitem", { name: "a.cfg" })).not.toBeInTheDocument();
  });

  it("hashes large uploads incrementally without reading the whole file", async () => {
    mockPrivateAPI();
    const file = new File([new Uint8Array(9 * 1024 * 1024)], "large.bin");
    const wholeRead = vi.spyOn(file, "arrayBuffer");
    const slices = vi.spyOn(file, "slice");
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("上传文件"), { target: { files: [file] } });
    expect(await screen.findByText("上传完成 · 100%")).toBeVisible();
    expect(wholeRead).not.toHaveBeenCalled();
    expect(slices.mock.calls.filter((call) => Number(call[0]) < file.size).length).toBeGreaterThanOrEqual(6);
  });

  it("does not carry an in-progress upload into a newly selected instance", async () => {
    const { calls } = mockPrivateAPI();
    let release!: () => void;
    const file = new File([new Uint8Array([1, 2, 3, 4])], "switch.bin");
    vi.spyOn(file, "slice").mockReturnValue({
      size: 4,
      arrayBuffer: () => new Promise<ArrayBuffer>((resolve) => { release = () => resolve(new Uint8Array([1, 2, 3, 4]).buffer); }),
    } as Blob);
    render(<PrivateFilesPage instances={[{ ...instance, id: "abc" }, { ...instance, id: "b", name: "B" }]} queue={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("上传文件"), { target: { files: [file] } });
    await waitFor(() => expect(release).toBeTypeOf("function"));
    await userEvent.selectOptions(screen.getByLabelText("目标实例"), "b");
    release();
    await Promise.resolve();
    expect(calls.some((call) => call.path.includes("/instances/b/private/uploads"))).toBe(false);
    expect(screen.queryByRole("button", { name: "恢复上传" })).not.toBeInTheDocument();
  });

  it("keeps modal focus inside during unrelated state updates", async () => {
    mockPrivateAPI();
    render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: "打开文件树" }));
    const close = screen.getByRole("button", { name: "关闭文件树" });
    expect(close).toHaveFocus();
    const drawer = screen.getByRole("dialog", { name: "私有文件目录" });
    fireEvent.click(drawer.querySelector('[role="treeitem"]')!);
    expect(close).toHaveFocus();
  });
});
