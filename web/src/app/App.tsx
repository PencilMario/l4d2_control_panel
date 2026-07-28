import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import {
  Activity,
  AlertCircle,
  ArrowDown,
  Ban,
  Box,
  CalendarClock,
  CheckCircle2,
  Clock,
  ChevronDown,
  ChevronUp,
  CircleStop,
  Database,
  Download,
  Edit3,
  ExternalLink,
  FileArchive,
  Files,
  FolderGit2,
  Gauge,
  Globe2,
  Layers,
  Key,
  ListTodo,
  Lock,
  Map,
  MapPin,
  Play,
  Plus,
  RefreshCw,
  ScrollText,
  Save,
  Send,
  Server,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  TerminalSquare,
  Trash2,
  Upload,
  UserX,
  Users,
  X,
} from "lucide-react";
import { api, normalizeInstance, type Job } from "../api/client";
import { JobsPage } from "./JobsPage";
import { JobLogsPage } from "./JobLogsPage";
import { GameLogsPage } from "./GameLogsPage";
import {
  InstanceConfigModal,
  type ConfigurableInstance,
  type InstanceConfigValues,
  type PackageVersion,
  type GitHubSource,
} from "./InstanceConfigModal";
import { PrivateFilesPage } from "./PrivateFilesPage";
import { SchedulesPage } from "./SchedulesPage";
import { VPKUploadQueue } from "./VPKUploadQueue";
import { cancelVPKUpload, enqueueVPKUploads, retryVPKUpload, startVPKUploadQueue, type VPKUploadMode, type VPKUploadTask } from "../vpk/uploadQueue";
import { useConsoleFollow } from "./useConsoleFollow";
import { appendConsoleOutput } from "./consoleBuffer";
import {
  formatBytes as formatMetricBytes,
  formatBytesPerSecond,
  formatLatency,
  formatPercent,
  PerformancePanel,
  type PerformanceHistoryPoint,
} from "./PerformancePanel";
import "../styles/app.css";
export type Instance = ConfigurableInstance & {
  players: number | null;
  cpu: number | null;
  memory: number | null;
  observed_state?: string;
  container_running?: boolean;
  observed_max_players?: number | null;
  current_map?: string;
  sampled_at?: string | null;
  run_id?: string | null;
  container_running_known?: boolean;
  memory_bytes?: number | null;
  memory_limit_bytes?: number | null;
  memory_percent?: number | null;
  network_rx_bytes_per_sec?: number | null;
  network_tx_bytes_per_sec?: number | null;
  network_rx_bytes?: number | null;
  network_tx_bytes?: number | null;
  block_read_bytes_per_sec?: number | null;
  block_write_bytes_per_sec?: number | null;
  block_read_bytes?: number | null;
  block_write_bytes?: number | null;
  pids?: number | null;
  uptime_seconds?: number | null;
  a2s_latency_ms?: number | null;
  image_size_bytes?: number | null;
  game_size_bytes?: number | null;
  private_size_bytes?: number | null;
  backups_size_bytes?: number | null;
  console_size_bytes?: number | null;
};
export type InstanceOverview = {
  actual_state: string;
  container_running: boolean;
  container_running_known: boolean;
  sampled_at: string | null;
  run_id: string | null;
  map: string;
  players: number | null;
  max_players: number | null;
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
  image_size_bytes?: number | null;
  game_size_bytes?: number | null;
  private_size_bytes?: number | null;
  backups_size_bytes?: number | null;
  console_size_bytes?: number | null;
  issues?: string[];
};
type Props = {
  initialInstances?: Instance[];
  initialPackages?: PackageVersion[];
  onAction?: (id: string, action: string) => void;
};
type Page = "overview" | "private" | "content" | "jobs" | "joblogs" | "gamelogs" | "schedules" | "settings";
type HealthState = {
  status: "checking" | "online" | "error";
  message: string;
};
type Confirmation = {
  title: string;
  description: string;
  confirmLabel: string;
  confirm: () => Promise<boolean | void> | boolean | void;
};
type PlayerMatch = {
  hostname: string;
  version: string;
  secure: boolean | null;
  os: string;
  map: string;
  private_address: string;
  public_address: string;
  humans: number;
  max_players: number;
};
type OnlinePlayer = {
  user_id: number;
  name: string;
  unique_id?: string;
  connected?: string;
  ping?: number;
  loss?: number;
  score: number | null;
};
type PlayerSnapshot = {
  map?: string;
  max_players?: number;
  match?: PlayerMatch;
  players: OnlinePlayer[];
};
type SharedGameState = {
  active_release_id?: string;
  version?: string;
  path?: string;
  migration_state?: string;
};

export const sharedGameVersionLabel = (state: SharedGameState) =>
  state.version || (state.active_release_id ? "版本未知" : "未初始化");

const errorMessage = (reason: unknown) =>
  reason instanceof Error ? reason.message : String(reason);
const EMPTY_PERFORMANCE_HISTORY: PerformanceHistoryPoint[] = [];

type HistoryBootstrap = {
  token: number;
  controller: AbortController;
  promise: Promise<PerformanceHistoryPoint[]>;
};

function useAsyncLocks() {
  const locks = useRef(new Set<string>());
  const [pending, setPending] = useState(new Set<string>());
  const run = useCallback(async (key: string, operation: () => Promise<unknown>) => {
    if (locks.current.has(key)) return false;
    locks.current.add(key);
    setPending((current) => new Set(current).add(key));
    try {
      await operation();
      return true;
    } finally {
      locks.current.delete(key);
      setPending((current) => {
        const next = new Set(current);
        next.delete(key);
        return next;
      });
    }
  }, []);
  return {
    pending,
    run,
    isLocked: (key: string) => locks.current.has(key),
  };
}

export function mergePerformanceHistory(
  existing: PerformanceHistoryPoint[],
  incoming: PerformanceHistoryPoint[],
): PerformanceHistoryPoint[] {
  const points = new globalThis.Map<
    string,
    { point: PerformanceHistoryPoint; timestamp: number; index: number }
  >();
  for (const [index, point] of [...existing, ...incoming].entries()) {
    const timestamp = Date.parse(point.at);
    if (!Number.isFinite(timestamp)) continue;
    const key = `${point.at}\u0000${point.run_id}`;
    const previous = points.get(key);
    points.set(key, {
      point,
      timestamp,
      index: previous?.index ?? index,
    });
  }
  return [...points.values()]
    .sort((a, b) => a.timestamp - b.timestamp || a.index - b.index)
    .slice(-720)
    .map(({ point }) => point);
}

export function prunePerformanceHistory(
  current: Record<string, PerformanceHistoryPoint[]>,
  liveIDs: Set<string>,
): Record<string, PerformanceHistoryPoint[]> {
  const next: Record<string, PerformanceHistoryPoint[]> = {};
  for (const [id, points] of Object.entries(current)) {
    if (liveIDs.has(id)) next[id] = points;
  }
  return next;
}

const historyPointFromOverview = (
  overview: Pick<
    Instance,
    | "sampled_at"
    | "run_id"
    | "cpu"
    | "memory_percent"
    | "network_rx_bytes_per_sec"
    | "network_tx_bytes_per_sec"
    | "block_read_bytes_per_sec"
    | "block_write_bytes_per_sec"
  >,
): PerformanceHistoryPoint | null =>
  overview.sampled_at
    ? {
        at: overview.sampled_at,
        run_id: overview.run_id || "",
        cpu_percent: overview.cpu,
        memory_percent: overview.memory_percent ?? null,
        network_rx_bytes_per_sec: overview.network_rx_bytes_per_sec ?? null,
        network_tx_bytes_per_sec: overview.network_tx_bytes_per_sec ?? null,
        block_read_bytes_per_sec: overview.block_read_bytes_per_sec ?? null,
        block_write_bytes_per_sec: overview.block_write_bytes_per_sec ?? null,
      }
    : null;

export function App({ initialInstances, initialPackages, onAction }: Props) {
  const injected = initialInstances !== undefined;
  const [auth, setAuth] = useState(injected ? "yes" : "checking");
  const [instances, setInstances] = useState<Instance[]>(
    initialInstances || [],
  );
  const [packages, setPackages] = useState<PackageVersion[]>(
    initialPackages || [],
  );
  const [packageSources, setPackageSources] = useState<GitHubSource[]>([]);
  const [sharedGame, setSharedGame] = useState<SharedGameState>({});
  const [performanceHistory, setPerformanceHistory] = useState<
    Record<string, PerformanceHistoryPoint[]>
  >({});
  const historyLoaded = useRef(new Set<string>());
  const historyInFlight = useRef(
    new globalThis.Map<string, HistoryBootstrap>(),
  );
  const historyToken = useRef(0);
  const liveInstanceIDs = useRef(new Set<string>());
  const loadGeneration = useRef(0);
  const mountedRef = useRef(true);
  const [pending, setPending] = useState<Instance | null>(null);
  const [page, setPage] = useState<Page>("overview");
  const [selectedInstanceID, setSelectedInstanceID] = useState(
    initialInstances?.[0]?.id ?? "",
  );
  const [logJob, setLogJob] = useState<Job | null>(null);
  const [terminal, setTerminal] = useState<Instance | null>(null);
  const [playersTarget, setPlayersTarget] = useState<Instance | null>(null);
  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState("");
  const [health, setHealth] = useState<HealthState>(
    injected
      ? { status: "online", message: "测试数据已加载" }
      : { status: "checking", message: "正在检查 Docker API…" },
  );
  const pollControllers = useRef(new globalThis.Map<string, AbortController>());
  const pollTimers = useRef(new globalThis.Map<string, number>());
  const actionLocks = useRef(new Set<string>());
  const [pendingActions, setPendingActions] = useState(new Set<string>());
  const queueLocks = useRef(new Set<string>());
  useEffect(() => () => {
    for (const controller of pollControllers.current.values()) controller.abort();
    for (const timer of pollTimers.current.values()) window.clearTimeout(timer);
    pollControllers.current.clear();
    pollTimers.current.clear();
  }, []);
  const loadInstances = useCallback(async () => {
    if (!mountedRef.current) return;
    const generation = ++loadGeneration.current;
    const isCurrent = () =>
      mountedRef.current && generation === loadGeneration.current;
    const base = (await api<any[]>("/api/instances")).map(normalizeInstance);
    if (!isCurrent()) return;
    const liveIDs = new Set(base.map((instance) => instance.id));
    liveInstanceIDs.current = liveIDs;
    for (const id of historyLoaded.current) {
      if (!liveIDs.has(id)) historyLoaded.current.delete(id);
    }
    for (const [id, bootstrap] of historyInFlight.current) {
      if (!liveIDs.has(id)) {
        bootstrap.controller.abort();
        historyInFlight.current.delete(id);
      }
    }
    for (const instance of base) {
      if (
        historyLoaded.current.has(instance.id) ||
        historyInFlight.current.has(instance.id)
      ) {
        continue;
      }
      const token = ++historyToken.current;
      const controller = new AbortController();
      const promise = api<PerformanceHistoryPoint[]>(
        `/api/instances/${instance.id}/performance-history`,
        { signal: controller.signal },
      );
      const bootstrap = { token, controller, promise };
      historyInFlight.current.set(instance.id, bootstrap);
      void promise
        .then((history) => {
          const owner = historyInFlight.current.get(instance.id);
          if (
            !mountedRef.current ||
            owner?.token !== token ||
            !liveInstanceIDs.current.has(instance.id)
          ) {
            return;
          }
          historyInFlight.current.delete(instance.id);
          historyLoaded.current.add(instance.id);
          setPerformanceHistory((current) => ({
            ...current,
            [instance.id]: mergePerformanceHistory(
              current[instance.id] || EMPTY_PERFORMANCE_HISTORY,
              Array.isArray(history) ? history : EMPTY_PERFORMANCE_HISTORY,
            ),
          }));
        })
        .catch(() => {
          const owner = historyInFlight.current.get(instance.id);
          if (mountedRef.current && owner?.token === token) {
            historyInFlight.current.delete(instance.id);
          }
        });
    }
    const enrichedPromise = Promise.all(
      base.map(async (instance): Promise<Instance> => {
        try {
          const overview = await api<InstanceOverview>(
            `/api/instances/${instance.id}/overview`,
          );
          return {
            ...instance,
            observed_state: overview.actual_state,
            container_running: overview.container_running,
            container_running_known: overview.container_running_known,
            sampled_at: overview.sampled_at ?? null,
            run_id: overview.run_id ?? null,
            observed_max_players: overview.max_players,
            current_map: overview.map || undefined,
            cpu: overview.cpu_percent,
            memory_bytes: overview.memory_bytes ?? null,
            memory_limit_bytes: overview.memory_limit_bytes ?? null,
            memory_percent: overview.memory_percent ?? null,
            network_rx_bytes_per_sec: overview.network_rx_bytes_per_sec ?? null,
            network_tx_bytes_per_sec: overview.network_tx_bytes_per_sec ?? null,
            network_rx_bytes: overview.network_rx_bytes ?? null,
            network_tx_bytes: overview.network_tx_bytes ?? null,
            block_read_bytes_per_sec: overview.block_read_bytes_per_sec ?? null,
            block_write_bytes_per_sec: overview.block_write_bytes_per_sec ?? null,
            block_read_bytes: overview.block_read_bytes ?? null,
            block_write_bytes: overview.block_write_bytes ?? null,
            pids: overview.pids ?? null,
            uptime_seconds: overview.uptime_seconds ?? null,
            a2s_latency_ms: overview.a2s_latency_ms ?? null,
            image_size_bytes: overview.image_size_bytes ?? null,
            game_size_bytes: overview.game_size_bytes ?? null,
            private_size_bytes: overview.private_size_bytes ?? null,
            backups_size_bytes: overview.backups_size_bytes ?? null,
            console_size_bytes: overview.console_size_bytes ?? null,
            memory:
              overview.memory_bytes === null
                ? null
                : overview.memory_bytes / (1 << 30),
            players: overview.players,
          };
        } catch {
          return {
            ...instance,
            observed_state: "unknown",
            container_running: false,
            container_running_known: false,
            sampled_at: null,
            run_id: null,
            observed_max_players: null,
            players: null,
            cpu: null,
            memory: null,
            memory_bytes: null,
            memory_limit_bytes: null,
            memory_percent: null,
            network_rx_bytes_per_sec: null,
            network_tx_bytes_per_sec: null,
            network_rx_bytes: null,
            network_tx_bytes: null,
            block_read_bytes_per_sec: null,
            block_write_bytes_per_sec: null,
            block_read_bytes: null,
            block_write_bytes: null,
            pids: null,
            uptime_seconds: null,
            a2s_latency_ms: null,
          };
        }
      }),
    );
    const enriched = await enrichedPromise;
    if (!isCurrent()) return;
    setPerformanceHistory((current) => {
      if (!isCurrent()) return current;
      const next = prunePerformanceHistory(current, liveIDs);
      for (const instance of enriched) {
        if (!instance.sampled_at) continue;
        const point = historyPointFromOverview(instance);
        if (point) next[instance.id] = mergePerformanceHistory(next[instance.id] || [], [point]);
      }
      return next;
    });
    setInstances((current) => (isCurrent() ? enriched : current));
  }, []);
  const loadPackages = async () => {
    const next = await api<PackageVersion[]>("/api/packages");
    if (mountedRef.current) setPackages(next);
  };
  const loadPackageSources = async () => {
    const next = await api<GitHubSource[]>("/api/github-sources");
    if (mountedRef.current) setPackageSources(Array.isArray(next) ? next : []);
  };
  const loadSharedGame = async () => {
    try {
      const next = await api<SharedGameState>("/api/game");
      if (mountedRef.current) setSharedGame(next);
    } catch {
      if (mountedRef.current) setSharedGame({});
    }
  };
  const loadHealth = async () => {
    try {
      await api("/api/health");
      if (mountedRef.current) {
        setHealth({ status: "online", message: "Docker API 正常" });
      }
    } catch (reason) {
      if (mountedRef.current) {
        setHealth({ status: "error", message: errorMessage(reason) });
      }
    }
  };
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      loadGeneration.current += 1;
      historyLoaded.current.clear();
      for (const bootstrap of historyInFlight.current.values()) {
        bootstrap.controller.abort();
      }
      historyInFlight.current.clear();
      liveInstanceIDs.current.clear();
    };
  }, []);
  useEffect(() => {
    if (injected) return;
    let cancelled = false;
    api("/api/session")
      .then(() => {
        if (cancelled || !mountedRef.current) return;
        setAuth("yes");
        void Promise.allSettled([
          loadInstances(),
          loadPackages(),
          loadPackageSources(),
          loadSharedGame(),
          loadHealth(),
        ]);
      })
      .catch(() => {
        if (!cancelled && mountedRef.current) setAuth("no");
      });
    return () => {
      cancelled = true;
    };
  }, []);
  useEffect(() => {
    if (injected || auth !== "yes") return;
    const timer = window.setInterval(() => void loadInstances(), 5_000);
    return () => {
      window.clearInterval(timer);
      if (!mountedRef.current) return;
      loadGeneration.current += 1;
      historyLoaded.current.clear();
      for (const bootstrap of historyInFlight.current.values()) {
        bootstrap.controller.abort();
      }
      historyInFlight.current.clear();
      liveInstanceIDs.current.clear();
    };
  }, [auth, injected, loadInstances]);
  useEffect(() => {
    const selectionStillExists = instances.some(
      (instance) => instance.id === selectedInstanceID,
    );
    if (!selectionStillExists) {
      setSelectedInstanceID(instances[0]?.id ?? "");
    }
    if (!instances.length && page === "gamelogs") setPage("overview");
  }, [instances, page, selectedInstanceID]);
  const queue = async (path: string, body: any) => {
    const serialized = JSON.stringify(body);
    const key = `${path}\u0000${serialized}`;
    if (queueLocks.current.has(key)) return;
    queueLocks.current.add(key);
    try {
      const created = await api<Job>(path, {
        method: "POST",
        body: serialized,
      });
      setJob(created);
      void pollJob(created.ID).catch(() => undefined);
    } finally {
      queueLocks.current.delete(key);
    }
  };
  const queueAndWait = async (
    path: string,
    body: unknown,
    method = "POST",
  ) => {
    const created = await api<Job>(path, {
      method,
      body: JSON.stringify(body),
    });
    setJob(created);
    return pollJob(created.ID);
  };
  const permanentlyDeleteInstance = async (id: string) => {
    try {
      await queueAndWait(
        `/api/instances/${id}`,
        { confirm: true, delete_data: true },
        "DELETE",
      );
      await loadInstances();
      return true;
    } catch (reason) {
      setError(errorMessage(reason));
      return false;
    }
  };
  const action = async (id: string, kind: string) => {
    const key = `${id}:${kind}`;
    if (actionLocks.current.has(key)) return;
    actionLocks.current.add(key);
    setPendingActions((current) => new Set(current).add(key));
    try {
      if (onAction) {
        onAction(id, kind);
        return;
      }
      await queue(`/api/instances/${id}/actions`, {
        action: kind,
        confirm: kind !== "start",
      });
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      actionLocks.current.delete(key);
      setPendingActions((current) => {
        const next = new Set(current);
        next.delete(key);
        return next;
      });
    }
  };
  const pollJob = (id: string) =>
    new Promise<Job>((resolve, reject) => {
      pollControllers.current.get(id)?.abort();
      const previousTimer = pollTimers.current.get(id);
      if (previousTimer !== undefined) window.clearTimeout(previousTimer);
      const controller = new AbortController();
      pollControllers.current.set(id, controller);
      let settled = false;
      const finish = (callback: () => void) => {
        if (settled) return;
        settled = true;
        pollControllers.current.delete(id);
        const timer = pollTimers.current.get(id);
        if (timer !== undefined) window.clearTimeout(timer);
        pollTimers.current.delete(id);
        callback();
      };
      const read = async () => {
      try {
        const next = await api<Job>(`/api/jobs/${id}`, { signal: controller.signal });
        if (controller.signal.aborted || settled) return;
        setJob(next);
        if (["succeeded", "failed", "interrupted"].includes(next.Status)) {
          void Promise.allSettled([loadInstances(), loadPackages(), loadSharedGame()]);
          finish(() => resolve(next));
          return;
        }
        const timer = window.setTimeout(() => void read(), 800);
        pollTimers.current.set(id, timer);
      } catch (reason) {
        if (controller.signal.aborted) return;
        setError(errorMessage(reason));
        finish(() => reject(reason));
      }
      };
      void read();
    });
  if (auth === "checking")
    return <div className="splash">正在连接控制节点…</div>;
  if (auth === "no")
    return (
      <Login
        onSuccess={() => {
          setAuth("yes");
          void Promise.allSettled([
            loadInstances(),
            loadPackages(),
            loadPackageSources(),
            loadSharedGame(),
            loadHealth(),
          ]);
        }}
      />
    );
  const running = instances.filter(
    (x) => displayState(x) === "running",
  ).length;
  const selectedInstance = instances.find(
    (instance) => instance.id === selectedInstanceID,
  );
  const pageTitle =
    page === "overview"
      ? "服务器作战室"
      : page === "private"
        ? "私有文件"
        : page === "content"
          ? "内容仓库"
          : page === "jobs"
            ? "后台任务"
            : page === "joblogs"
              ? "任务日志"
              : page === "gamelogs"
                ? "游戏日志分类预览"
                : page === "schedules"
                  ? "计划任务"
                  : "系统设置";
  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="product-mark">
          <span><ShieldCheck aria-hidden="true" /></span>
          <b>L4D2 控制面板</b>
        </div>
        <div className="topbar-actions">
          <span className={`node-pill ${health.status}`}>
            <i />控制节点：<b>{health.status === "online" ? "在线" : health.status === "error" ? "异常" : "检查中"}</b>
          </span>
          <button type="button" className="topbar-task" aria-label="打开后台任务" onClick={() => { setLogJob(null); setPage("jobs"); }}>
            <ListTodo aria-hidden="true" />后台任务
          </button>
          <span className="operator-name"><i />管理员</span>
          <ShieldCheck className="session-shield" aria-label="安全会话" />
        </div>
      </header>
      <div className="app-body">
      <aside className="sidebar">
        <span className="nav-caption">管理入口</span>
        <nav className="sidebar-nav" aria-label="主导航">
          <Nav
            active={page === "overview"}
            onClick={() => setPage("overview")}
            icon={<Gauge />}
          >
            总览
          </Nav>
          <Nav
            active={page === "private"}
            onClick={() => setPage("private")}
            icon={<Files />}
          >
            私有文件
          </Nav>
          <Nav
            active={page === "content"}
            onClick={() => setPage("content")}
            icon={<Box />}
          >
            内容仓库
          </Nav>
          <Nav
            active={page === "gamelogs"}
            onClick={() => setPage("gamelogs")}
            icon={<ScrollText />}
            disabled={!instances.length}
          >
            游戏日志
          </Nav>
          <Nav
            active={page === "jobs" || page === "joblogs"}
            onClick={() => { setLogJob(null); setPage("jobs"); }}
            icon={<ListTodo />}
          >
            后台任务
          </Nav>
          <Nav
            active={page === "schedules"}
            onClick={() => setPage("schedules")}
            icon={<CalendarClock />}
          >
            计划任务
          </Nav>
          <Nav
            active={page === "settings"}
            onClick={() => setPage("settings")}
            icon={<Settings />}
          >
            系统设置
          </Nav>
        </nav>
        <div className="aside-foot">
          <div className={`node-card ${health.status}`}>
            <span className="node-state-text">
              {health.status === "online" ? "控制节点在线" : health.status === "error" ? "控制节点异常" : "控制节点检查中"}
            </span>
            <div>
              <Server aria-hidden="true" />
              <b>控制节点状态</b>
              <em>{health.status === "online" ? "在线" : health.status === "error" ? "异常" : "检查中"}</em>
            </div>
            <dl>
              <div><dt>连接状态</dt><dd>{health.message || "正常"}</dd></div>
              <div><dt>活动后台任务</dt><dd>{job && !["succeeded", "failed", "interrupted"].includes(job.Status) ? "1 项" : "0 项"}</dd></div>
            </dl>
          </div>
        </div>
      </aside>
      <main className={`page-main page-${page}`}>
        <div className="page-content">
        {["overview", "content", "jobs", "settings"].includes(page) ? (
          <div className={page === "overview" ? "page-heading overview-heading" : "page-heading"}>
            <h1>{pageTitle}</h1>
            {page !== "overview" ? <p>{page === "content" ? "统一管理共享游戏本体、共享 VPK 模组包、插件版本包及 GitHub 自动发布源" : page === "jobs" ? "持久化排队与异步执行的游戏维护、更新、备份及清理任务" : "管理游戏进程、内容部署与计划维护"}</p> : null}
          </div>
        ) : null}
        {error && (
          <div className="error-banner">
            {error}
            <button onClick={() => setError("")}>
              <X />
            </button>
          </div>
        )}
        {page === "overview" && (
          <Overview
            instances={instances}
            packages={packages}
            packageSources={packageSources}
            sharedGame={sharedGame}
            running={running}
            performanceHistory={performanceHistory}
            pendingActions={pendingActions}
            setPending={setPending}
            action={action}
            setTerminal={setTerminal}
            setPlayers={setPlayersTarget}
            queue={queue}
            reload={loadInstances}
            deleteInstance={permanentlyDeleteInstance}
            acceptJob={(next) => {
              setJob(next);
              void pollJob(next.ID).catch(() => undefined);
            }}
          />
        )}{" "}
        {page === "private" && (
          <PrivateFilesPage instances={instances} queue={queue} queueAndWait={queueAndWait} />
        )}
        {page === "content" && (
          <ContentPage
            instances={instances}
            packages={packages}
            sharedGame={sharedGame}
            reloadPackages={loadPackages}
            reloadSharedGame={loadSharedGame}
            queue={queue}
          />
        )}
        {page === "jobs" && <JobsPage onOpenLogs={(selected) => { setLogJob(selected); setPage("joblogs"); }} />}
        {page === "joblogs" && logJob ? <JobLogsPage job={logJob} onBack={() => setPage("jobs")} /> : null}
        {page === "gamelogs" ? <GameLogsPage instances={instances} /> : null}
        {page === "schedules" && (
          <SchedulesPage instances={instances} packages={packages} />
        )}{" "}
        {page === "settings" && <SettingsPage />}{" "}
        {job && <JobStrip job={job} />}
        </div>
      </main>
      </div>
      {pending && (
        <Confirm
          instance={pending}
          close={() => setPending(null)}
          confirm={() => {
            action(pending.id, "stop");
            setPending(null);
          }}
        />
      )}
      {terminal && (
        <Terminal instance={terminal} close={() => setTerminal(null)} />
      )}
      {playersTarget && (
        <PlayersModal
          instance={playersTarget}
          close={() => setPlayersTarget(null)}
          queue={queue}
        />
      )}
    </div>
  );
}

function Login({ onSuccess }: { onSuccess: () => void }) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (submittingRef.current) return;
    submittingRef.current = true;
    setSubmitting(true);
    try {
      await api("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ password }),
      });
      onSuccess();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  };
  return (
    <div className="login">
      <form onSubmit={submit}>
        <span className="hazard">L4D</span>
        <p className="eyebrow">RESTRICTED CONTROL NODE</p>
        <h1>管理员认证</h1>
        <p>连接单主机 L4D2 控制平面</p>
        <label>
          管理员密码
          <input
            autoFocus
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            minLength={12}
          />
        </label>
        {error && <div className="form-error">{error}</div>}
        <button disabled={submitting} aria-busy={submitting}>
          {submitting ? <RefreshCw /> : null}
          {submitting ? "正在认证…" : "进入作战室"}
        </button>
      </form>
    </div>
  );
}
function Nav({
  active,
  onClick,
  icon,
  disabled = false,
  children,
}: {
  active: boolean;
  onClick: () => void;
  icon: ReactNode;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <button className={active ? "active" : ""} onClick={onClick} disabled={disabled}>
      {icon}
      {children}
    </button>
  );
}
function Overview({
  instances,
  packages,
  packageSources,
  sharedGame,
  running,
  performanceHistory,
  pendingActions,
  setPending,
  action,
  setTerminal,
  setPlayers,
  queue,
  reload,
  deleteInstance,
  acceptJob,
}: {
  instances: Instance[];
  packages: PackageVersion[];
  packageSources: GitHubSource[];
  sharedGame: SharedGameState;
  running: number;
  performanceHistory: Record<string, PerformanceHistoryPoint[]>;
  pendingActions: Set<string>;
  setPending: (v: Instance) => void;
  action: (id: string, a: string) => void;
  setTerminal: (v: Instance) => void;
  setPlayers: (v: Instance) => void;
  queue: (path: string, body: any) => Promise<void>;
  reload: () => Promise<void>;
  deleteInstance: (id: string) => Promise<boolean>;
  acceptJob: (job: Job) => void;
}) {
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Instance | null>(null);
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const [reinstalling, setReinstalling] = useState<Instance | null>(null);
  const [deleting, setDeleting] = useState<Instance | null>(null);
  const [expandedPerformance, setExpandedPerformance] = useState<Record<string, boolean>>({});
  const packagesByID = new globalThis.Map(
    packages.map((item) => [item.id, item]),
  );
  const totalPlayers = instances.some((instance) => instance.players === null)
    ? "--"
    : String(instances.reduce((total, instance) => total + (instance.players ?? 0), 0));
  const pendingItems = instances.filter(
    (instance) => Boolean(instance.package_id) && instance.package_id !== instance.applied_package_id,
  ).length;
  const saveConfig = async (
    values: InstanceConfigValues,
    instance?: Instance,
  ) => {
    const result = await api<any>(
      instance ? `/api/instances/${instance.id}` : "/api/instances",
      {
        method: instance ? "PUT" : "POST",
        body: JSON.stringify(values),
      },
    );
    if (result?.Status && result?.ID) {
      acceptJob(result as Job);
      await reload();
      return;
    }
    await reload();
  };
  return (
    <>
      <header className="overview-head">
        <div>
          <h1>游戏实例总览</h1>
          <p>实时观测《求生之路 2》所有专用服务器实例的生命周期、负载与在线玩家</p>
        </div>
        <button className="command-primary overview-create" aria-label="创建实例" onClick={() => setCreating(true)}>
          <Plus />
          创建游戏实例
        </button>
      </header>
      <section className="metrics overview-summary">
        <Metric
          icon={<Server />}
          label="运行实例"
          value={String(running)}
          unit={`/ ${instances.length}`}
          note={running === instances.length ? "全量服务运行正常" : `${instances.length - running} 个实例处于停止状态`}
          noteTone="success"
        />
        <Metric
          icon={<Users />}
          label="在线玩家"
          value={totalPlayers}
          unit="人"
          note="活跃在全服各对局中"
        />
        <Metric
          icon={<Layers />}
          label="游戏实例"
          value={String(instances.length)}
          unit="个"
          note="独立隔离进程"
        />
        <Metric
          icon={<CheckCircle2 />}
          label="共享游戏本体"
          value={sharedGameVersionLabel(sharedGame).split(" ")[0]}
          note="就绪 (Steam AppID 222860)"
          compact
          noteTone="success"
        />
        <Metric
          icon={<AlertCircle />}
          label="待处理事项"
          value={String(pendingItems)}
          unit="项"
          note={pendingItems ? "插件包更新待应用" : "暂无待处理风险操作"}
          noteTone={pendingItems ? "warning" : undefined}
        />
      </section>
      <section className="work">
        <div className="section-head">
          <h2>所有游戏实例 ({instances.length})</h2>
        </div>
        <div className="grid instance-list">
          {instances.map((x, index) => {
            const selectedPackage = packagesByID.get(x.package_id);
            const packagePending =
              Boolean(x.package_id) && x.package_id !== x.applied_package_id;
            const state = displayState(x);
            const containerRunning = x.container_running ?? state === "running";
            const observedCapacity =
              x.observed_max_players === undefined
                ? x.max_players
                : x.observed_max_players;
            const starting = pendingActions.has(`${x.id}:start`);
            const stopping = pendingActions.has(`${x.id}:stop`);
            const performanceExpanded = expandedPerformance[x.id] ?? index === 0;
            return (
              <article className={`card instance-panel ${state}`} key={x.id}>
                <header className="instance-command-bar">
                  <span className="instance-mark"><Server /></span>
                  <div className="instance-identity">
                    <div className="instance-title-line">
                      <h3>{x.name}</h3>
                      <span className="status-badge">
                        <i></i>
                        {stateLabel(state)}
                      </span>
                    </div>
                    <p className="endpoint">
                      LOCAL-01 : {x.game_port}
                      {x.sourcetv_port ? ` · TV ${x.sourcetv_port}` : ""}
                      {x.plugin_ports.length ? ` · 插件 ${x.plugin_ports.join(", ")}` : ""}
                    </p>
                    <p className="instance-package">
                      {selectedPackage ? `${selectedPackage.filename} · ${selectedPackage.version}` : "未选择插件包"}
                      {packagePending ? <em>待应用</em> : null}
                    </p>
                  </div>
                  <div className="instance-commands">
                    {containerRunning ? (
                      <button className="instance-command command-danger" aria-label={`停止 ${x.name}`} disabled={stopping} aria-busy={stopping} onClick={() => setPending(x)}>{stopping ? <RefreshCw /> : <CircleStop />}{stopping ? "停止中" : "停止游戏实例"}</button>
                    ) : (
                      <button className="instance-command command-primary" aria-label="启动" disabled={starting} aria-busy={starting} onClick={() => void action(x.id, "start")}>{starting ? <RefreshCw /> : <Play />}{starting ? "启动中" : "启动游戏实例"}</button>
                    )}
                    <button className="instance-command" aria-label="控制台" onClick={() => setTerminal(x)}><TerminalSquare />游戏控制台</button>
                    <button className="instance-command" aria-label="玩家" onClick={() => setPlayers(x)}><Users />在线玩家 ({x.players === null ? "--" : x.players}/{observedCapacity === null ? "--" : observedCapacity})</button>
                    <button className="instance-command" aria-label={`配置 ${x.name}`} onClick={() => setEditing(x)}><SlidersHorizontal />私有配置</button>
                    <button className="instance-command" aria-label="更新" onClick={() => setReinstalling(x)}><RefreshCw />插件更新</button>
                    <button className="tool-button command-danger" aria-label={`删除实例 ${x.name}`} title="永久删除实例" onClick={() => setDeleting(x)}><Trash2 /></button>
                  </div>
                </header>
                <div className="instance-metrics" aria-label={`${x.name} 关键指标`}>
                  <InstanceMetric label={x.current_map ? "当前地图" : "启动地图"} value={x.current_map || x.start_map} note={x.game_mode.toUpperCase()} />
                  <InstanceMetric label="玩家" value={`${x.players === null ? "--" : x.players} / ${observedCapacity === null ? "--" : observedCapacity}`} />
                  <InstanceMetric label="CPU" value={formatPercent(x.cpu)} />
                  <InstanceMetric label="内存" value={`${formatMetricBytes(x.memory_bytes)} / ${formatMetricBytes(x.memory_limit_bytes)} (${formatPercent(x.memory_percent ?? null)})`} />
                  <InstanceMetric label="下载" value={formatBytesPerSecond(x.network_rx_bytes_per_sec ?? null)} note={`累计 ${formatMetricBytes(x.network_rx_bytes)}`} />
                  <InstanceMetric label="上传" value={formatBytesPerSecond(x.network_tx_bytes_per_sec ?? null)} note={`累计 ${formatMetricBytes(x.network_tx_bytes)}`} />
                  <InstanceMetric label="A2S 延迟" value={formatLatency(x.a2s_latency_ms ?? null)} />
                  <InstanceMetric label="总占用" value={formatMetricBytes((x.image_size_bytes ?? 0) + (x.game_size_bytes ?? 0) + (x.private_size_bytes ?? 0) + (x.backups_size_bytes ?? 0) + (x.console_size_bytes ?? 0))} note={`游戏 ${formatMetricBytes(x.game_size_bytes)} · 私有 ${formatMetricBytes(x.private_size_bytes)} · 备份 ${formatMetricBytes(x.backups_size_bytes)} · 日志 ${formatMetricBytes(x.console_size_bytes)} · 镜像 ${formatMetricBytes(x.image_size_bytes)}`} />
                </div>
                <div className="performance-section-title">
                  <span><Activity />性能历史采样（最近约 1 小时）</span>
                  <button
                    type="button"
                    aria-expanded={performanceExpanded}
                    onClick={() => setExpandedPerformance((current) => ({ ...current, [x.id]: !performanceExpanded }))}
                  >
                    {performanceExpanded ? "收起性能详情" : "展开性能详情与历史曲线"}
                    {performanceExpanded ? <ChevronUp /> : <ChevronDown />}
                  </button>
                </div>
                {performanceExpanded ? <PerformancePanel
                  snapshot={{
                    image_size_bytes: x.image_size_bytes ?? null,
                    game_size_bytes: x.game_size_bytes ?? null,
                    private_size_bytes: x.private_size_bytes ?? null,
                    backups_size_bytes: x.backups_size_bytes ?? null,
                    console_size_bytes: x.console_size_bytes ?? null,
                    cpu_percent: x.cpu,
                    memory_bytes: x.memory_bytes ?? (x.memory === null ? null : x.memory * (1 << 30)),
                    memory_limit_bytes: x.memory_limit_bytes ?? null,
                    memory_percent: x.memory_percent ?? null,
                    network_rx_bytes_per_sec: x.network_rx_bytes_per_sec ?? null,
                    network_tx_bytes_per_sec: x.network_tx_bytes_per_sec ?? null,
                    network_rx_bytes: x.network_rx_bytes ?? null,
                    network_tx_bytes: x.network_tx_bytes ?? null,
                    block_read_bytes_per_sec: x.block_read_bytes_per_sec ?? null,
                    block_write_bytes_per_sec: x.block_write_bytes_per_sec ?? null,
                    block_read_bytes: x.block_read_bytes ?? null,
                    block_write_bytes: x.block_write_bytes ?? null,
                    pids: x.pids ?? null,
                    uptime_seconds: x.uptime_seconds ?? null,
                    a2s_latency_ms: x.a2s_latency_ms ?? null,
                  }}
                  history={performanceHistory[x.id] || EMPTY_PERFORMANCE_HISTORY}
                /> : null}
                <div className="bar">
                  <i
                    style={{
                      width: state === "running" ? "100%" : "2%",
                    }}
                  />
                </div>
              </article>
            );
          })}
        </div>
        {instances.length === 0 && (
          <div className="empty">尚无实例。创建第一个 Host 网络服务器。</div>
        )}
      </section>
      {creating && (
        <InstanceConfigModal
          mode="create"
          packages={packages}
          sources={packageSources}
          onClose={() => setCreating(false)}
          onSubmit={(values) => saveConfig(values)}
        />
      )}
      {editing ? (
        <InstanceConfigModal
          key={editing.id}
          mode="edit"
          instance={editing}
          packages={packages}
          sources={packageSources}
          onClose={() => setEditing(null)}
          onSubmit={(values) => saveConfig(values, editing)}
        />
      ) : null}
      {confirmation && (
        <ConfirmationDialog
          {...confirmation}
          close={() => setConfirmation(null)}
          onConfirm={async () => {
            const succeeded = await confirmation.confirm();
            if (succeeded !== false) setConfirmation(null);
          }}
        />
      )}
      {reinstalling && (
        <ReinstallDialog
          instance={reinstalling}
          close={() => setReinstalling(null)}
          onConfirm={async () => {
            await queue(`/api/instances/${reinstalling.id}/game-update`, {
              confirm: true,
				reinstall_game: false,
				reinstall_package: true,
            });
            setReinstalling(null);
          }}
        />
      )}
      {deleting ? (
        <DeleteInstanceDialog
          instance={deleting}
          close={() => setDeleting(null)}
          onConfirm={async () => {
            const succeeded = await deleteInstance(deleting.id);
            if (succeeded) setDeleting(null);
            return succeeded;
          }}
        />
      ) : null}
    </>
  );
}
function Terminal({
  instance,
  close,
}: {
  instance: Instance;
  close: () => void;
}) {
  const [output, setOutput] = useState("");
  const [input, setInput] = useState("");
  const [connected, setConnected] = useState(false);
  const socket = useRef<WebSocket | null>(null);
  const consoleFollow = useConsoleFollow(output);
  useEffect(() => {
    const protocol = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(
      `${protocol}://${location.host}/api/instances/${instance.id}/console`,
    );
    ws.binaryType = "arraybuffer";
    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);
    ws.onerror = () => setConnected(false);
    ws.onmessage = (e) => {
      const text =
        typeof e.data === "string" ? e.data : new TextDecoder().decode(e.data);
      setOutput((current) => appendConsoleOutput(current, text));
    };
    socket.current = ws;
    return () => ws.close();
  }, [instance.id]);
  return (
    <div className="terminal-backdrop">
      <section className="terminal-modal" role="dialog" aria-modal="true" aria-label={`${instance.name} 原生游戏控制台`}>
        <header className="terminal-head">
          <div className="terminal-title">
            <span className="terminal-mark"><TerminalSquare /></span>
            <div>
              <h3>原生游戏控制台 <small>（{instance.name}）</small></h3>
              <p><span className={connected ? "connected" : "connecting"}><i></i>{connected ? "控制台已建立连接" : "正在连接控制台"}</span><b>·</b> 端口: {instance.game_port}</p>
            </div>
          </div>
          <div className="terminal-head-actions">
            <button type="button" onClick={() => setOutput("")}><Trash2 />清空显示</button>
            <button type="button" className="terminal-close" aria-label="关闭控制台" onClick={close}><X /></button>
          </div>
        </header>
        <div className="terminal-body">
          <pre ref={consoleFollow.outputRef} onScroll={consoleFollow.onScroll}>
            {output || <span className="terminal-empty">暂无游戏控制台输出日志</span>}
          </pre>
          {!consoleFollow.following ? (
            <button type="button" className="terminal-resume" onClick={consoleFollow.forceFollow}><ArrowDown />继续跟随最新输出</button>
          ) : null}
        </div>
        <form
        onSubmit={(e) => {
          e.preventDefault();
          if (input) {
            consoleFollow.forceFollow();
            socket.current?.send(input + "\n");
            setInput("");
          }
        }}
      >
        <b>&gt;</b>
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="输入原生控制台指令 (例: status, changelevel c1m1_hotel, sm_say Hello)"
          autoFocus
        />
        <button aria-label="发送"><Send />发送命令</button>
        </form>
      </section>
    </div>
  );
}

const DEFAULT_PLUGIN_REPOSITORY =
  "PencilMario/L4D2-Not0721Here-CoopSvPlugins";
const DEFAULT_PLUGIN_ASSET_PATTERN =
  "^L4D2-Not0721Here-CoopSvPlugins-compiled\\.zip$";

function ContentPage({
  instances,
  packages,
  sharedGame,
  reloadPackages,
  reloadSharedGame,
  queue,
}: {
  instances: Instance[];
  packages: PackageVersion[];
  sharedGame: SharedGameState;
  reloadPackages: () => Promise<void>;
  reloadSharedGame: () => Promise<void>;
  queue: (path: string, body: any) => Promise<void>;
}) {
  const [vpks, setVpks] = useState<any[]>([]);
  const [selected, setSelected] = useState(instances[0]?.id || "");
  const [contentError, setContentError] = useState("");
  const [vpkUploadTasks, setVPKUploadTasks] = useState<VPKUploadTask[]>([]);
  const [vpkUploadMode, setVPKUploadMode] = useState<VPKUploadMode>("clean");
  const [vpkDragActive, setVPKDragActive] = useState(false);
  const [packageDragActive, setPackageDragActive] = useState(false);
  const vpkInputRef = useRef<HTMLInputElement>(null);
  const packageInputRef = useRef<HTMLInputElement>(null);
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const [sources, setSources] = useState<GitHubSource[]>([]);
  const [sourceEditor, setSourceEditor] = useState<GitHubSource | null>(null);
  const [gamePolicy, setGamePolicy] = useState("wait");
  const [contentTab, setContentTab] = useState<"game" | "vpk" | "packages" | "github">("game");
  const contentActions = useAsyncLocks();
  const loadVPK = () => api<any[]>("/api/content/vpk").then(setVpks);
  const loadSources = () => api<GitHubSource[]>("/api/github-sources").then((items) => setSources(Array.isArray(items) ? items : []));
  useEffect(() => {
    Promise.all([loadVPK(), reloadPackages(), loadSources()]).catch((reason) =>
      setContentError(errorMessage(reason)),
    );
  }, []);
  useEffect(() => { let cleanup: (() => void) | undefined; void startVPKUploadQueue(setVPKUploadTasks, () => { void loadVPK(); }).then((stop) => { cleanup = stop; }); return () => cleanup?.(); }, []);
  const uploadPackage = async (file: File) => {
    await api(
      `/api/packages/uploads?filename=${encodeURIComponent(file.name)}&version=${encodeURIComponent(file.name)}`,
      {
        method: "POST",
        headers: { "Content-Type": "application/zip" },
        body: file,
      },
    );
    await Promise.all([loadVPK(), reloadPackages()]);
  };
  const addPackageFiles = (files: File[]) => {
    const file = files.find((item) => item.name.toLowerCase().endsWith(".zip"));
    if (!file) {
      setContentError("请选择 .zip 插件包");
      return;
    }
    void runContentAction("upload:package", () => uploadPackage(file));
  };
  const addVPKFiles = (files: File[]) => {
    const accepted = files.filter((file) => file.name.toLowerCase().endsWith(".vpk"));
    if (!accepted.length) {
      setContentError("请选择 .vpk 文件");
      return;
    }
    setContentError("");
    void enqueueVPKUploads(accepted.map((file) => ({ file, mode: vpkUploadMode })))
      .catch((reason) => setContentError(errorMessage(reason)));
  };
  const renameVPK = async (name: string) => {
    const next = window.prompt("新的 VPK 文件名", name);
    if (
      !next ||
      next === name ||
      !window.confirm("重命名可见 VPK？运行中的实例可能需要换图或重启。")
    ) {
      return;
    }
    await api(`/api/content/vpk/${encodeURIComponent(name)}/rename`, {
      method: "POST",
      body: JSON.stringify({ name: next, confirm: true }),
    });
    await loadVPK();
  };
  const deleteVPK = async (name: string) => {
    if (!window.confirm(`删除 ${name}？运行中的实例可能仍缓存该内容。`)) {
      return;
    }
    await api(`/api/content/vpk/${encodeURIComponent(name)}?confirm=true`, {
      method: "DELETE",
    });
    await loadVPK();
  };
  const cleanVPK = async (name: string) => {
    if (!window.confirm(`清理 ${name} 中服务器不需要的资源并覆盖原文件？`)) return;
    await api(`/api/content/vpk/${encodeURIComponent(name)}/clean`, {
      method: "POST",
      body: JSON.stringify({ confirm: true }),
    });
    await loadVPK();
  };
  const runContentAction = (key: string, operation: () => Promise<unknown>) => {
    setContentError("");
    return contentActions.run(key, operation).catch((reason) => {
      setContentError(errorMessage(reason));
      return false;
    });
  };
  return (
    <div className="content-layout">
      {contentError && (
        <div className="error" role="alert">
          {contentError}
        </div>
      )}
      <div className="content-tabs" role="tablist" aria-label="内容仓库分类">
        {[
          { value: "game", label: "共享游戏本体", icon: Server, count: null },
          { value: "vpk", label: "共享 VPK", icon: FileArchive, count: vpks.length },
          { value: "packages", label: "插件包", icon: FolderGit2, count: packages.length },
          { value: "github", label: "GitHub 发布源", icon: ExternalLink, count: sources.length },
        ].map(({ value, label, icon: Icon, count }) => (
          <button
            aria-label={label}
            aria-selected={contentTab === value}
            className={contentTab === value ? "active" : ""}
            key={value}
            role="tab"
            type="button"
            onClick={() => setContentTab(value as typeof contentTab)}
          >
            <Icon />
            <span>{label}</span>
            {count !== null ? <em aria-hidden="true"> ({count})</em> : null}
          </button>
        ))}
      </div>
      <div className="content-tab-panel" role="tabpanel" hidden={contentTab !== "game"}>
        <section className="shared-game-panel">
          <div className="shared-game-details">
            <div>
              <small>当前安装版本</small>
              <b>{sharedGameVersionLabel(sharedGame)}</b>
            </div>
            <div>
              <small>物理保存位置</small>
              <code>{sharedGame.path || "/data/game/current"}</code>
            </div>
            <div>
              <small>初始化就绪状态</small>
              <b className={sharedGame.migration_state === "ready" ? "ready" : "pending"}>
                <CheckCircle2 />
                {sharedGame.migration_state === "ready" ? "就绪 (全量内容未损坏)" : sharedGame.migration_state || "未知"}
              </b>
            </div>
          </div>

          <div className="shared-game-update-panel">
            <h3><RefreshCw />触发共享游戏本体更新</h3>
            <fieldset>
              <legend>在线玩家处理策略 (在线玩家策略)</legend>
              <div className="shared-game-policies">
                {[
                  { value: "skip", title: "有玩家时跳过", description: "保护对局无打扰" },
                  { value: "wait", title: "等待玩家离开", description: "空服后自动执行" },
                  { value: "force", title: "强制执行", description: "中断当前在线连接" },
                ].map((policy) => (
                  <label className={`shared-game-policy-card ${policy.value === "force" ? "force" : ""}`} key={policy.value}>
                    <input
                      type="radio"
                      name="shared-game-policy"
                      value={policy.value}
                      checked={gamePolicy === policy.value}
                      onChange={() => setGamePolicy(policy.value)}
                    />
                    <span>
                      <b>{policy.title}</b>
                      <small>{policy.description}</small>
                    </span>
                  </label>
                ))}
              </div>
            </fieldset>

            <div className="shared-game-update-actions">
          <button
                className="shared-game-update"
            disabled={contentActions.pending.has("game:update")}
            aria-busy={contentActions.pending.has("game:update")}
            onClick={() => {
              if (!window.confirm("更新共享游戏本体？所有依赖实例均符合在线玩家策略后才会停止并更新。")) return;
              void runContentAction("game:update", async () => {
                await queue("/api/game/update", { confirm: true, online_policy: gamePolicy });
                await reloadSharedGame();
              });
            }}
          >
                <RefreshCw /><span>更新游戏本体</span>
          </button>
            </div>
          </div>
        </section>
      </div>
      <div className="content-tab-panel" role="tabpanel" hidden={contentTab !== "vpk"}>
      <VPKUploadQueue tasks={vpkUploadTasks} onRetry={(task) => void retryVPKUpload(task)} onCancel={(task) => void cancelVPKUpload(task)} />
      <section
        className={`content-vpk-drop ${vpkDragActive ? "dragging" : ""}`}
        aria-label="VPK 上传区"
        tabIndex={0}
        onClick={(event) => { if (!(event.target as HTMLElement).closest("button")) vpkInputRef.current?.click(); }}
        onKeyDown={(event) => { if ((event.key === "Enter" || event.key === " ") && event.target === event.currentTarget) { event.preventDefault(); vpkInputRef.current?.click(); } }}
        onDragEnter={(event) => { event.preventDefault(); setVPKDragActive(true); }}
        onDragOver={(event) => { event.preventDefault(); setVPKDragActive(true); }}
        onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setVPKDragActive(false); }}
        onDrop={(event) => { event.preventDefault(); setVPKDragActive(false); addVPKFiles(Array.from(event.dataTransfer.files)); }}
      >
        <div className="content-vpk-upload-icon"><Upload /></div>
        <div className="content-vpk-drop-copy">
          <h2>拖放 .vpk 文件至此处进行上传</h2>
          <p>支持多文件同时加入队列，文件将供所有游戏实例共享使用</p>
        </div>
        <div className="content-vpk-upload-controls">
          <span>上传处理模式:</span>
          <div className="content-vpk-mode-switch">
            <button type="button" aria-pressed={vpkUploadMode === "clean"} onClick={() => setVPKUploadMode("clean")}>上传前清理</button>
            <button type="button" aria-pressed={vpkUploadMode === "direct"} onClick={() => setVPKUploadMode("direct")}>直接上传</button>
          </div>
          <input ref={vpkInputRef} className="content-drop-input" aria-label="上传 VPK" type="file" accept=".vpk" multiple onChange={(event) => { addVPKFiles(Array.from(event.target.files || [])); event.target.value = ""; }} />
        </div>
      </section>
      <section className="content-vpk-library" aria-labelledby="content-vpk-library-title">
        <header><h2 id="content-vpk-library-title">共享 VPK 库 ({vpks.length})</h2></header>
        {vpks.length ? <div className="content-vpk-table-wrap"><table className="content-vpk-table">
          <thead><tr><th>文件名</th><th>大小</th><th>校验 Hash</th><th>操作</th></tr></thead>
          <tbody>{vpks.map((x) => (
            <tr key={x.name}>
              <td><b>{x.name}</b></td>
              <td>{formatBytes(x.size)}</td>
              <td><code>{String(x.hash).slice(0, 16)}</code></td>
              <td><div className="content-vpk-actions">
                <a title="下载" aria-label={`下载 ${x.name}`} download href={`/api/content/vpk/${encodeURIComponent(x.name)}/download`}><Download /></a>
                <button title="重命名" aria-label={`重命名 ${x.name}`} disabled={contentActions.pending.has(`vpk:rename:${x.name}`)} aria-busy={contentActions.pending.has(`vpk:rename:${x.name}`)} onClick={() => runContentAction(`vpk:rename:${x.name}`, () => renameVPK(x.name))}><Edit3 /></button>
                <button title="手动清理" aria-label={`清理资源 ${x.name}`} disabled={contentActions.pending.has(`vpk:clean:${x.name}`)} aria-busy={contentActions.pending.has(`vpk:clean:${x.name}`)} onClick={() => runContentAction(`vpk:clean:${x.name}`, () => cleanVPK(x.name))}><RefreshCw /></button>
                <button title="删除" aria-label={`删除 ${x.name}`} className="danger" disabled={contentActions.pending.has(`vpk:delete:${x.name}`)} aria-busy={contentActions.pending.has(`vpk:delete:${x.name}`)} onClick={() => runContentAction(`vpk:delete:${x.name}`, () => deleteVPK(x.name))}><Trash2 /></button>
              </div></td>
            </tr>
          ))}</tbody>
        </table></div> : <div className="empty">暂无共享 VPK</div>}
      </section>
      </div>
      <div className="content-tab-panel" role="tabpanel" hidden={contentTab !== "packages" && contentTab !== "github"}>
      {contentTab === "packages" ? (
        <section
          className={`content-package-drop ${packageDragActive ? "dragging" : ""}`}
          aria-label="插件包上传区"
          tabIndex={0}
          aria-busy={contentActions.pending.has("upload:package")}
          onClick={() => { if (!contentActions.pending.has("upload:package")) packageInputRef.current?.click(); }}
          onKeyDown={(event) => { if ((event.key === "Enter" || event.key === " ") && !contentActions.pending.has("upload:package")) { event.preventDefault(); packageInputRef.current?.click(); } }}
          onDragEnter={(event) => { event.preventDefault(); setPackageDragActive(true); }}
          onDragOver={(event) => { event.preventDefault(); setPackageDragActive(true); }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setPackageDragActive(false); }}
          onDrop={(event) => { event.preventDefault(); setPackageDragActive(false); if (!contentActions.pending.has("upload:package")) addPackageFiles(Array.from(event.dataTransfer.files)); }}
        >
          <div className="content-package-upload-icon">{contentActions.pending.has("upload:package") ? <RefreshCw /> : <Upload />}</div>
          <div><h2>{contentActions.pending.has("upload:package") ? "正在上传插件包" : "拖放 .zip 插件包至此处上传"}</h2><p>上传后可部署至指定游戏实例，支持 SourceMod 与 Metamod 插件归档</p></div>
          <input ref={packageInputRef} className="content-drop-input" aria-label="上传 ZIP" type="file" accept=".zip" disabled={contentActions.pending.has("upload:package")} onChange={(event) => { addPackageFiles(Array.from(event.target.files || [])); event.target.value = ""; }} />
        </section>
      ) : null}
      {contentTab === "packages" ? (
        <label className="content-instance-selector">
          更新目标实例
          <select value={selected} onChange={(event) => setSelected(event.target.value)}>
            {instances.map((instance) => (
              <option key={instance.id} value={instance.id}>{instance.name}</option>
            ))}
          </select>
        </label>
      ) : null}
      <Panel
        title={contentTab === "github" ? "GitHub 发布源" : "插件包"}
        action={
          contentTab === "github" ? (
            <button onClick={() => setSourceEditor({ id: "", name: "", repository: "", asset_pattern: "" })}><Plus />添加 GitHub 源</button>
          ) : undefined
        }
      >
        {contentTab === "github" && sourceEditor ? (
          <form className="release-source" onSubmit={(event) => {
            event.preventDefault();
            runContentAction(`source:save:${sourceEditor.id || "new"}`, async () => {
              await api(sourceEditor.id ? `/api/github-sources/${sourceEditor.id}` : "/api/github-sources", {
                method: sourceEditor.id ? "PUT" : "POST",
                body: JSON.stringify({ name: sourceEditor.name, repository: sourceEditor.repository, asset_pattern: sourceEditor.asset_pattern }),
              });
              setSourceEditor(null);
              await loadSources();
            });
          }}>
            <label>源名称<input aria-label="源名称" value={sourceEditor.name} onChange={(event) => setSourceEditor({ ...sourceEditor, name: event.target.value })} required /></label>
            <label>GitHub 仓库<input aria-label="GitHub 仓库" value={sourceEditor.repository} onChange={(event) => setSourceEditor({ ...sourceEditor, repository: event.target.value })} required /></label>
            <label>Release 资源规则<input aria-label="Release 资源规则" value={sourceEditor.asset_pattern} onChange={(event) => setSourceEditor({ ...sourceEditor, asset_pattern: event.target.value })} required /></label>
            <div className="inline-actions"><button className="command-primary" disabled={contentActions.pending.has(`source:save:${sourceEditor.id || "new"}`)} aria-busy={contentActions.pending.has(`source:save:${sourceEditor.id || "new"}`)}>{contentActions.pending.has(`source:save:${sourceEditor.id || "new"}`) ? <><RefreshCw />保存中…</> : "保存源"}</button><button type="button" onClick={() => setSourceEditor(null)}>取消</button></div>
          </form>
        ) : null}
        {contentTab === "github" ? <div className="source-grid">
          {sources.map((source) => (
            <article className="source-card" key={source.id}>
              <div><b>{source.name}</b><small>{source.repository}</small><code>{source.asset_pattern}</code></div>
              <div className="inline-actions">
                <button disabled={contentActions.pending.has(`source:check:${source.id}`)} aria-busy={contentActions.pending.has(`source:check:${source.id}`)} aria-label={`同步 ${source.name}`} onClick={() => runContentAction(`source:check:${source.id}`, () => queue(`/api/github-sources/${source.id}/check`, {}))}>同步</button>
                <button onClick={() => setSourceEditor(source)}><Edit3 />编辑</button>
                <button className="danger" disabled={contentActions.pending.has(`source:delete:${source.id}`)} aria-busy={contentActions.pending.has(`source:delete:${source.id}`)} onClick={() => { if (window.confirm(`删除源 ${source.name}？已下载插件包会保留。`)) runContentAction(`source:delete:${source.id}`, async () => { await api(`/api/github-sources/${source.id}`, { method: "DELETE" }); await loadSources(); }); }}>删除</button>
              </div>
            </article>
          ))}
        </div> : null}
        {contentTab === "packages" ? packages.map((x) => (
          <div className="data-row" key={x.id}>
            <div>
              <b>
                {x.filename} · {x.version}
              </b>
              <small>
                {formatBytes(x.size)} ·{" "}
                {x.source_repository ? "GitHub Release 版本" : "常规插件包"}
              </small>
            </div>
          </div>
        )) : null}
        {contentTab === "packages" && !packages.length ? <div className="empty">暂无插件包</div> : null}
        {contentTab === "github" && !sources.length ? <div className="empty">暂无 GitHub 发布源</div> : null}
      </Panel>
      </div>
      {confirmation && (
        <ConfirmationDialog
          title={confirmation.title}
          description={confirmation.description}
          confirmLabel={confirmation.confirmLabel}
          close={() => setConfirmation(null)}
          onConfirm={async () => {
            const succeeded = await confirmation.confirm();
            if (succeeded !== false) setConfirmation(null);
          }}
        />
      )}
    </div>
  );
}
function PlayersModal({
  instance,
  close,
  queue,
}: {
  instance: Instance;
  close: () => void;
  queue: (path: string, body: any) => Promise<void>;
}) {
  const [snapshot, setSnapshot] = useState<PlayerSnapshot | null>(null);
  const [playersError, setPlayersError] = useState("");
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  useEffect(() => {
    api<PlayerSnapshot>(`/api/instances/${instance.id}/players`)
      .then(setSnapshot)
      .catch((reason) => {
        setSnapshot({ players: [] });
        setPlayersError(errorMessage(reason));
      });
  }, [instance.id]);
  const requestAction = (player: OnlinePlayer, action: "kick" | "ban") => {
    const kick = action === "kick";
    setConfirmation({
      title: kick ? `踢出 ${player.name}？` : `永久封禁 ${player.name}？`,
      description: kick
        ? "玩家会立即从当前服务器断开。"
        : "该玩家将被永久封禁，直至管理员手动解除。",
      confirmLabel: kick ? "确认踢出" : "确认永久封禁",
      confirm: async () => {
        setPlayersError("");
        try {
          await queue(
            `/api/instances/${instance.id}/players/${player.user_id}/actions`,
            {
              action,
              ...(kick ? {} : { minutes: 0 }),
              confirm: true,
            },
          );
        } catch (reason) {
          setPlayersError(errorMessage(reason));
          throw reason;
        }
      },
    });
  };
  return (
    <>
      <div className="modal-wrap">
        <div className="modal players-modal" role="dialog" aria-modal="true" aria-labelledby="players-title">
          <header className="players-head">
            <div className="players-heading">
              <span className="players-heading-icon"><Users /></span>
              <div>
                <h2 id="players-title">在线玩家与对局摘要</h2>
                <p>{instance.name} ({snapshot?.players?.length ?? 0} / {snapshot?.max_players ?? instance.max_players} 人在线)</p>
              </div>
            </div>
            <button className="players-close" aria-label="关闭玩家列表" onClick={close}>
              <X />
            </button>
          </header>
          {playersError && (
            <div className="error" role="alert">
              {playersError}
            </div>
          )}
          <section className="players-summary" aria-label="对局摘要">
            <PlayerSummaryItem icon={<MapPin />} label="当前地图" value={snapshot?.match?.map || snapshot?.map || instance.current_map || instance.start_map} />
            <PlayerSummaryItem icon={<Users />} label="在线容量" value={`${snapshot?.players?.length ?? 0} / ${snapshot?.max_players ?? snapshot?.match?.max_players ?? instance.max_players}`} />
            <PlayerSummaryItem icon={<Globe2 />} label="公网 IP" value={snapshot?.match?.public_address || "--"} />
          </section>
          {snapshot?.players?.length ? (
            <div className="players-list">
              <div className="player-operations-wrap">
              <table className="player-operations">
                <thead>
                  <tr><th>玩家名称</th><th>Steam ID</th><th>加入时长</th><th>延迟</th><th>得分</th><th>处置操作</th></tr>
                </thead>
                <tbody>
                  {snapshot.players.map((player) => (
                    <tr className="player-row" key={`${player.name}-${player.user_id}`}>
                      <td data-label="玩家名称"><b>{player.name}</b></td>
                      <td data-label="Steam ID"><code>{player.unique_id || "--"}</code></td>
                      <td className="player-connected" data-label="加入时长">{player.connected || "--"}</td>
                      <td className="player-ping" data-label="延迟">{player.ping === undefined ? "--" : `${player.ping} ms`}</td>
                      <td className="player-score" data-label="得分">{player.score ?? "--"}</td>
                      <td data-label="处置操作">
                        {player.user_id > 0 ? (
                          <div className="player-actions">
                            <button className="player-kick" onClick={() => requestAction(player, "kick")}><UserX />踢出玩家</button>
                            <button className="player-ban" onClick={() => requestAction(player, "ban")}><Ban />永久封禁</button>
                          </div>
                        ) : "--"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              </div>
            </div>
          ) : null}
          {snapshot && !snapshot.players?.length && (
            <div className="players-empty">当前服务器暂无在线玩家</div>
          )}
        </div>
      </div>
      {confirmation && (
        <ConfirmationDialog
          {...confirmation}
          close={() => setConfirmation(null)}
          onConfirm={async () => {
            const succeeded = await confirmation.confirm();
            if (succeeded !== false) setConfirmation(null);
          }}
        />
      )}
    </>
  );
}

function PlayerSummaryItem({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return (
    <div className="players-summary-item">
      {icon}
      <div>
        <small>{label}</small>
        <b>{value || "--"}</b>
      </div>
    </div>
  );
}

function SettingsPage() {
  const [steam, setSteam] = useState(false);
  const [github, setGithub] = useState(false);
  const [settingsError, setSettingsError] = useState("");
  const [confirmedJobLimit, setConfirmedJobLimit] = useState(25);
  const [draftJobLimit, setDraftJobLimit] = useState("25");
  const [jobSettingsReady, setJobSettingsReady] = useState(false);
  const [savingJobs, setSavingJobs] = useState(false);
  const [jobsNotice, setJobsNotice] = useState("");
  const [confirmedGameLogDays, setConfirmedGameLogDays] = useState(14);
  const [draftGameLogDays, setDraftGameLogDays] = useState("14");
  const [gameLogSettingsReady, setGameLogSettingsReady] = useState(false);
  const [gameLogBusy, setGameLogBusy] = useState<"save" | "cleanup" | "">("");
  const [gameLogsNotice, setGameLogsNotice] = useState("");
  const gameLogRequestSequence = useRef(0);
  const settingsActions = useAsyncLocks();
  useEffect(() => {
    api<any>("/api/settings/steam")
      .then((x) => setSteam(x.configured))
      .catch((reason) => setSettingsError(errorMessage(reason)));
    api<any>("/api/settings/github-token")
      .then((x) => setGithub(x.configured))
      .catch((reason) => setSettingsError(errorMessage(reason)));
    api<{ successful_job_limit: number }>("/api/settings/jobs")
      .then((settings) => {
        if (!Number.isInteger(settings.successful_job_limit)) {
          throw new Error("任务记录设置数据无效");
        }
        setConfirmedJobLimit(settings.successful_job_limit);
        setDraftJobLimit(String(settings.successful_job_limit));
        setJobSettingsReady(true);
      })
      .catch((reason) => setSettingsError(errorMessage(reason)));
    const gameLogLoadSequence = ++gameLogRequestSequence.current;
    api<{ retention_days: number }>("/api/settings/game-logs")
      .then((settings) => {
        if (gameLogLoadSequence !== gameLogRequestSequence.current) return;
        if (!Number.isInteger(settings.retention_days) || settings.retention_days < 1 || settings.retention_days > 365) {
          throw new Error("游戏日志设置数据无效");
        }
        setConfirmedGameLogDays(settings.retention_days);
        setDraftGameLogDays(String(settings.retention_days));
        setGameLogSettingsReady(true);
      })
      .catch((reason) => {
        if (gameLogLoadSequence === gameLogRequestSequence.current) {
          setSettingsError(errorMessage(reason));
        }
      });
  }, []);
  const saveSteam = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    const form = e.currentTarget;
    setSettingsError("");
    try {
      await settingsActions.run("steam", async () => {
        await api("/api/settings/steam", {
          method: "PUT",
          body: JSON.stringify({
            username: data.get("username"),
            password: data.get("password"),
          }),
        });
        setSteam(true);
        form.reset();
      });
    } catch (reason) {
      setSettingsError(errorMessage(reason));
    }
  };
  const saveGithub = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    const form = e.currentTarget;
    setSettingsError("");
    try {
      await settingsActions.run("github", async () => {
        await api("/api/settings/github-token", {
          method: "PUT",
          body: JSON.stringify({ token: data.get("token") }),
        });
        setGithub(true);
        form.reset();
      });
    } catch (reason) {
      setSettingsError(errorMessage(reason));
    }
  };
  const saveJobSettings = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (settingsActions.isLocked("jobs")) return;
    const limit = Number(draftJobLimit);
    if (!Number.isInteger(limit) || limit < 1 || limit > 500) {
      setSettingsError("已完成任务保留数量必须为 1 至 500 的整数");
      return;
    }
    setSettingsError("");
    setJobsNotice("");
    setSavingJobs(true);
    try {
      await settingsActions.run("jobs", async () => {
        const saved = await api<{ successful_job_limit: number }>(
          "/api/settings/jobs",
          {
            method: "PUT",
            body: JSON.stringify({ successful_job_limit: limit }),
          },
        );
        setConfirmedJobLimit(saved.successful_job_limit);
        setDraftJobLimit(String(saved.successful_job_limit));
        setJobsNotice("任务记录设置已保存");
      });
    } catch (reason) {
      setDraftJobLimit(String(confirmedJobLimit));
      setSettingsError(errorMessage(reason));
    } finally {
      setSavingJobs(false);
    }
  };
  type EnqueueStats = { Queued: number; Deduplicated: number; Failed: number };
  const formatEnqueueStats = (stats: EnqueueStats) =>
    `已排队 ${stats.Queued}，已去重 ${stats.Deduplicated}，失败 ${stats.Failed}`;
  const saveGameLogSettings = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (settingsActions.isLocked("game-logs")) return;
    const days = Number(draftGameLogDays);
    if (!Number.isInteger(days) || days < 1 || days > 365) {
      setSettingsError("游戏日志保留天数必须为 1 至 365 的整数");
      return;
    }
    setSettingsError("");
    setGameLogsNotice("");
    setGameLogBusy("save");
    const sequence = ++gameLogRequestSequence.current;
    try {
      await settingsActions.run("game-logs", async () => {
        const saved = await api<{ retention_days: number; enqueue: EnqueueStats }>(
          "/api/settings/game-logs",
          { method: "PUT", body: JSON.stringify({ retention_days: days }) },
        );
        if (sequence === gameLogRequestSequence.current) {
          setConfirmedGameLogDays(saved.retention_days);
          setDraftGameLogDays(String(saved.retention_days));
          setGameLogsNotice(`游戏日志设置已保存；${formatEnqueueStats(saved.enqueue)}`);
        }
      });
    } catch (reason) {
      if (sequence === gameLogRequestSequence.current) {
        setDraftGameLogDays(String(confirmedGameLogDays));
        setSettingsError(errorMessage(reason));
      }
    } finally {
      if (sequence === gameLogRequestSequence.current) setGameLogBusy("");
    }
  };
  const cleanupGameLogs = async () => {
    if (settingsActions.isLocked("game-logs")) return;
    setSettingsError("");
    setGameLogsNotice("");
    setGameLogBusy("cleanup");
    const sequence = ++gameLogRequestSequence.current;
    try {
      await settingsActions.run("game-logs", async () => {
        const result = await api<EnqueueStats>("/api/settings/game-logs/cleanup", {
          method: "POST",
        });
        if (sequence === gameLogRequestSequence.current) {
          setGameLogsNotice(`清理任务已提交；${formatEnqueueStats(result)}`);
        }
      });
    } catch (reason) {
      if (sequence === gameLogRequestSequence.current) {
        setSettingsError(errorMessage(reason));
      }
    } finally {
      if (sequence === gameLogRequestSequence.current) setGameLogBusy("");
    }
  };
  return (
    <div className="settings-reference-page">
      <header className="settings-reference-head">
        <h2>系统设置</h2>
        <p>配置 SteamCMD 登录凭据、GitHub 访问令牌、后台任务保留上限及日志清理策略</p>
      </header>
      {settingsError && (
        <div className="error" role="alert">
          {settingsError}
        </div>
      )}
      <div className="settings-reference-grid">
        <form className="settings-card" onSubmit={saveSteam}>
          <div className="settings-card-title"><h3><Key />Steam 安装凭据</h3><span className={steam ? "configured" : "unconfigured"}>{steam ? "已配置" : "未配置"}</span></div>
          <p>{steam ? "已加密配置；匿名首装仍可用" : "匿名首装已支持；仅许可账号需要配置凭据"}</p>
          <div className="settings-fields"><label>Steam 账号用户名<input aria-label="用户名" name="username" autoComplete="username" placeholder="Steam 登录用户名" required /></label><label>Steam 账号密码<input aria-label="密码" name="password" type="password" autoComplete="current-password" placeholder="••••••••••••" required /></label></div>
          <footer><small><Lock />敏感信息密文存储</small><button className="settings-save" aria-label="加密保存" disabled={settingsActions.pending.has("steam")} aria-busy={settingsActions.pending.has("steam")}>{settingsActions.pending.has("steam") ? <RefreshCw /> : <Save />}<span>{settingsActions.pending.has("steam") ? "保存中…" : "保存 Steam 凭据"}</span></button></footer>
        </form>
        <form className="settings-card" onSubmit={saveGithub}>
          <div className="settings-card-title"><h3><ShieldCheck />GitHub 访问令牌 <span>(Personal Access Token)</span></h3><span className={github ? "configured" : "unconfigured"}>{github ? "已配置" : "未配置"}</span></div>
          <p>用于突破 GitHub REST API 速率限制并访问私有插件发布仓库。</p>
          <div className="settings-fields"><label>Personal Access Token (ghp_...)<input name="token" type="password" placeholder="ghp_xxxxxxxxxxxxxxxxxxxx" required /></label></div>
          <footer><small>未配置时公开仓库仍可受限访问</small><button className="settings-save" disabled={settingsActions.pending.has("github")} aria-busy={settingsActions.pending.has("github")}>{settingsActions.pending.has("github") ? <RefreshCw /> : <Save />}<span>{settingsActions.pending.has("github") ? "保存中…" : "更新 GitHub 令牌"}</span></button></footer>
        </form>
        <form className="settings-card" onSubmit={saveJobSettings}>
          <div className="settings-card-title"><h3><Clock />后台任务记录保留上限</h3></div>
          <p>设定已完成或已失败的历史任务保留数量上限（允许范围：1 - 500 条）。</p>
          <div className="settings-fields"><label>已完成任务保留数量<input type="number" min={1} max={500} step={1} required value={draftJobLimit} disabled={!jobSettingsReady || savingJobs} onChange={(event) => { setDraftJobLimit(event.target.value); setJobsNotice(""); }} /></label></div>
          {jobsNotice ? <p className="settings-notice" role="status">{jobsNotice}</p> : null}
          <footer><small>除正在运行的任务外，所有已结束任务共用此保留上限。</small><button className="settings-save" type="submit" aria-label="保存任务记录设置" disabled={!jobSettingsReady || savingJobs} aria-busy={savingJobs}>{savingJobs ? <RefreshCw /> : <Save />}<span>{savingJobs ? "保存中…" : "保存任务保留设置"}</span></button></footer>
        </form>
        <section className="settings-card" aria-labelledby="game-log-settings-title">
          <div className="settings-card-title"><h3 id="game-log-settings-title"><Trash2 />游戏日志保留策略</h3></div>
          <p>设定游戏控制台与 SourceMod 插件日志保留天数（允许范围：1 - 365 天）。</p>
          <form className="settings-fields" onSubmit={saveGameLogSettings}><label>游戏日志保留天数<input type="number" min={1} max={365} step={1} required value={draftGameLogDays} disabled={!gameLogSettingsReady || gameLogBusy !== ""} onChange={(event) => { const value = event.target.value; setDraftGameLogDays(value); setGameLogsNotice(""); const days = Number(value); setSettingsError(value !== "" && (!Number.isInteger(days) || days < 1 || days > 365) ? "游戏日志保留天数必须为 1 至 365 的整数" : ""); }} /></label><p>当前确认值：{confirmedGameLogDays} 天</p><footer><button className="settings-cleanup" type="button" aria-label="立即清理游戏日志" disabled={!gameLogSettingsReady || gameLogBusy !== ""} aria-busy={gameLogBusy === "cleanup"} onClick={() => void cleanupGameLogs()}>{gameLogBusy === "cleanup" ? "提交中…" : "立即清理过期日志"}</button><button className="settings-save" type="submit" aria-label="保存游戏日志设置" disabled={!gameLogSettingsReady || gameLogBusy !== ""} aria-busy={gameLogBusy === "save"}>{gameLogBusy === "save" ? <RefreshCw /> : <Save />}<span>{gameLogBusy === "save" ? "保存中…" : "保存日志策略"}</span></button></footer></form>
          {gameLogsNotice ? <p className="settings-notice" role="status">{gameLogsNotice}</p> : null}
        </section>
      </div>
    </div>
  );
}

function FileButton({
  label,
  accept,
  onFile,
  onFiles,
  multiple = false,
  disabled = false,
  busy = false,
}: {
  label: string;
  accept: string;
  onFile?: (f: File) => void;
  onFiles?: (files: File[]) => void;
  multiple?: boolean;
  disabled?: boolean;
  busy?: boolean;
}) {
  return (
    <label className="command-primary file-button" aria-busy={busy} aria-disabled={disabled}>
      {busy ? <RefreshCw /> : <Upload />}
      {busy ? "处理中…" : label}
      <input
        type="file"
        accept={accept}
        multiple={multiple}
        disabled={disabled}
        onChange={(e) => { const files = Array.from(e.target.files || []); if (files.length) { onFiles?.(files); onFile?.(files[0]); } e.target.value = ""; }}
      />
    </label>
  );
}
function Panel({
  title,
  action,
  children,
}: {
  title: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="data-panel">
      <div className="section-head">
        <h2>{title}</h2>
        {action}
      </div>
      {children}
    </section>
  );
}
function Confirm({
  instance,
  close,
  confirm,
}: {
  instance: Instance;
  close: () => void;
  confirm: () => void;
}) {
  return (
    <div className="modal-wrap">
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-title"
      >
        <span className="danger-icon">
          <CircleStop />
        </span>
        <p className="eyebrow">DESTRUCTIVE ACTION</p>
        <h2 id="confirm-title">停止 {instance.name}？</h2>
        <p>
          服务器将先通过原生控制台执行 quit，再进入 Docker
          宽限停止。在线玩家会断开连接。
        </p>
        <div>
          <button onClick={close}>取消</button>
          <button className="danger" aria-label="确认停止" onClick={confirm}>
            确认停止
          </button>
        </div>
      </div>
    </div>
  );
}
function ConfirmationDialog({
  title,
  description,
  confirmLabel,
  close,
  onConfirm,
}: {
  title: string;
  description: string;
  confirmLabel: string;
  close: () => void;
  onConfirm: () => Promise<boolean | void> | boolean | void;
}) {
  const dialog = useRef<HTMLDivElement | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const confirm = async () => {
    if (submittingRef.current) return;
    submittingRef.current = true;
    setSubmitting(true);
    try {
      await onConfirm();
    } catch {
      // The owning feature keeps the dialog open and renders the request error.
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  };
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (submitting) return;
      if (event.key === "Escape") {
        close();
        return;
      }
      if (event.key !== "Tab" || !dialog.current) return;
      const focusable = Array.from(
        dialog.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ),
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [close, submitting]);
  return (
    <div className="modal-wrap">
      <div
        ref={dialog}
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirmation-title"
      >
        <span className="danger-icon">
          <CircleStop />
        </span>
        <p className="eyebrow">CONFIRM OPERATION</p>
        <h2 id="confirmation-title">{title}</h2>
        <p>{description}</p>
        <div>
          <button disabled={submitting} onClick={close}>取消</button>
          <button
            autoFocus
            className="danger"
            aria-label={confirmLabel}
            disabled={submitting}
            aria-busy={submitting}
            onClick={() => void confirm()}
          >
            {submitting ? <RefreshCw /> : null}
            {submitting ? "处理中…" : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

function DeleteInstanceDialog({
  instance,
  close,
  onConfirm,
}: {
  instance: Instance;
  close: () => void;
  onConfirm: () => Promise<boolean>;
}) {
  const [confirmationName, setConfirmationName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const confirmed = confirmationName === instance.name;
  const confirm = async () => {
    if (!confirmed || submittingRef.current) return;
    submittingRef.current = true;
    setSubmitting(true);
    try {
      await onConfirm();
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  };
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !submitting) close();
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [close, submitting]);
  return (
    <div className="modal-wrap">
      <div
        className="modal instance-delete-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="instance-delete-title"
      >
        <span className="danger-icon"><Trash2 /></span>
        <p className="eyebrow">PERMANENT DELETION</p>
        <h2 id="instance-delete-title">永久删除 {instance.name}？</h2>
        <p>此操作不可恢复，将永久删除：</p>
        <ul>
          <li>实例记录和托管游戏容器</li>
          <li>游戏文件、私有配置、插件、存档和备份</li>
          <li>实例数据目录和控制台数据</li>
        </ul>
        <label>
          <span>输入实例名称 <b>{instance.name}</b> 确认</span>
          <input
            autoFocus
            aria-label="输入实例名称确认"
            disabled={submitting}
            value={confirmationName}
            onChange={(event) => setConfirmationName(event.target.value)}
          />
        </label>
        <div>
          <button disabled={submitting} onClick={close}>取消</button>
          <button
            className="danger"
            disabled={!confirmed || submitting}
            aria-busy={submitting}
            onClick={() => void confirm()}
          >
            {submitting ? <RefreshCw /> : <Trash2 />}
            {submitting ? "删除中…" : "永久删除"}
          </button>
        </div>
      </div>
    </div>
  );
}

function ReinstallDialog({
  instance,
  close,
  onConfirm,
}: {
  instance: Instance;
  close: () => void;
	onConfirm: () => Promise<void>;
}) {
	const hasPackage = Boolean(instance.package_id || instance.source_id);
	const packageLabel = instance.source_id ? `GitHub 源：${instance.source_id}` : `常规包：${instance.package_id}`;
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const submit = async () => {
		if (submittingRef.current || !hasPackage) return;
    submittingRef.current = true;
    setSubmitting(true);
    try {
			await onConfirm();
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  };
  return (
    <div className="modal-wrap">
      <div
        className="modal reinstall-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="reinstall-title"
      >
        <span className="danger-icon">
          <RefreshCw />
        </span>
        <p className="eyebrow">FORCED REINSTALL</p>
		<h2 id="reinstall-title">重新安装实例插件包</h2>
		<p>{instance.name} 将完整部署当前选中的插件包并重新应用私有文件。共享游戏本体不会在此操作中更新。</p>
		<p>{hasPackage ? packageLabel : "该实例尚未选择插件包。"}</p>
        <div>
          <button disabled={submitting} onClick={close}>取消</button>
          <button
            className="danger"
            aria-label="确认重新安装"
			disabled={submitting || !hasPackage}
            aria-busy={submitting}
            onClick={() => void submit()}
          >
            {submitting ? <RefreshCw /> : null}
            {submitting ? "正在创建任务…" : "确认重新安装"}
          </button>
        </div>
      </div>
    </div>
  );
}
function JobStrip({ job }: { job: Job }) {
  const terminal = ["succeeded", "failed", "interrupted"].includes(job.Status);
  const description =
    job.Error ||
    (job.Status === "succeeded"
      ? "任务已成功完成"
      : job.Status === "failed"
        ? "任务执行失败"
        : job.Status === "interrupted"
          ? "任务已中断，请查看任务记录"
          : "后台任务持久化执行中");
  return (
    <section className="activity">
      <div>
        <p className="eyebrow">{terminal ? "JOB RESULT" : "LIVE JOB"}</p>
        <h2>{job.Stage || job.Status}</h2>
        <p>{description}</p>
      </div>
      <strong>{job.Percent || 0}%</strong>
      <div className="jobbar">
        <i style={{ width: `${job.Percent || 0}%` }} />
      </div>
    </section>
  );
}
function Metric({
  icon,
  label,
  value,
  unit,
  note,
  compact = false,
  noteTone,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  unit?: string;
  note: string;
  compact?: boolean;
  noteTone?: "success" | "warning";
}) {
  return (
    <article className={`metric${compact ? " compact" : ""}${noteTone ? ` note-${noteTone}` : ""}`}>
      <span>{icon}</span>
      <div>
        <small>{label}</small>
        <b>{value}{unit ? <i>{unit}</i> : null}</b>
        <em>{note}</em>
      </div>
    </article>
  );
}
function InstanceMetric({ label, value, note }: { label: string; value: string; note?: string }) {
  return <span className="performance-metric" title={note}><small>{label}</small><b>{value}</b>{note ? <em>{note}</em> : null}</span>;
}
const displayState = (instance: Instance) =>
  instance.observed_state ?? instance.actual_state;
const stateLabel = (s: string) =>
  ({
    running: "运行中",
    stopped: "已停止",
    uninstalled: "未安装",
    faulted: "故障",
    orphaned: "孤立",
    unknown: "状态未知",
  })[s] || s;
const formatBytes = (v: number) =>
  v > 1 << 30
    ? `${(v / (1 << 30)).toFixed(1)} GB`
    : `${(v / (1 << 20)).toFixed(1)} MB`;
