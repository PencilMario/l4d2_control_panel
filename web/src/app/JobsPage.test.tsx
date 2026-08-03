import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { JobsPage } from "./JobsPage";

class FakeEventSource {
  static latest: FakeEventSource | null = null;
  listeners = new Map<string, EventListener>();
  onerror: ((event: Event) => void) | null = null;

  constructor(readonly url: string) {
    FakeEventSource.latest = this;
  }

  addEventListener(type: string, listener: EventListener) {
    this.listeners.set(type, listener);
  }

  emitJobs(value: unknown) {
    this.listeners
      .get("jobs")
      ?.(new MessageEvent("jobs", { data: JSON.stringify(value) }));
  }

  close() {}
}

describe("JobsPage", () => {
  afterEach(() => {
    FakeEventSource.latest = null;
    vi.unstubAllGlobals();
  });

  it("renders the reference task filters and table columns", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json([])));

    render(<JobsPage />);

    expect(await screen.findByRole("group", { name: "任务状态筛选" })).toBeVisible();
    expect(screen.getByRole("group", { name: "任务状态筛选" }).closest(".job-filters")).toBeVisible();
    expect(screen.getByRole("searchbox", { name: "搜索任务" })).toBeVisible();
    for (const column of ["任务编号", "任务类型", "目标对象", "阶段 / 进度", "状态", "创建时间", "操作"]) {
      expect(screen.getByRole("columnheader", { name: column })).toBeVisible();
    }
  });

  it("shows reference status styling and a progress bar for a running task", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json([{
      ID: "job-running",
      Type: "game_update",
      Status: "running",
      Stage: "download",
      Percent: 42,
	  TimeoutMinutes: 90,
      CreatedAt: "2026-07-16T08:00:00Z",
    }])));

    render(<JobsPage />);

    const progress = await screen.findByRole("progressbar", { name: "game_update 任务进度" });
    expect(progress).toHaveAttribute("aria-valuenow", "42");
    expect(progress.closest(".job-row")?.querySelector(".job-state")).toHaveClass("job-state", "running");
	expect(screen.getByText("超时 90 分钟")).toBeVisible();
    expect(screen.getByRole("button", { name: /查看 game_update 任务日志/ })).toHaveTextContent("事件");
  });

  it("refreshes an expanded running job when its summary timestamp changes", async () => {
    let detailCalls = 0;
    const summary = {
      ID: "job-running",
      Type: "game_update",
      Status: "running",
      Stage: "download",
      Percent: 20,
      Message: "phase one",
      CreatedAt: "2026-07-16T08:00:00Z",
      UpdatedAt: "2026-07-16T08:00:10Z",
      StartedAt: "2026-07-16T08:00:02Z",
    };
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) === "/api/jobs") {
        return new Response(JSON.stringify([summary]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      detailCalls += 1;
      return new Response(
        JSON.stringify({
          ...summary,
          UpdatedAt:
            detailCalls === 1
              ? "2026-07-16T08:00:10Z"
              : "2026-07-16T08:00:20Z",
          Events: [
            {
              ID: detailCalls,
              JobID: summary.ID,
              Kind: "progress",
              Stage: "download",
              Percent: detailCalls === 1 ? 20 : 40,
              Message: detailCalls === 1 ? "phase one" : "phase two",
              CreatedAt:
                detailCalls === 1
                  ? "2026-07-16T08:00:10Z"
                  : "2026-07-16T08:00:20Z",
            },
          ],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("EventSource", FakeEventSource);

    render(<JobsPage />);
    await userEvent.click(
      await screen.findByRole("button", {
        name: "查看 game_update 任务日志，任务 ID job-running",
      }),
    );
    expect(await screen.findByText("phase one")).toBeVisible();

    FakeEventSource.latest?.emitJobs([
      {
        ...summary,
        Percent: 40,
        Message: "phase two",
        UpdatedAt: "2026-07-16T08:00:20Z",
      },
    ]);

    await waitFor(() => expect(detailCalls).toBe(2));
    expect(await screen.findByText("phase two")).toBeVisible();
  });

  it("collapses a terminal job and reuses its cached details", async () => {
    const summary = {
      ID: "job-failed",
      Type: "game_update",
      Status: "failed",
      Stage: "download",
      Percent: 42,
      Error: "download interrupted",
      CreatedAt: "2026-07-16T08:00:00Z",
      UpdatedAt: "2026-07-16T08:02:20Z",
      StartedAt: "2026-07-16T08:00:02Z",
      FinishedAt: "2026-07-16T08:02:20Z",
    };
    let detailCalls = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input) === "/api/jobs") {
          return Response.json([summary]);
        }
        detailCalls += 1;
        return Response.json({
          ...summary,
          Events: [
            {
              ID: 1,
              JobID: summary.ID,
              Kind: "failed",
              Stage: "download",
              Percent: 42,
              Message: "download interrupted",
              CreatedAt: "2026-07-16T08:02:20Z",
            },
          ],
        });
      }),
    );

    render(<JobsPage />);
    const toggle = await screen.findByRole("button", {
      name: "查看 game_update 任务日志，任务 ID job-failed",
    });
    await userEvent.click(toggle);
    expect(await screen.findByText("任务失败")).toBeVisible();
    expect(toggle).toHaveAttribute("aria-expanded", "true");

    await userEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(
      screen.queryByRole("region", { name: "game_update 任务日志" }),
    ).not.toBeInTheDocument();

    await userEvent.click(toggle);
    expect(await screen.findByText("任务失败")).toBeVisible();
    expect(detailCalls).toBe(1);
  });

  it("shows a terminal job that never started as not executed", async () => {
    const summary = {
      ID: "job-start-failed",
      Type: "install",
      Status: "failed",
      Stage: "failed",
      Percent: 0,
      Error: "running state could not be persisted",
      CreatedAt: "2026-07-16T08:00:00Z",
      UpdatedAt: "2026-07-16T08:00:05Z",
      FinishedAt: "2026-07-16T08:00:05Z",
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) =>
        Response.json(
          String(input) === "/api/jobs"
            ? [summary]
            : {
                ...summary,
                Events: [
                  {
                    ID: 1,
                    JobID: summary.ID,
                    Kind: "failed",
                    Stage: "failed",
                    Percent: 0,
                    Message: summary.Error,
                    CreatedAt: summary.FinishedAt,
                  },
                ],
              },
        ),
      ),
    );

    render(<JobsPage />);
    const toggle = await screen.findByRole("button", {
      name: "查看 install 任务日志，任务 ID job-start-failed",
    });
    expect(toggle.closest(".job-row")).toHaveTextContent("未执行");

    await userEvent.click(toggle);
    const details = await screen.findByRole("region", {
      name: "install 任务日志",
    });
    expect(details).toHaveTextContent("排队耗时 5秒");
    expect(details).toHaveTextContent("执行用时 未执行");
  });

  it("keeps only one task log expanded", async () => {
    const jobs = [
      {
        ID: "job-one",
        Type: "game_update",
        Status: "succeeded",
        Stage: "complete",
        Percent: 100,
        Error: "",
        CreatedAt: "2026-07-16T08:00:00Z",
        UpdatedAt: "2026-07-16T08:01:00Z",
        StartedAt: "2026-07-16T08:00:01Z",
        FinishedAt: "2026-07-16T08:01:00Z",
      },
      {
        ID: "job-two",
        Type: "plugin_update",
        Status: "succeeded",
        Stage: "complete",
        Percent: 100,
        Error: "",
        CreatedAt: "2026-07-16T09:00:00Z",
        UpdatedAt: "2026-07-16T09:01:00Z",
        StartedAt: "2026-07-16T09:00:01Z",
        FinishedAt: "2026-07-16T09:01:00Z",
      },
    ];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input) === "/api/jobs") return Response.json(jobs);
        const id = String(input).endsWith("job-one") ? "job-one" : "job-two";
        const detail = jobs.find((job) => job.ID === id)!;
        return Response.json({
          ...detail,
          Events: [
            {
              ID: id === "job-one" ? 1 : 2,
              JobID: id,
              Kind: "succeeded",
              Stage: "complete",
              Percent: 100,
              Message: `${id} complete`,
              CreatedAt: detail.FinishedAt,
            },
          ],
        });
      }),
    );

    const onOpenLogs = vi.fn();
    render(<JobsPage onOpenLogs={onOpenLogs} />);
    await userEvent.click(
      await screen.findByRole("button", {
        name: "查看 game_update 任务日志，任务 ID job-one",
      }),
    );
    expect(
      await screen.findByRole("region", { name: "game_update 任务日志" }),
    ).toHaveTextContent("job-one complete");
    expect(screen.getByText("任务事件链:")).toBeVisible();
    await userEvent.click(screen.getAllByRole("button", { name: /打开 .* 完整任务日志/ })[0]);
    expect(onOpenLogs).toHaveBeenCalledWith(expect.objectContaining({ ID: "job-one" }));

    await userEvent.click(
      screen.getByRole("button", {
        name: "查看 plugin_update 任务日志，任务 ID job-two",
      }),
    );
    expect(
      await screen.findByRole("region", { name: "plugin_update 任务日志" }),
    ).toHaveTextContent("job-two complete");
    expect(screen.queryByText("job-one complete")).not.toBeInTheDocument();
  });

  it("distinguishes log controls for jobs with the same type", async () => {
    const jobs = [
      {
        ID: "apply-private-one",
        Type: "apply_private",
        Status: "succeeded",
        Stage: "complete",
        Percent: 100,
        Error: "",
      },
      {
        ID: "apply-private-two",
        Type: "apply_private",
        Status: "succeeded",
        Stage: "complete",
        Percent: 100,
        Error: "",
      },
    ];
    vi.stubGlobal("fetch", vi.fn(async () => Response.json(jobs)));

    render(<JobsPage />);
    expect(
      await screen.findByRole("button", {
        name: "查看 apply_private 任务日志，任务 ID apply-private-one",
      }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", {
        name: "查看 apply_private 任务日志，任务 ID apply-private-two",
      }),
    ).toBeVisible();
  });

  it("retries a failed detail request", async () => {
    const summary = {
      ID: "job-retry",
      Type: "game_update",
      Status: "failed",
      Stage: "download",
      Percent: 10,
      Error: "network unavailable",
      CreatedAt: "2026-07-16T08:00:00Z",
      UpdatedAt: "2026-07-16T08:00:10Z",
      StartedAt: "2026-07-16T08:00:01Z",
      FinishedAt: "2026-07-16T08:00:10Z",
    };
    let detailCalls = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (String(input) === "/api/jobs") return Response.json([summary]);
        detailCalls += 1;
        if (detailCalls === 1) {
          return Response.json(
            { error: { message: "日志暂时不可用" } },
            { status: 503 },
          );
        }
        return Response.json({ ...summary, Events: [] });
      }),
    );

    render(<JobsPage />);
    await userEvent.click(
      await screen.findByRole("button", {
        name: "查看 game_update 任务日志，任务 ID job-retry",
      }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "日志暂时不可用",
    );
    await userEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(
      await screen.findByText("此任务没有可显示的结构化事件"),
    ).toBeVisible();
    expect(detailCalls).toBe(2);
  });
});
