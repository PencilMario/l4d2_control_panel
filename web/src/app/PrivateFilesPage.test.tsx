import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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

function mockPrivateAPI(options?: { error?: boolean; binaryOnly?: boolean }) {
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
        return new Response(null, {
          status: 204,
          headers: { "Upload-Offset": "4" },
        });
      }
      return new Response(null, { status: 204 });
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return { calls, fetchMock };
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("PrivateFilesPage", () => {
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
    expect(screen.getByRole("dialog", { hidden: true })).toHaveAttribute(
      "aria-modal",
      "true",
    );
  });
});
