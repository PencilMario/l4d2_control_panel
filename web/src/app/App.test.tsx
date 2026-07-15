import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App, type Instance } from "./App";
const instance: Instance = {
  id: "1",
  name: "深夜战役",
  actual_state: "running",
  game_port: 27015,
  sourcetv_port: 27020,
  plugin_ports: [27021],
  start_map: "c2m1_highway",
  game_mode: "coop",
  tickrate: 100,
  max_players: 8,
  extra_args: `-strictportbind`,
  package_id: "package-a",
  applied_package_id: "package-a",
  players: 4,
  cpu: 31,
  memory: 2.4,
};
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
describe("App", () => {
  it("shows operational instance data", () => {
    render(
      <App
        initialInstances={[instance]}
        initialPackages={[
          {
            id: "package-a",
            filename: "coop-a.zip",
            version: "v1",
            size: 1024,
            hot_compatible: true,
          },
        ]}
      />,
    );
    expect(screen.getByText("深夜战役")).toBeInTheDocument();
    expect(screen.getByText("4 / 8")).toBeInTheDocument();
    expect(screen.getByText("c2m1_highway")).toBeInTheDocument();
    expect(screen.getByText(/TV 27020.*插件 27021/)).toBeInTheDocument();
    expect(screen.getByText(/coop-a\.zip.*v1/)).toBeInTheDocument();
  });
  it("opens the existing instance configuration from its card", async () => {
    render(
      <App
        initialInstances={[instance]}
        initialPackages={[
          {
            id: "package-a",
            filename: "coop-a.zip",
            version: "v1",
            size: 1024,
            hot_compatible: true,
          },
        ]}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "配置 深夜战役" }),
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText("额外 SRCDS 启动项")).toHaveValue(
      "-strictportbind",
    );
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
  it("submits SourceTV and plugin ports when creating an instance", async () => {
    let submitted: Record<string, unknown> | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "POST") {
          submitted = JSON.parse(String(init.body));
        }
        return new Response(init?.method === "POST" ? "{}" : "[]", {
          status: 201,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(
      <App
        initialInstances={[]}
        initialPackages={[
          {
            id: "package-a",
            filename: "coop-a.zip",
            version: "v1",
            size: 1024,
            hot_compatible: true,
          },
        ]}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "创建实例" }));
    await userEvent.type(screen.getByLabelText("名称"), "端口测试");
    await userEvent.clear(screen.getByLabelText("SourceTV 端口"));
    await userEvent.type(screen.getByLabelText("SourceTV 端口"), "27020");
    await userEvent.type(screen.getByLabelText("插件端口"), "27021, 27022");
    await userEvent.click(screen.getByRole("button", { name: "创建" }));
    expect(submitted).toMatchObject({
      sourcetv_port: 27020,
      plugin_ports: [27021, 27022],
      package_id: "package-a",
    });
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

  it("reports the real control-node health", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        if (path === "/api/session") {
          return new Response('{"authenticated":true}', {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (path === "/api/instances") {
          return new Response("[]", {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response(
          JSON.stringify({ error: { message: "Docker proxy unavailable" } }),
          { status: 503, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
    render(<App />);
    expect(await screen.findByText("控制节点异常")).toBeInTheDocument();
    expect(screen.getByText("Docker proxy unavailable")).toBeInTheDocument();
  });

  it("shows Cron save success and sends the snake-case contract", async () => {
    let saved = false;
    let submitted: Record<string, unknown> | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input);
        if (path === "/api/schedules" && init?.method === "POST") {
          submitted = JSON.parse(String(init.body));
          saved = true;
          return new Response(JSON.stringify({ id: "schedule-1" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        const body =
          path === "/api/schedules" && saved
            ? '[{"id":"schedule-1","type":"game_update","cron":"0 4 * * *","timezone":"Asia/Hong_Kong","enabled":true}]'
            : "[]";
        return new Response(body, {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "计划任务" }));
    await userEvent.click(screen.getByRole("button", { name: "保存计划" }));
    expect(await screen.findByRole("status")).toHaveTextContent("计划已保存");
    expect(submitted).toMatchObject({
      instance_id: "1",
      online_policy: "skip",
    });
  });

  it("shows Cron save errors inline", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) =>
        init?.method === "POST"
          ? new Response(
              JSON.stringify({ error: { message: "invalid Cron expression" } }),
              { status: 422, headers: { "Content-Type": "application/json" } },
            )
          : new Response("[]", {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
      ),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "计划任务" }));
    await userEvent.click(screen.getByRole("button", { name: "保存计划" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "invalid Cron expression",
    );
  });

  it("describes Steam credentials as optional for anonymous installs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response('{"configured":false}', {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "系统设置" }));
    expect(
      await screen.findByText("匿名首装已支持；仅许可账号需要配置凭据"),
    ).toBeInTheDocument();
  });

  it("confirms game updates before submitting them", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response('{"ID":"job-1","Status":"pending"}', {
        status: 202,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "更新" }));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toHaveTextContent("更新游戏");
    await userEvent.click(
      screen.getByRole("button", { name: "确认更新游戏" }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/instances/1/game-update",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ confirm: true }),
      }),
    );
  });

  it("confirms full package updates before submitting them", async () => {
    const calls: Array<[RequestInfo | URL, RequestInit | undefined]> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push([input, init]);
        const path = String(input);
        const body = path === "/api/packages"
          ? '[{"id":"pkg-1","filename":"plugins.zip","version":"v1","size":4,"hot_compatible":true}]'
          : "[]";
        return new Response(body, {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    await userEvent.click(
      await screen.findByRole("button", { name: "完整更新" }),
    );
    expect(calls.some(([, init]) => init?.method === "POST")).toBe(false);
    await userEvent.click(
      screen.getByRole("button", { name: "确认完整更新" }),
    );
    expect(
      calls.some(
        ([path, init]) =>
          String(path) === "/api/instances/1/updates" &&
          init?.method === "POST" &&
          String(init.body).includes('"confirm":true'),
      ),
    ).toBe(true);
  });

  it("checks the configured GitHub Release without applying it", async () => {
    const calls: Array<[RequestInfo | URL, RequestInit | undefined]> = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push([input, init]);
      const path = String(input);
      const response = path === "/api/github-sources" ? '[{"id":"default","name":"默认源","repository":"owner/repo","asset_pattern":"^plugins[.]zip$"}]' : path.endsWith("/check") ? '{"ID":"job-1","Status":"pending"}' : "[]";
      return new Response(response, {
        status: path.endsWith("/check") ? 202 : 200,
        headers: { "Content-Type": "application/json" },
      });
    }));
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    await userEvent.click(await screen.findByRole("button", { name: "检查更新 默认源" }));
    expect(calls).toContainEqual([
      "/api/github-sources/default/check",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({}),
      }),
    ]);
  });

  it("saves independent scheduled Release update modes", async () => {
    let submitted: any;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/schedules" && init?.method === "POST") submitted = JSON.parse(String(init.body));
      const response = path === "/api/github-sources" ? '[{"id":"source-1","name":"源一","repository":"owner/repo","asset_pattern":"^plugins[.]zip$"}]' : init?.method === "POST" ? "{}" : "[]";
      return new Response(response, { status: 200, headers: { "Content-Type": "application/json" } });
    }));
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "计划任务" }));
    await userEvent.selectOptions(screen.getByLabelText("任务"), "release_hot");
    expect(screen.getByLabelText("GitHub 源")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "保存计划" }));
    expect(submitted.type).toBe("release_hot");
    expect(JSON.parse(submitted.payload)).toEqual({ source_id: "source-1" });
  });

  it("confirms player kicks and bans before submitting them", async () => {
    const calls: Array<[RequestInfo | URL, RequestInit | undefined]> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push([input, init]);
        if (String(input).endsWith("/players") && !init?.method) {
          return new Response(
            '{"players":[{"name":"Ellis","user_id":7,"score":2}]}',
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response('{"ID":"job-1","Status":"pending"}', {
          status: 202,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "玩家" }));
    await userEvent.click(await screen.findByRole("button", { name: "踢出" }));
    expect(calls.some(([, init]) => init?.method === "POST")).toBe(false);
    await userEvent.click(screen.getByRole("button", { name: "确认踢出" }));
    expect(calls.some(([, init]) => init?.method === "POST")).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: "永久封禁" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("永久封禁 Ellis");
    await userEvent.click(screen.getByRole("button", { name: "确认永久封禁" }));
    expect(
      calls.some(([, init]) =>
        String(init?.body).includes('"action":"ban"'),
      ),
    ).toBe(true);
  });

  it("treats interrupted jobs as terminal and keeps their error visible", async () => {
    vi.useFakeTimers();
    let jobReads = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input);
        if (init?.method === "POST") {
          return new Response('{"ID":"job-1","Status":"pending"}', {
            status: 202,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (path === "/api/jobs/job-1") {
          jobReads += 1;
          return new Response(
            '{"ID":"job-1","Status":"interrupted","Error":"Panel restarted; inspect and retry"}',
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response("[]", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[{ ...instance, actual_state: "stopped" }]} />);
    fireEvent.click(screen.getByRole("button", { name: "启动" }));
    await act(async () => {
      await Promise.resolve();
      vi.advanceTimersByTime(1_700);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(jobReads).toBe(1);
    expect(screen.getByText("Panel restarted; inspect and retry")).toBeInTheDocument();
  });

  it("hashes and uploads VPK files in sequential 8 MiB chunks", async () => {
    const chunkSize = 8 * 1024 * 1024;
    const patchCalls: Array<{ offset: number; size: number }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input);
        if (path === "/api/content/vpk/uploads" && init?.method === "POST") {
          return new Response('{"id":"upload-1"}', {
            status: 201,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (init?.method === "PATCH") {
          patchCalls.push({
            offset: Number(new URL(path, "http://panel.test").searchParams.get("offset")),
            size: (init.body as Blob).size,
          });
          return new Response("{}", {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response(path.includes("/complete") ? "{}" : "[]", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    const wholeRead = vi.fn(() => Promise.reject(new Error("whole-file read")));
    const fakeFile = {
      name: "maps.vpk",
      size: chunkSize + 3,
      arrayBuffer: wholeRead,
      slice: vi.fn((start: number, end: number) => {
        const size = end - start;
        return {
          size,
          arrayBuffer: async () => new Uint8Array(size).buffer,
        } as Blob;
      }),
    } as unknown as File;
    fireEvent.change(screen.getByLabelText("上传 VPK"), {
      target: { files: [fakeFile] },
    });
    await waitFor(() => expect(patchCalls).toHaveLength(2));
    expect(patchCalls).toEqual([
      { offset: 0, size: chunkSize },
      { offset: chunkSize, size: 3 },
    ]);
    expect(wholeRead).not.toHaveBeenCalled();
    expect(screen.getByRole("status")).toHaveTextContent("VPK 上传完成");
  });
});
