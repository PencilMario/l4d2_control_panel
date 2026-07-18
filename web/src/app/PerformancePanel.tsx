import { memo, useMemo, useState } from "react";
import {
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  type TooltipContentProps,
} from "recharts";

export type PerformanceSnapshot = {
  image_size_bytes: number | null;
  game_size_bytes?: number | null;
  private_size_bytes?: number | null;
  backups_size_bytes?: number | null;
  console_size_bytes?: number | null;
  cpu_percent: number | null;
  memory_bytes: number | null;
  memory_limit_bytes: number | null;
  memory_percent: number | null;
  network_rx_bytes_per_sec: number | null;
  network_tx_bytes_per_sec: number | null;
  network_rx_bytes: number | null;
  network_tx_bytes: number | null;
  block_read_bytes_per_sec: number | null;
  block_write_bytes_per_sec: number | null;
  block_read_bytes: number | null;
  block_write_bytes: number | null;
  pids: number | null;
  uptime_seconds: number | null;
  a2s_latency_ms: number | null;
};

export type PerformanceHistoryPoint = {
  at: string;
  run_id: string;
  cpu_percent: number | null;
  memory_percent: number | null;
  network_rx_bytes_per_sec: number | null;
  network_tx_bytes_per_sec: number | null;
  block_read_bytes_per_sec: number | null;
  block_write_bytes_per_sec: number | null;
};

type Mode = "CPU" | "内存" | "网络" | "磁盘";
const MODES: Mode[] = ["CPU", "内存", "网络", "磁盘"];
export const LINE_CONNECTS_NULLS = false;
const SNAPSHOT_KEYS: ReadonlyArray<keyof PerformanceSnapshot> = [
  "image_size_bytes",
  "cpu_percent",
  "memory_bytes",
  "memory_limit_bytes",
  "memory_percent",
  "network_rx_bytes_per_sec",
  "network_tx_bytes_per_sec",
  "network_rx_bytes",
  "network_tx_bytes",
  "block_read_bytes_per_sec",
  "block_write_bytes_per_sec",
  "block_read_bytes",
  "block_write_bytes",
  "pids",
  "uptime_seconds",
  "a2s_latency_ms",
];

const trim = (value: number, digits = 1) =>
  value.toFixed(digits).replace(/\.0+$|(?<=\.[0-9]*[1-9])0+$/, "");

export function formatBytes(value: number | null | undefined): string {
  if (value === null || value === undefined) return "--";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let amount = value;
  let unit = 0;
  while (Math.abs(amount) >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  return `${trim(amount)} ${units[unit]}`;
}

export const formatBytesPerSecond = (value: number | null) =>
  value === null ? "--" : `${formatBytes(value)}/s`;

export function formatDuration(value: number | null): string {
  if (value === null) return "--";
  const seconds = Math.max(0, Math.floor(value));
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  return [days && `${days}d`, hours && `${hours}h`, minutes && `${minutes}m`, `${remainder}s`]
    .filter(Boolean)
    .join(" ");
}

export const formatLatency = (value: number | null) =>
  value === null ? "--" : `${trim(value)} ms`;
export const formatPercent = (value: number | null) =>
  value === null ? "--" : `${trim(value)}%`;

const seriesFor = (mode: Mode) => {
  if (mode === "CPU") return [{ key: "cpu", label: "CPU", color: "#8de35b" }];
  if (mode === "内存") return [{ key: "memory", label: "内存", color: "#e4b84d" }];
  if (mode === "网络") return [
    { key: "rx", label: "下载", color: "#53b7d8" },
    { key: "tx", label: "上传", color: "#d78c59" },
  ];
  return [
    { key: "read", label: "磁盘读", color: "#8ccf87" },
    { key: "write", label: "磁盘写", color: "#d47b79" },
  ];
};

type ChartPoint = { at: string; label: string; runId: string; synthetic?: boolean; cpu: number | null; memory: number | null; rx: number | null; tx: number | null; read: number | null; write: number | null };

const formatChartTime = (value: string): string => {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "";
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
};

const chartPoint = (point: PerformanceHistoryPoint): ChartPoint => ({
    at: point.at,
    label: formatChartTime(point.at),
    runId: point.run_id,
    cpu: point.cpu_percent,
    memory: point.memory_percent,
    rx: point.network_rx_bytes_per_sec,
    tx: point.network_tx_bytes_per_sec,
    read: point.block_read_bytes_per_sec,
    write: point.block_write_bytes_per_sec,
  });

const isAllSeriesNull = (point: ChartPoint) =>
  point.cpu === null &&
  point.memory === null &&
  point.rx === null &&
  point.tx === null &&
  point.read === null &&
  point.write === null;

const gapPoint = (previous: ChartPoint, next: ChartPoint, index: number): ChartPoint => {
  const previousTime = Date.parse(previous.at);
  const nextTime = Date.parse(next.at);
  const midpoint = Number.isFinite(previousTime) && Number.isFinite(nextTime) && nextTime > previousTime
    ? new Date(previousTime + (nextTime - previousTime) / 2).toISOString()
    : `__run_gap_${index}`;
  return {
    at: midpoint,
    label: formatChartTime(midpoint),
    runId: "",
    synthetic: true,
    cpu: null,
    memory: null,
    rx: null,
    tx: null,
    read: null,
    write: null,
  };
};

export const buildChartData = (history: PerformanceHistoryPoint[]): ChartPoint[] => {
  const data: ChartPoint[] = [];
  for (const [index, historyPoint] of history.entries()) {
    const next = chartPoint(historyPoint);
    const previous = data.at(-1);
    if (
      previous &&
      !previous.synthetic &&
      previous.runId.trim() !== "" &&
      next.runId.trim() !== "" &&
      previous.runId !== next.runId &&
      !isAllSeriesNull(previous) &&
      !isAllSeriesNull(next)
    ) {
      data.push(gapPoint(previous, next, index));
    }
    data.push(next);
  }
  return data;
};

function ChartTooltip({ active, payload, label, mode }: TooltipContentProps<any, any> & { mode: Mode }) {
  if (
    !active ||
    !payload?.length ||
    payload.some((item) => item.payload?.synthetic)
  ) return null;
  return (
    <div className="performance-tooltip">
      <b>{String(label)}</b>
      {payload.map((item) => (
        <span key={String(item.dataKey)} style={{ color: item.color }}>
          {item.name}: {mode === "CPU" || mode === "内存"
            ? formatPercent(typeof item.value === "number" ? item.value : null)
            : formatBytesPerSecond(typeof item.value === "number" ? item.value : null)}
        </span>
      ))}
    </div>
  );
}

function Metric({ label, value, note }: { label: string; value: string; note?: string }) {
  return <span className="performance-metric" title={note}><small>{label}</small><b>{value}</b>{note ? <em>{note}</em> : null}</span>;
}

export type PerformancePanelProps = { snapshot: PerformanceSnapshot; history: PerformanceHistoryPoint[]; initialMode?: Mode; loading?: boolean };

export function performancePanelPropsEqual(
  previous: PerformancePanelProps,
  next: PerformancePanelProps,
): boolean {
  return (
    previous.history === next.history &&
    (previous.loading ?? false) === (next.loading ?? false) &&
    (previous.initialMode ?? "CPU") === (next.initialMode ?? "CPU") &&
    SNAPSHOT_KEYS.every(
      (key) => previous.snapshot[key] === next.snapshot[key],
    )
  );
}

export const PerformancePanel = memo(function PerformancePanel({ snapshot, history, initialMode = "CPU", loading = false }: PerformancePanelProps) {
  const [mode, setMode] = useState<Mode>(initialMode);
  const data = useMemo<ChartPoint[]>(() => buildChartData(history), [history]);
  const series = useMemo(() => seriesFor(mode), [mode]);
  return (
    <section className="performance-panel">
      <div className="performance-current">
        <Metric label="总占用" value={formatBytes((snapshot.image_size_bytes ?? 0) + (snapshot.game_size_bytes ?? 0) + (snapshot.private_size_bytes ?? 0) + (snapshot.backups_size_bytes ?? 0) + (snapshot.console_size_bytes ?? 0))} note={`游戏 ${formatBytes(snapshot.game_size_bytes)} · 私有 ${formatBytes(snapshot.private_size_bytes)} · 备份 ${formatBytes(snapshot.backups_size_bytes)} · 日志 ${formatBytes(snapshot.console_size_bytes)} · 镜像 ${formatBytes(snapshot.image_size_bytes)}`} />
        <Metric label="CPU" value={formatPercent(snapshot.cpu_percent)} />
        <Metric label="内存" value={`${formatBytes(snapshot.memory_bytes)} / ${formatBytes(snapshot.memory_limit_bytes)} (${formatPercent(snapshot.memory_percent)})`} />
        <Metric label="下载" value={formatBytesPerSecond(snapshot.network_rx_bytes_per_sec)} note={`累计 ${formatBytes(snapshot.network_rx_bytes)}`} />
        <Metric label="上传" value={formatBytesPerSecond(snapshot.network_tx_bytes_per_sec)} note={`累计 ${formatBytes(snapshot.network_tx_bytes)}`} />
        <Metric label="磁盘读" value={formatBytesPerSecond(snapshot.block_read_bytes_per_sec)} note={`累计 ${formatBytes(snapshot.block_read_bytes)}`} />
        <Metric label="磁盘写" value={formatBytesPerSecond(snapshot.block_write_bytes_per_sec)} note={`累计 ${formatBytes(snapshot.block_write_bytes)}`} />
        <Metric label="PID" value={snapshot.pids === null ? "--" : String(snapshot.pids)} />
        <Metric label="运行时间" value={formatDuration(snapshot.uptime_seconds)} />
        <Metric label="A2S 延迟" value={formatLatency(snapshot.a2s_latency_ms)} />
      </div>
      <div className="performance-chart-head">
        <div className="performance-modes" role="group" aria-label="性能图表指标">
          {MODES.map((item) => <button key={item} type="button" aria-pressed={mode === item} onClick={() => setMode(item)}>{item}</button>)}
        </div>
        <div className="performance-legend">{series.map((item) => <span key={item.key}><i style={{ background: item.color }} />{item.label}</span>)}</div>
      </div>
      <div className="performance-chart" data-testid="performance-chart" data-point-count={data.length} data-series-count={series.length}>
        {loading ? <div className="performance-chart-state">正在加载历史数据…</div> : data.length === 0 ? <div className="performance-chart-state">暂无历史数据</div> : (
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: -20 }}>
              <XAxis dataKey="label" tick={{ fontSize: 9 }} minTickGap={28} />
              <YAxis tick={{ fontSize: 9 }} width={48} tickFormatter={(value) => mode === "CPU" || mode === "内存" ? `${trim(value)}%` : formatBytesPerSecond(value)} />
              <Tooltip content={(props) => <ChartTooltip {...props} mode={mode} />} />
              {series.map((item) => <Line key={item.key} type="linear" dataKey={item.key} name={item.label} stroke={item.color} strokeWidth={1.5} dot={false} connectNulls={LINE_CONNECTS_NULLS} isAnimationActive={false} />)}
            </LineChart>
          </ResponsiveContainer>
        )}
      </div>
    </section>
  );
}, performancePanelPropsEqual);
