import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { App, type Instance } from "./App";
const instance: Instance = {
  id: "1",
  name: "深夜战役",
  actual_state: "running",
  game_port: 27015,
  start_map: "c2m1_highway",
  game_mode: "coop",
  max_players: 8,
  players: 4,
  cpu: 31,
  memory: 2.4,
};
describe("App", () => {
  it("shows operational instance data", () => {
    render(<App initialInstances={[instance]} />);
    expect(screen.getByText("深夜战役")).toBeInTheDocument();
    expect(screen.getByText("4 / 8")).toBeInTheDocument();
    expect(screen.getByText("c2m1_highway")).toBeInTheDocument();
  });
  it("requires confirmation before stopping", async () => {
    const onAction = vi.fn();
    render(<App initialInstances={[instance]} onAction={onAction} />);
    await userEvent.click(
      screen.getByRole("button", { name: "停止 深夜战役" }),
    );
    expect(onAction).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "确认停止" }));
    expect(onAction).toHaveBeenCalledWith("1", "stop");
  });
  it("logs in and loads real instances", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response("{}", {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response('{"authenticated":true}', {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response("[]", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);
    render(<App />);
    expect(await screen.findByText("管理员认证")).toBeInTheDocument();
    await userEvent.type(
      screen.getByLabelText("管理员密码"),
      "correct horse battery staple",
    );
    await userEvent.click(screen.getByRole("button", { name: "进入作战室" }));
    expect(
      await screen.findByText("尚无实例。创建第一个 Host 网络服务器。"),
    ).toBeInTheDocument();
    vi.unstubAllGlobals();
  });
  it("does not apply a private overlay when saving the file fails", async () => {
    const calls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input);
        calls.push(`${init?.method || "GET"} ${path}`);
        if (
          path === "/api/content/vpk" ||
          path === "/api/packages" ||
          path === "/api/instances/1/private"
        ) {
          return new Response("[]", {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (init?.method === "PUT" && path.includes("/private/")) {
          return new Response(
            JSON.stringify({ error: { message: "invalid private path" } }),
            { status: 422, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(
          JSON.stringify({ ID: "job-1", Status: "pending" }),
          { status: 202, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    await screen.findByText("实例私有覆盖");
    await userEvent.click(
      screen.getByRole("button", { name: "保存并立即应用" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "invalid private path",
    );
    expect(calls.some((x) => x.includes("/private/apply"))).toBe(false);
    vi.unstubAllGlobals();
  });
  it("disables instance-scoped content actions when no instance exists", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        const body =
          path === "/api/packages"
            ? '[{"id":"pkg-1","filename":"plugins.zip","version":"v1","size":4,"hot_compatible":true}]'
            : "[]";
        return new Response(body, {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    expect(
      await screen.findByRole("button", { name: "热更新" }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: "完整更新" })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "保存并立即应用" }),
    ).toBeDisabled();
    await waitFor(() =>
      expect(screen.getByText("plugins.zip · v1")).toBeInTheDocument(),
    );
    vi.unstubAllGlobals();
  });
  it("shows persisted jobs on a dedicated operations page", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input) === "/api/jobs") {
          return new Response(
            '[{"ID":"job-1","Type":"game_update","Status":"failed","Stage":"steamcmd","Percent":37,"Error":"download interrupted"}]',
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response("[]", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "任务" }));
    expect(await screen.findByText("game_update")).toBeInTheDocument();
    expect(screen.getByText("download interrupted")).toBeInTheDocument();
    vi.unstubAllGlobals();
  });
  it("loads VPK downloads and private files into the editor", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        if (path === "/api/content/vpk") {
          return new Response(
            '[{"name":"maps.vpk","size":1024,"hash":"abcdef"}]',
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        if (path === "/api/packages") {
          return new Response("[]", {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (path === "/api/instances/1/private") {
          return new Response(
            '[{"path":"cfg/server.cfg","size":14,"hash":"123456"}]',
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        if (path === "/api/instances/1/private/file/cfg/server.cfg") {
          return new Response("hostname smoke", {
            status: 200,
            headers: { "Content-Type": "text/plain" },
          });
        }
        return new Response("[]", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    const download = await screen.findByRole("link", { name: "下载 maps.vpk" });
    expect(download).toHaveAttribute(
      "href",
      "/api/content/vpk/maps.vpk/download",
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "编辑 cfg/server.cfg" }),
    );
    expect(
      await screen.findByDisplayValue("hostname smoke"),
    ).toBeInTheDocument();
    vi.unstubAllGlobals();
  });
});
