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
      if (path.includes("file=stackwalk")) return "#0 server!Crash+0x10\n#1 engine!Run+0x20\n    Found by: stack scanning";
      throw new Error(`unexpected text request ${path}`);
    });

    render(<CrashReportsPage instances={[{ id: "i1", name: "死亡中心" }]} apiRequest={request} textRequest={textRequest} />);

    expect(screen.getByRole("heading", { name: "崩溃报告", level: 1 })).toBeVisible();
    expect(await screen.findByRole("button", { name: /死亡中心/ })).toBeVisible();
    const diagnostics = await screen.findByRole("list", { name: "崩溃诊断" });
    expect(diagnostics.querySelectorAll(":scope > .crash-diagnostic-row")).toHaveLength(4);
    expect(screen.queryByRole("list", { name: "崩溃诊断" })?.querySelector(".crash-analysis-grid")).not.toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "崩溃诊断" })?.querySelector(".crash-data-grid")).not.toBeInTheDocument();
    expect(within(diagnostics).getAllByRole("heading", { level: 3 }).map((heading) => heading.textContent)).toEqual([
      "Stackwalk",
      "AI 诊断",
      "上传元数据",
      "崩溃模块",
    ]);
    const stackwalk = within(diagnostics).getByRole("listitem", { name: /Stackwalk/ });
    expect(within(stackwalk).getAllByRole("listitem")).toHaveLength(2);
    expect(within(stackwalk).getByText("server")).toBeVisible();
    expect(within(stackwalk).getByText("Crash")).toBeVisible();
    expect(within(stackwalk).getByText("0x10")).toBeVisible();
    expect(within(stackwalk).getByText("来源：stack scanning")).toBeVisible();
    const metadata = within(diagnostics).getByRole("listitem", { name: /上传元数据/ });
    expect(metadata).toHaveAttribute("data-expanded", "false");
    fireEvent.click(within(metadata).getByRole("button", { name: "展开上传元数据" }));
    expect(metadata.querySelector("pre")).toHaveTextContent("hostname coop");
    const modules = within(diagnostics).getByRole("listitem", { name: /崩溃模块/ });
    expect(modules).toHaveAttribute("data-expanded", "false");
    fireEvent.click(within(modules).getByRole("button", { name: "展开崩溃模块" }));
    expect(screen.getByRole("table", { name: "崩溃模块" })).toBeVisible();
    expect(screen.getByRole("button", { name: "查看 AI 分析" })).toBeVisible();

    expect(request.mock.calls.filter(([path]) => path.endsWith("/analyze"))).toHaveLength(0);

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

    const list = screen.getByRole("region", { name: "崩溃报告列表" });
    await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(2));
    fireEvent.change(screen.getByRole("combobox", { name: "筛选实例" }), { target: { value: "i2" } });
    await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(1));
    fireEvent.change(screen.getByRole("searchbox", { name: "搜索崩溃签名" }), { target: { value: "ACCESS_VIOLATION" } });
    await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(1));
    const metadata = await screen.findByRole("listitem", { name: /上传元数据/ });
    fireEvent.click(within(metadata).getByRole("button", { name: "展开上传元数据" }));
    expect(await screen.findByText("second")).toBeVisible();
  });
});
