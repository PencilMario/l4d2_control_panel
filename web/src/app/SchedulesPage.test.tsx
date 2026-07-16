import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SchedulesPage } from "./SchedulesPage";

const instances = [
  { id: "instance-1", name: "深夜战役" },
  { id: "instance-2", name: "备用服务器" },
];

const packages = [
  {
    id: "package-hot",
    filename: "plugins-hot.zip",
    version: "v2.1.0",
    size: 1024,
    hot_compatible: true,
  },
  {
    id: "package-full",
    filename: "plugins-full.zip",
    version: "v3.0.0",
    size: 2048,
    hot_compatible: false,
  },
];

const gameUpdateTask = {
  id: "schedule-1",
  instance_id: "instance-1",
  type: "game_update",
  cron: "0 4 * * *",
  timezone: "Asia/Hong_Kong",
  online_policy: "skip",
  payload: "{}",
  enabled: true,
  last_run: "2026-07-15T20:00:00Z",
  next_run: "2026-07-16T20:00:00Z",
};

const json = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function mockSchedules(initial = [gameUpdateTask]) {
  let tasks = [...initial];
  const requests: Array<{ path: string; init?: RequestInit }> = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      requests.push({ path, init });
      if (path === "/api/github-sources") {
        return json([
          {
            id: "source-1",
            name: "默认插件源",
            repository: "owner/repository",
            asset_pattern: "^plugins[.]zip$",
          },
        ]);
      }
      if (path === "/api/schedules" && init?.method === "POST") {
        const submitted = JSON.parse(String(init.body));
        const task = {
          ...submitted,
          id: submitted.id || `schedule-${tasks.length + 1}`,
          last_run: submitted.last_run || "0001-01-01T00:00:00Z",
          next_run: "2026-07-17T20:00:00Z",
        };
        const index = tasks.findIndex((item) => item.id === task.id);
        if (index >= 0) tasks[index] = task;
        else tasks.push(task);
        return json(task);
      }
      if (path.startsWith("/api/schedules/") && init?.method === "DELETE") {
        const id = path.split("/").at(-1);
        tasks = tasks.filter((task) => task.id !== id);
        return new Response(null, { status: 204 });
      }
      if (path === "/api/schedules") return json(tasks);
      return json([]);
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return { fetchMock, requests };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("SchedulesPage", () => {
  it("shows detailed descriptions for all eight task types", async () => {
    mockSchedules([]);
    const user = userEvent.setup();
    render(<SchedulesPage instances={instances} packages={packages} />);

    await user.click(await screen.findByRole("button", { name: "任务说明" }));
    const dialog = screen.getByRole("dialog", { name: "计划任务类型说明" });
    for (const label of [
      "游戏更新",
      "插件热更新",
      "插件完整更新",
      "仅同步 GitHub 源",
      "GitHub Release 热更新",
      "GitHub Release 完整更新",
      "备份",
      "清理",
    ]) {
      expect(within(dialog).getByRole("heading", { name: label })).toBeVisible();
    }
    expect(dialog).toHaveTextContent("在线玩家策略");
    expect(dialog).toHaveTextContent("不会取消已经排队或正在执行的任务");
    expect(dialog).toHaveTextContent("保留天数");

    await user.click(within(dialog).getByRole("button", { name: "关闭任务说明" }));
    expect(screen.queryByRole("dialog", { name: "计划任务类型说明" })).not.toBeInTheDocument();
  });

  it("edits only Cron, player policy, and enabled state while preserving identity", async () => {
    const { requests } = mockSchedules();
    const user = userEvent.setup();
    render(<SchedulesPage instances={instances} packages={packages} />);

    await user.click(await screen.findByRole("button", { name: "编辑 游戏更新" }));
    expect(screen.getByLabelText("任务")).toBeDisabled();
    expect(screen.getByLabelText("实例")).toBeDisabled();
    expect(screen.getByLabelText("任务时区")).toHaveTextContent("Asia/Hong_Kong");

    await user.clear(screen.getByLabelText("Cron"));
    await user.type(screen.getByLabelText("Cron"), "30 5 * * *");
    await user.selectOptions(screen.getByLabelText("在线玩家策略"), "wait");
    await user.click(screen.getByLabelText("启用计划"));
    await user.click(screen.getByRole("button", { name: "保存修改" }));

    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent("计划已更新"),
    );
    const update = requests.find(
      ({ path, init }) => path === "/api/schedules" && init?.method === "POST",
    );
    expect(JSON.parse(String(update?.init?.body))).toEqual({
      id: "schedule-1",
      instance_id: "instance-1",
      type: "game_update",
      cron: "30 5 * * *",
      timezone: "Asia/Hong_Kong",
      online_policy: "wait",
      payload: "{}",
      enabled: false,
      last_run: "2026-07-15T20:00:00Z",
    });
  });

  it("requires confirmation before deleting and removes the task after success", async () => {
    const { requests } = mockSchedules();
    const user = userEvent.setup();
    render(<SchedulesPage instances={instances} packages={packages} />);

    await user.click(await screen.findByRole("button", { name: "删除 游戏更新" }));
    const dialog = screen.getByRole("dialog", { name: "删除游戏更新计划？" });
    expect(dialog).toHaveTextContent("已经排队或正在执行的任务不会被取消");
    expect(
      requests.some(({ init }) => init?.method === "DELETE"),
    ).toBe(false);

    await user.click(within(dialog).getByRole("button", { name: "确认删除计划" }));
    expect(await screen.findByText("暂无计划任务")).toBeVisible();
    expect(
      requests.some(
        ({ path, init }) =>
          path === "/api/schedules/schedule-1" && init?.method === "DELETE",
      ),
    ).toBe(true);
  });

  it("creates hot-package schedules with the selected compatible package", async () => {
    const { requests } = mockSchedules([]);
    const user = userEvent.setup();
    render(<SchedulesPage instances={instances} packages={packages} />);

    await user.selectOptions(await screen.findByLabelText("任务"), "package_hot");
    const packageSelect = screen.getByLabelText("插件包");
    expect(within(packageSelect).getByRole("option", { name: "plugins-hot.zip · v2.1.0" })).toBeVisible();
    expect(within(packageSelect).queryByRole("option", { name: "plugins-full.zip · v3.0.0" })).not.toBeInTheDocument();
    await user.selectOptions(packageSelect, "package-hot");
    await user.click(screen.getByRole("button", { name: "保存计划" }));

    await screen.findByRole("status");
    const create = requests.find(
      ({ path, init }) => path === "/api/schedules" && init?.method === "POST",
    );
    const body = JSON.parse(String(create?.init?.body));
    expect(body.type).toBe("package_hot");
    expect(JSON.parse(body.payload)).toEqual({ package_id: "package-hot" });
  });

  it("creates cleanup schedules with an explicit retention period", async () => {
    const { requests } = mockSchedules([]);
    const user = userEvent.setup();
    render(<SchedulesPage instances={instances} packages={packages} />);

    await user.selectOptions(await screen.findByLabelText("任务"), "cleanup");
    expect(screen.queryByLabelText("实例")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("在线玩家策略")).not.toBeInTheDocument();
    await user.clear(screen.getByLabelText("保留天数"));
    await user.type(screen.getByLabelText("保留天数"), "45");
    await user.click(screen.getByRole("button", { name: "保存计划" }));

    await screen.findByRole("status");
    const create = requests.find(
      ({ path, init }) => path === "/api/schedules" && init?.method === "POST",
    );
    const body = JSON.parse(String(create?.init?.body));
    expect(body.instance_id).toBe("");
    expect(body.online_policy).toBe("force");
    expect(JSON.parse(body.payload)).toEqual({ retention_days: 45 });
  });
});
