import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import {
  PerformancePanel,
  buildChartData,
  LINE_CONNECTS_NULLS,
  formatBytes,
  formatBytesPerSecond,
  formatDuration,
  formatLatency,
  formatPercent,
  performancePanelPropsEqual,
  type PerformanceSnapshot,
  type PerformanceHistoryPoint,
} from "./PerformancePanel";

const snapshot: PerformanceSnapshot = {
  image_size_bytes: 5 * 1024 ** 3,
  cpu_percent: 0,
  memory_bytes: 0,
  memory_limit_bytes: 2 * 1024 ** 3,
  memory_percent: 0,
  network_rx_bytes_per_sec: 0,
  network_tx_bytes_per_sec: null,
  network_rx_bytes: 0,
  network_tx_bytes: 1024,
  block_read_bytes_per_sec: 2048,
  block_write_bytes_per_sec: null,
  block_read_bytes: 4096,
  block_write_bytes: 8192,
  pids: 0,
  uptime_seconds: 3661,
  a2s_latency_ms: 0,
};

const history: PerformanceHistoryPoint[] = [
  { at: "2026-07-15T12:00:00Z", run_id: "r1", cpu_percent: 0, memory_percent: 0, network_rx_bytes_per_sec: null, network_tx_bytes_per_sec: null, block_read_bytes_per_sec: null, block_write_bytes_per_sec: null },
  { at: "2026-07-15T12:00:05Z", run_id: "r1", cpu_percent: null, memory_percent: null, network_rx_bytes_per_sec: 1024, network_tx_bytes_per_sec: 2048, block_read_bytes_per_sec: 4096, block_write_bytes_per_sec: 8192 },
];

describe("performance formatters", () => {
  it("formats IEC bytes and rates without locale dependence", () => {
    expect(formatBytes(null)).toBe("--");
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(1024)).toBe("1 KiB");
    expect(formatBytes(1536)).toBe("1.5 KiB");
    expect(formatBytesPerSecond(1024)).toBe("1 KiB/s");
    expect(formatBytesPerSecond(null)).toBe("--");
  });
  it("formats duration, latency and percent zeroes", () => {
    expect(formatDuration(0)).toBe("0s");
    expect(formatDuration(3661)).toBe("1h 1m 1s");
    expect(formatDuration(null)).toBe("--");
    expect(formatLatency(0)).toBe("0 ms");
    expect(formatLatency(null)).toBe("--");
    expect(formatPercent(0)).toBe("0%");
    expect(formatPercent(null)).toBe("--");
  });
  it("preserves null chart gaps", () => {
    expect(buildChartData(history)[1]).toMatchObject({ cpu: null, memory: null });
    expect(buildChartData(history)[0]).toMatchObject({ rx: null, tx: null });
  });
});

describe("PerformancePanel", () => {
  it("inserts one all-series null gap when a nonempty run ID changes", () => {
    const boundary: PerformanceHistoryPoint[] = [
      { at: "2026-07-15T12:00:00Z", run_id: "run-1", cpu_percent: 1, memory_percent: 2, network_rx_bytes_per_sec: 3, network_tx_bytes_per_sec: 4, block_read_bytes_per_sec: 5, block_write_bytes_per_sec: 6 },
      { at: "2026-07-15T12:00:10Z", run_id: "run-2", cpu_percent: 7, memory_percent: 8, network_rx_bytes_per_sec: 9, network_tx_bytes_per_sec: 10, block_read_bytes_per_sec: 11, block_write_bytes_per_sec: 12 },
    ];
    const data = buildChartData(boundary);
    expect(data).toHaveLength(3);
    expect(data[0]).toMatchObject({ cpu: 1, memory: 2, rx: 3, tx: 4, read: 5, write: 6 });
    expect(data[1]).toMatchObject({ synthetic: true, cpu: null, memory: null, rx: null, tx: null, read: null, write: null });
    expect(data[2]).toMatchObject({ cpu: 7, memory: 8, rx: 9, tx: 10, read: 11, write: 12 });
    expect(Date.parse(data[1].at)).toBe(Date.parse("2026-07-15T12:00:05Z"));
  });

  it("does not add redundant gaps for same/empty runs or an existing all-null point", () => {
    const point = (at: string, run_id: string, cpu: number | null) => ({ at, run_id, cpu_percent: cpu, memory_percent: cpu, network_rx_bytes_per_sec: cpu, network_tx_bytes_per_sec: cpu, block_read_bytes_per_sec: cpu, block_write_bytes_per_sec: cpu });
    expect(buildChartData([point("2026-07-15T12:00:00Z", "same", 1), point("2026-07-15T12:00:05Z", "same", 2)])).toHaveLength(2);
    expect(buildChartData([point("2026-07-15T12:00:00Z", "", 1), point("2026-07-15T12:00:05Z", "run-2", 2)])).toHaveLength(2);
    const stopped = buildChartData([
      point("2026-07-15T12:00:00Z", "run-1", 1),
      point("2026-07-15T12:00:05Z", "run-1", null),
      point("2026-07-15T12:00:10Z", "run-2", 2),
    ]);
    expect(stopped).toHaveLength(3);
    expect(stopped.filter((item) => item.synthetic)).toHaveLength(0);
  });

  it("uses a stable non-interactive fallback when boundary timestamps are equal", () => {
    const base = { at: "2026-07-15T12:00:00Z", cpu_percent: 1, memory_percent: 1, network_rx_bytes_per_sec: 1, network_tx_bytes_per_sec: 1, block_read_bytes_per_sec: 1, block_write_bytes_per_sec: 1 };
    const data = buildChartData([{ ...base, run_id: "run-1" }, { ...base, run_id: "run-2" }]);
    expect(data[1]).toMatchObject({ at: "__run_gap_1", label: "", synthetic: true });
  });

  it("uses transformed boundary data for every mode without connecting nulls", async () => {
    const boundary = [history[0], { ...history[1], run_id: "r2", cpu_percent: 1, memory_percent: 2 }];
    render(<PerformancePanel snapshot={snapshot} history={boundary} />);
    expect(LINE_CONNECTS_NULLS).toBe(false);
    expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-point-count", "3");
    for (const mode of ["内存", "网络", "磁盘", "CPU"]) {
      await userEvent.click(screen.getByRole("button", { name: mode }));
      expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-point-count", "3");
    }
  });
  it("memoizes by history reference, loading and snapshot scalars", () => {
    const historyReference = history;
    expect(performancePanelPropsEqual(
      { snapshot, history: historyReference, loading: false },
      { snapshot: { ...snapshot }, history: historyReference, loading: false },
    )).toBe(true);
    expect(performancePanelPropsEqual(
      { snapshot, history: historyReference, loading: false },
      { snapshot: { ...snapshot, a2s_latency_ms: 1 }, history: historyReference, loading: false },
    )).toBe(false);
    expect(performancePanelPropsEqual(
      { snapshot, history: historyReference, loading: false },
      { snapshot: { ...snapshot, image_size_bytes: 6 * 1024 ** 3 }, history: historyReference, loading: false },
    )).toBe(false);
    expect(performancePanelPropsEqual(
      { snapshot, history: historyReference, loading: false },
      { snapshot: { ...snapshot }, history: [...historyReference], loading: false },
    )).toBe(false);
    expect(performancePanelPropsEqual(
      { snapshot, history: historyReference, loading: false },
      { snapshot: { ...snapshot }, history: historyReference, loading: true },
    )).toBe(false);
  });
  it("renders current metrics and switches chart modes", async () => {
    render(<PerformancePanel snapshot={snapshot} history={history} />);
    expect(screen.getAllByText("CPU")).toHaveLength(3);
    expect(screen.getByText("0%")).toBeInTheDocument();
    expect(screen.getByText("0 B / 2 GiB (0%)")).toBeInTheDocument();
    expect(screen.getByText("下载")).toBeInTheDocument();
    expect(screen.getByText("上传")).toBeInTheDocument();
    expect(screen.getByText("磁盘读")).toBeInTheDocument();
    expect(screen.getByText("磁盘写")).toBeInTheDocument();
    expect(screen.getByText("PID")).toBeInTheDocument();
    expect(screen.getAllByText("0")).toHaveLength(1);
    expect(screen.getByText("1h 1m 1s")).toBeInTheDocument();
    expect(screen.getByText("0 ms")).toBeInTheDocument();
    expect(screen.getByText("总占用")).toBeInTheDocument();
    expect(screen.getByText("5 GiB")).toBeInTheDocument();
    expect(screen.queryByText("玩家")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "网络" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "CPU" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("performance-chart")).toBeInTheDocument();
    expect(screen.getByTestId("performance-chart")).toHaveClass("performance-chart");
    expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-series-count", "1");
    await userEvent.click(screen.getByRole("button", { name: "磁盘" }));
    expect(screen.getByRole("button", { name: "磁盘" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("磁盘读", { selector: ".performance-legend span" })).toBeInTheDocument();
    expect(screen.getByText("磁盘写", { selector: ".performance-legend span" })).toBeInTheDocument();
    expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-series-count", "2");
  });

  it("shows null as -- while retaining zero and simplified network legends", () => {
    render(<PerformancePanel snapshot={snapshot} history={history} initialMode="网络" />);
    expect(screen.getByRole("button", { name: "网络" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getAllByText("下载").length).toBeGreaterThan(0);
    expect(screen.getAllByText("上传").length).toBeGreaterThan(0);
    expect(screen.queryByText("网络 RX")).not.toBeInTheDocument();
    expect(screen.queryByText("网络 TX")).not.toBeInTheDocument();
    expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-series-count", "2");
    expect(screen.getAllByText("--").length).toBeGreaterThan(0);
  });
  it("keeps the chart dimensions while history is loading", () => {
    render(<PerformancePanel snapshot={snapshot} history={[]} loading />);
    expect(screen.getByTestId("performance-chart")).toHaveClass("performance-chart");
    expect(screen.getByText("正在加载历史数据…")).toBeInTheDocument();
  });
});
