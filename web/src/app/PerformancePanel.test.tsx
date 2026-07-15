import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import {
  PerformancePanel,
  buildChartData,
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
  players: 0,
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
    expect(screen.getAllByText("0")).toHaveLength(2);
    expect(screen.getByText("1h 1m 1s")).toBeInTheDocument();
    expect(screen.getByText("0 ms")).toBeInTheDocument();
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

  it("shows null as -- while retaining zero and mode legends", () => {
    render(<PerformancePanel snapshot={snapshot} history={history} initialMode="网络" />);
    expect(screen.getByRole("button", { name: "网络" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("网络 RX")).toBeInTheDocument();
    expect(screen.getByText("网络 TX")).toBeInTheDocument();
    expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-series-count", "2");
    expect(screen.getAllByText("--").length).toBeGreaterThan(0);
  });
  it("keeps the chart dimensions while history is loading", () => {
    render(<PerformancePanel snapshot={snapshot} history={[]} loading />);
    expect(screen.getByTestId("performance-chart")).toHaveClass("performance-chart");
    expect(screen.getByText("正在加载历史数据…")).toBeInTheDocument();
  });
});
