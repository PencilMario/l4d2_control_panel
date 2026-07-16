import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Job } from "../api/client";
import { JobLogsPage } from "./JobLogsPage";

class FakeEventSource {
  static latest: FakeEventSource | null = null;
  listeners: Record<string, (event: MessageEvent<string>) => void> = {};
  onerror: (() => void) | null = null;
  constructor(public url: string) {
    FakeEventSource.latest = this;
  }
  addEventListener(name: string, listener: EventListener) {
    this.listeners[name] = listener as (event: MessageEvent<string>) => void;
  }
  close() {}
  emit(record: unknown) {
    this.listeners["job-log"]?.({ data: JSON.stringify(record) } as MessageEvent<string>);
  }
}

const job: Job = {
  ID: "job-1",
  InstanceID: "abc",
  Type: "start",
  Status: "running",
  Stage: "running",
  Percent: 0,
  Error: "",
};

describe("JobLogsPage", () => {
  beforeEach(() => {
    FakeEventSource.latest = null;
    vi.stubGlobal("EventSource", FakeEventSource);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        if (path.includes("/download")) {
          return new Response("download", { status: 200 });
        }
        return new Response(
          JSON.stringify({
            records: [
              { seq: 1, timestamp: "2026-07-16T10:00:00Z", source: "task", level: "info", message: "Task started" },
              { seq: 2, timestamp: "2026-07-16T10:00:01Z", source: "steamcmd", level: "output", message: "Downloading update" },
            ],
            next_seq: 2,
            truncated: false,
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
  });

  it("loads history, filters it, and appends live records", async () => {
    render(<JobLogsPage job={job} onBack={vi.fn()} />);
    expect(await screen.findByText("Downloading update")).toBeVisible();
    expect(FakeEventSource.latest?.url).toContain("after_seq=2");
    fireEvent.change(screen.getByLabelText("搜索任务日志"), { target: { value: "started" } });
    expect(await screen.findByText("Task started")).toBeVisible();
    expect(screen.queryByText("Downloading update")).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("搜索任务日志"), { target: { value: "" } });
    FakeEventSource.latest?.emit({ seq: 3, timestamp: "2026-07-16T10:00:02Z", source: "steamcmd", level: "error", message: "Missing configuration" });
    expect(await screen.findByText("Missing configuration")).toBeVisible();
  });

  it("supports back, copy, and download commands", async () => {
    const onBack = vi.fn();
    const writeText = vi.fn(async () => undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    vi.stubGlobal("URL", { createObjectURL: vi.fn(() => "blob:test"), revokeObjectURL: vi.fn() });
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    render(<JobLogsPage job={job} onBack={onBack} />);
    await screen.findByText("Downloading update");
    fireEvent.click(screen.getByRole("button", { name: "返回任务列表" }));
    expect(onBack).toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "复制当前日志" }));
    await waitFor(() => expect(writeText).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: "下载完整日志" }));
    await waitFor(() => expect(click).toHaveBeenCalled());
  });

  it("anchors the resume control to the log output", async () => {
    render(<JobLogsPage job={job} onBack={vi.fn()} />);
    const output = (await screen.findByText("Downloading update")).closest("pre")!;
    Object.defineProperties(output, {
      scrollHeight: { configurable: true, value: 1000 },
      clientHeight: { configurable: true, value: 360 },
      scrollTop: { configurable: true, writable: true, value: 0 },
    });
    fireEvent.scroll(output);
    FakeEventSource.latest?.emit({ seq: 3, timestamp: "2026-07-16T10:00:02Z", source: "steamcmd", level: "output", message: "More output" });

    const resume = await screen.findByRole("button", { name: "恢复跟随 · 1 条新日志" });
    expect(output.parentElement).toHaveClass("task-log-output-wrap");
    expect(output.parentElement).toContainElement(resume);
  });
});
