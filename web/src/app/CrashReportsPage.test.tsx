import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CrashReportsPage, type CrashReport } from "./CrashReportsPage";

const report: CrashReport = {
  id: "a".repeat(64),
  instance_id: "i1",
  received_at: "2026-08-09T10:00:00Z",
  updated_at: "2026-08-09T10:01:00Z",
  minidump_size: 4096,
  metadata_size: 128,
  sha256: "a".repeat(64),
  user_id: "steam-user",
  game_directory: "left4dead2",
  extension_version: "1.0.0",
  server_id: "server-1",
  crash_signature: "2|2026-08-09|linux|x86_64|1|SIGSEGV|0x10|M|server|ABC|F|0|0x10",
  parsed_signature: {
    version: 2,
    platform: "linux",
    architecture: "x86_64",
    crashed: "1",
    crash_reason: "SIGSEGV",
    crash_address: "0x10",
    modules: [{ index: 0, debug_file: "server", debug_identifier: "ABC", decision: "N" }],
    frames: [{ module_index: 0, offset: "0x10" }],
  },
  modules: [{ index: 0, debug_file: "server", debug_identifier: "ABC", decision: "N" }],
  stackwalk_status: "succeeded",
  stackwalk_tool: "minidump_stackwalk",
  ai_status: "succeeded",
  ai_model: "local-model",
  ai_analysis: "# Left 4 Dead 2 崩溃分析\n\n建议检查最近部署的插件。\n\n<mark data-testid=\"unsafe-html\">不应作为 HTML 执行</mark>\n\n![远程图片](https://tracker.invalid/crash-analysis.png)\n\n- 核对 `server.so` 符号\n- 回退最近插件\n\n```text\n#0 server!Crash+0x10\n```",
};

describe("CrashReportsPage", () => {
  it("loads reports, shows online diagnostics, and queues AI analysis", async () => {
    const request = vi.fn(async (path: string) => {
      if (path === "/api/crash-reports") return [report];
      if (path === `/api/crash-reports/${report.id}`) return { ...report, metadata: "hostname coop\nmap c2m1_highway" };
      if (path.endsWith("/analyze")) return { ID: "job-analysis", Status: "queued", Stage: "crash analysis", Percent: 0 };
      throw new Error(`unexpected request ${path}`);
    });
    const textRequest = vi.fn(async (path: string) => {
      if (path.includes("file=stackwalk")) return "#0 server!Crash+0x10\n#1 engine!Run+0x20";
      throw new Error(`unexpected text request ${path}`);
    });

    render(<CrashReportsPage instances={[{ id: "i1", name: "死亡中心" }]} apiRequest={request} textRequest={textRequest} />);

    expect(screen.getByRole("heading", { name: "崩溃报告", level: 1 })).toBeVisible();
    expect(await screen.findByRole("button", { name: /死亡中心/ })).toBeVisible();
    const metadata = (await screen.findByRole("heading", { name: "上传元数据", level: 3 })).closest("section");
    expect(metadata?.querySelector("pre")?.textContent).toContain("hostname coop");
    const stackwalk = (await screen.findByRole("heading", { name: "Stackwalk", level: 3 })).closest("section");
    expect(stackwalk?.querySelector("pre")?.textContent).toContain("#0 server!Crash+0x10");
    expect(screen.getByRole("button", { name: "查看 AI 分析" })).toBeVisible();
    expect(screen.getByRole("table", { name: "崩溃模块" })).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "重新分析" }));
    await waitFor(() => expect(request).toHaveBeenCalledWith(
      `/api/crash-reports/${report.id}/analyze`,
      expect.objectContaining({ method: "POST", body: JSON.stringify({ ai: true }) }),
    ));
    expect(await screen.findByText("分析任务已提交")).toBeVisible();
  });

  it("opens the full Markdown AI analysis reader and returns to the selected report", async () => {
    const request = vi.fn(async (path: string) => {
      if (path === "/api/crash-reports") return [report];
      if (path === `/api/crash-reports/${report.id}`) return { ...report, metadata: "hostname coop" };
      throw new Error(`unexpected request ${path}`);
    });

    render(<CrashReportsPage instances={[{ id: "i1", name: "死亡中心" }]} apiRequest={request} textRequest={vi.fn().mockResolvedValue("")} />);

    fireEvent.click(await screen.findByRole("button", { name: "查看 AI 分析" }));
    expect(await screen.findByRole("heading", { name: "Left 4 Dead 2 崩溃分析", level: 1 })).toBeVisible();
    expect(screen.getByRole("button", { name: "返回崩溃详情" })).toHaveFocus();
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(screen.getByRole("list")).toHaveTextContent("核对 server.so 符号");
    expect(screen.getByText("#0 server!Crash+0x10")).toBeVisible();
    expect(screen.queryByTestId("unsafe-html")).not.toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "上传元数据", level: 3 })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "返回崩溃详情" }));
    expect(await screen.findByRole("heading", { name: "上传元数据", level: 3 })).toBeVisible();
    expect(screen.getByRole("button", { name: "查看 AI 分析" })).toHaveFocus();
  });

  it("filters reports by instance and signature", async () => {
    const second = { ...report, id: "b".repeat(64), sha256: "b".repeat(64), instance_id: "i2", crash_signature: "2|other|windows|x64|1|ACCESS_VIOLATION" };
    const request = vi.fn(async (path: string) => {
      if (path === "/api/crash-reports") return [report, second];
      if (path.includes(`/api/crash-reports/${report.id}`)) return { ...report, metadata: "first" };
      if (path.includes(`/api/crash-reports/${second.id}`)) return { ...second, metadata: "second" };
      throw new Error(`unexpected request ${path}`);
    });
    render(<CrashReportsPage instances={[{ id: "i1", name: "一号" }, { id: "i2", name: "二号" }]} apiRequest={request} textRequest={vi.fn().mockResolvedValue("")} />);

    await screen.findByText("一号");
    const list = screen.getByRole("region", { name: "崩溃报告列表" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(2);
    fireEvent.change(screen.getByRole("combobox", { name: "筛选实例" }), { target: { value: "i2" } });
    expect(within(list).getAllByRole("listitem")).toHaveLength(1);
    fireEvent.change(screen.getByRole("searchbox", { name: "搜索崩溃签名" }), { target: { value: "ACCESS_VIOLATION" } });
    expect(within(list).getAllByRole("listitem")).toHaveLength(1);
    expect(await screen.findByText("second")).toBeVisible();
  });
});
