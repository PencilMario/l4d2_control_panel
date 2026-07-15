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
  Box,
  CalendarClock,
  ChevronRight,
  CircleStop,
  Database,
  Files,
  Gauge,
  ListTodo,
  Map,
  Play,
  Plus,
  RefreshCw,
  Server,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  TerminalSquare,
  Users,
  X,
} from "lucide-react";
import { sha256 } from "@noble/hashes/sha2.js";
import { api, normalizeInstance, type Job } from "../api/client";
import {
  InstanceConfigModal,
  type ConfigurableInstance,
  type InstanceConfigValues,
  type PackageVersion,
} from "./InstanceConfigModal";
import { PrivateFilesPage } from "./PrivateFilesPage";
import { useConsoleFollow } from "./useConsoleFollow";
import "../styles/app.css";
export type Instance = ConfigurableInstance & {
  players: number | null;
  cpu: number | null;
  memory: number | null;
  observed_state?: string;
  container_running?: boolean;
  observed_max_players?: number | null;
  current_map?: string;
};
type InstanceOverview = {
  actual_state: string;
  container_running: boolean;
  map: string;
  players: number | null;
  max_players: number | null;
  cpu_percent: number | null;
  memory_bytes: number | null;
  issues?: string[];
};
type Props = {
  initialInstances?: Instance[];
  initialPackages?: PackageVersion[];
  onAction?: (id: string, action: string) => void;
};
type Page = "overview" | "private" | "content" | "jobs" | "schedules" | "settings";
type HealthState = {
  status: "checking" | "online" | "error";
  message: string;
};
type Confirmation = {
  title: string;
  description: string;
  confirmLabel: string;
  confirm: () => void;
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
type GitHubSource = {
  id: string;
  name: string;
  repository: string;
  asset_pattern: string;
};

const errorMessage = (reason: unknown) =>
  reason instanceof Error ? reason.message : String(reason);

export function App({ initialInstances, initialPackages, onAction }: Props) {
  const injected = initialInstances !== undefined;
  const [auth, setAuth] = useState(injected ? "yes" : "checking");
  const [instances, setInstances] = useState<Instance[]>(
    initialInstances || [],
  );
  const [packages, setPackages] = useState<PackageVersion[]>(
    initialPackages || [],
  );
  const [pending, setPending] = useState<Instance | null>(null);
  const [page, setPage] = useState<Page>("overview");
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
  useEffect(() => () => {
    for (const controller of pollControllers.current.values()) controller.abort();
    for (const timer of pollTimers.current.values()) window.clearTimeout(timer);
    pollControllers.current.clear();
    pollTimers.current.clear();
  }, []);
  const loadInstances = useCallback(async () => {
    const base = (await api<any[]>("/api/instances")).map(normalizeInstance);
    const enriched = await Promise.all(
      base.map(async (instance) => {
        try {
          const overview = await api<InstanceOverview>(
            `/api/instances/${instance.id}/overview`,
          );
          return {
            ...instance,
            observed_state: overview.actual_state,
            container_running: overview.container_running,
            observed_max_players: overview.max_players,
            current_map: overview.map || undefined,
            cpu: overview.cpu_percent,
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
            observed_max_players: null,
            players: null,
            cpu: null,
            memory: null,
          };
        }
      }),
    );
    setInstances(enriched);
  }, []);
  const loadPackages = async () => {
    setPackages(await api<PackageVersion[]>("/api/packages"));
  };
  const loadHealth = async () => {
    try {
      await api("/api/health");
      setHealth({ status: "online", message: "Docker API 正常" });
    } catch (reason) {
      setHealth({ status: "error", message: errorMessage(reason) });
    }
  };
  useEffect(() => {
    if (injected) return;
    api("/api/session")
      .then(() => {
        setAuth("yes");
        void Promise.allSettled([
          loadInstances(),
          loadPackages(),
          loadHealth(),
        ]);
      })
      .catch(() => setAuth("no"));
  }, []);
  useEffect(() => {
    if (injected || auth !== "yes") return;
    const timer = window.setInterval(() => void loadInstances(), 5_000);
    return () => window.clearInterval(timer);
  }, [auth, injected, loadInstances]);
  const queue = async (path: string, body: any) => {
    const created = await api<Job>(path, {
      method: "POST",
      body: JSON.stringify(body),
    });
    setJob(created);
    void pollJob(created.ID).catch(() => undefined);
  };
  const queueAndWait = async (path: string, body: unknown) => {
    const created = await api<Job>(path, {
      method: "POST",
      body: JSON.stringify(body),
    });
    setJob(created);
    return pollJob(created.ID);
  };
  const action = async (id: string, kind: string) => {
    if (onAction) {
      onAction(id, kind);
      return;
    }
    try {
      await queue(`/api/instances/${id}/actions`, {
        action: kind,
        confirm: kind !== "start",
      });
    } catch (e) {
      setError(errorMessage(e));
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
          void Promise.allSettled([loadInstances(), loadPackages()]);
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
            loadHealth(),
          ]);
        }}
      />
    );
  const running = instances.filter(
    (x) => displayState(x) === "running",
  ).length;
  return (
    <div className="shell">
      <aside>
        <div className="brand">
          <span className="hazard">L4D</span>
          <div>
            <b>CONTROL</b>
            <small>NODE / LOCAL-01</small>
          </div>
        </div>
        <nav aria-label="主导航">
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
            active={page === "jobs"}
            onClick={() => setPage("jobs")}
            icon={<ListTodo />}
          >
            任务
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
          <div className={`node ${health.status}`}>
            <i></i>
            <span>
              {health.status === "online"
                ? "控制节点在线"
                : health.status === "error"
                  ? "控制节点异常"
                  : "控制节点检查中"}
              <small>{health.message}</small>
            </span>
          </div>
        </div>
      </aside>
      <main>
        <header>
          <div>
            <p className="eyebrow">OPERATIONS / {page.toUpperCase()}</p>
            <h1>
              {page === "overview"
                ? "服务器作战室"
                : page === "private"
                  ? "实例私有文件"
                : page === "content"
                  ? "内容仓库"
                  : page === "jobs"
                    ? "持久任务流水"
                    : page === "schedules"
                      ? "自动维护计划"
                      : "系统与凭据"}
            </h1>
            <p>管理游戏进程、内容部署与计划维护。</p>
          </div>
          <div className="operator">
            <span className="pulse"></span>
            <div>
              <b>管理员</b>
              <small>安全会话</small>
            </div>
            <ShieldCheck />
          </div>
        </header>
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
            running={running}
            setPending={setPending}
            action={action}
            setTerminal={setTerminal}
            setPlayers={setPlayersTarget}
            queue={queue}
            reload={loadInstances}
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
            reloadPackages={loadPackages}
            queue={queue}
          />
        )}
        {page === "jobs" && <JobsPage />}
        {page === "schedules" && <SchedulesPage instances={instances} />}{" "}
        {page === "settings" && <SettingsPage />}{" "}
        {job && <JobStrip job={job} />}
      </main>
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
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await api("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ password }),
      });
      onSuccess();
    } catch (err) {
      setError(errorMessage(err));
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
        <button>进入作战室</button>
      </form>
    </div>
  );
}
function Nav({
  active,
  onClick,
  icon,
  children,
}: {
  active: boolean;
  onClick: () => void;
  icon: ReactNode;
  children: ReactNode;
}) {
  return (
    <button className={active ? "active" : ""} onClick={onClick}>
      {icon}
      {children}
    </button>
  );
}
function Overview({
  instances,
  packages,
  running,
  setPending,
  action,
  setTerminal,
  setPlayers,
  queue,
  reload,
  acceptJob,
}: {
  instances: Instance[];
  packages: PackageVersion[];
  running: number;
  setPending: (v: Instance) => void;
  action: (id: string, a: string) => void;
  setTerminal: (v: Instance) => void;
  setPlayers: (v: Instance) => void;
  queue: (path: string, body: any) => Promise<void>;
  reload: () => Promise<void>;
  acceptJob: (job: Job) => void;
}) {
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Instance | null>(null);
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const [reinstalling, setReinstalling] = useState<Instance | null>(null);
  const packagesByID = new globalThis.Map(
    packages.map((item) => [item.id, item]),
  );
  const totalPlayers = instances.some((instance) => instance.players === null)
    ? "--"
    : String(instances.reduce((total, instance) => total + (instance.players ?? 0), 0));
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
      <section className="metrics">
        <Metric
          icon={<Activity />}
          label="运行实例"
          value={`${running} / ${instances.length}`}
          note="实时观测状态"
        />
        <Metric
          icon={<Users />}
          label="在线玩家"
          value={totalPlayers}
          note="A2S 查询"
        />
        <Metric
          icon={<Database />}
          label="持久实例"
          value={String(instances.length)}
          note="SQLite WAL"
        />
        <Metric
          icon={<RefreshCw />}
          label="控制通道"
          value="PTY"
          note="非 RCON"
        />
      </section>
      <section className="work">
        <div className="section-head">
          <div>
            <p className="eyebrow">INSTANCE GRID</p>
            <h2>游戏实例</h2>
          </div>
          <button className="create" onClick={() => setCreating(true)}>
            <Plus />
            创建实例
          </button>
        </div>
        <div className="grid">
          {instances.map((x) => {
            const selectedPackage = packagesByID.get(x.package_id);
            const packagePending =
              Boolean(x.package_id) && x.package_id !== x.applied_package_id;
            const state = displayState(x);
            const containerRunning = x.container_running ?? state === "running";
            const observedCapacity =
              x.observed_max_players === undefined
                ? x.max_players
                : x.observed_max_players;
            return (
              <article className={`card ${state}`} key={x.id}>
                <div className="card-top">
                  <span className="status">
                    <i></i>
                    {stateLabel(state)}
                  </span>
                  <button
                    className="icon-btn"
                    aria-label={`配置 ${x.name}`}
                    title="实例配置"
                    onClick={() => setEditing(x)}
                  >
                    <SlidersHorizontal />
                  </button>
                </div>
                <h3>{x.name}</h3>
                <p className="endpoint">
                  LOCAL-01 : {x.game_port}
                  {x.sourcetv_port ? ` · TV ${x.sourcetv_port}` : ""}
                  {x.plugin_ports.length
                    ? ` · 插件 ${x.plugin_ports.join(", ")}`
                    : ""}
                </p>
                <div className="package-line">
                  <span>
                    <small>插件包</small>
                    <b>
                      {selectedPackage
                        ? `${selectedPackage.filename} · ${selectedPackage.version}`
                        : "未选择"}
                    </b>
                  </span>
                  {packagePending ? <em>待应用</em> : null}
                </div>
                <div className="map">
                  <Map />
                  <span>
                    <small>{x.current_map ? "当前地图" : "启动地图"}</small>
                    <b>{x.current_map || x.start_map}</b>
                  </span>
                  <em>{x.game_mode.toUpperCase()}</em>
                </div>
                <div className="stats">
                  <span>
                    <small>玩家</small>
                    <b>
                      {x.players === null ? "--" : x.players}
                      {" / "}
                      {observedCapacity === null ? "--" : observedCapacity}
                    </b>
                  </span>
                  <span>
                    <small>CPU</small>
                    <b>{x.cpu === null ? "--" : `${x.cpu.toFixed(1)}%`}</b>
                  </span>
                  <span>
                    <small>内存</small>
                    <b>
                      {x.memory === null ? "--" : `${x.memory.toFixed(2)} GB`}
                    </b>
                  </span>
                </div>
                <div className="bar">
                  <i
                    style={{
                      width: state === "running" ? "100%" : "2%",
                    }}
                  />
                </div>
                <div className="actions">
                  {containerRunning ? (
                    <button
                      aria-label={`停止 ${x.name}`}
                      onClick={() => setPending(x)}
                    >
                      <CircleStop />
                      停止
                    </button>
                  ) : (
                    <button onClick={() => action(x.id, "start")}>
                      <Play />
                      启动
                    </button>
                  )}
                  <button onClick={() => setTerminal(x)}>
                    <TerminalSquare />
                    控制台
                  </button>
                  <button onClick={() => setPlayers(x)}>
                    <Users />
                    玩家
                  </button>
                  <button
                    onClick={() => setReinstalling(x)}
                  >
                    <RefreshCw />
                    更新
                  </button>
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
          onClose={() => setEditing(null)}
          onSubmit={(values) => saveConfig(values, editing)}
        />
      ) : null}
      {confirmation && (
        <ConfirmationDialog
          {...confirmation}
          close={() => setConfirmation(null)}
          onConfirm={() => {
            confirmation.confirm();
            setConfirmation(null);
          }}
        />
      )}
      {reinstalling && (
        <ReinstallDialog
          instance={reinstalling}
          close={() => setReinstalling(null)}
          onConfirm={(game, packageOption) => {
            void queue(`/api/instances/${reinstalling.id}/game-update`, {
              confirm: true,
              reinstall_game: game,
              reinstall_package: packageOption,
            });
            setReinstalling(null);
          }}
        />
      )}
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
  const [lines, setLines] = useState<string[]>([]);
  const [input, setInput] = useState("");
  const socket = useRef<WebSocket | null>(null);
  const consoleFollow = useConsoleFollow(lines);
  useEffect(() => {
    const protocol = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(
      `${protocol}://${location.host}/api/instances/${instance.id}/console`,
    );
    ws.binaryType = "arraybuffer";
    ws.onmessage = (e) => {
      const text =
        typeof e.data === "string" ? e.data : new TextDecoder().decode(e.data);
      setLines((old) => [...old, text].slice(-500));
    };
    socket.current = ws;
    return () => ws.close();
  }, [instance.id]);
  return (
    <div className="terminal-modal">
      <div className="terminal-head">
        <span>
          <i></i>
          {instance.name} / 原生控制台
        </span>
        <button onClick={close}>
          <X />
        </button>
      </div>
      <pre ref={consoleFollow.outputRef} onScroll={consoleFollow.onScroll}>
        {lines.join("")}
      </pre>
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
        <b>srcds&gt;</b>
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          autoFocus
        />
        <button>发送</button>
      </form>
    </div>
  );
}

type JobRecord = Job & {
  InstanceID: string;
  Type: string;
  Message: string;
  CreatedAt: string;
  UpdatedAt: string;
};

function JobsPage() {
  const [items, setItems] = useState<JobRecord[]>([]);
  const [jobsError, setJobsError] = useState("");
  useEffect(() => {
    let active = true;
    api<JobRecord[]>("/api/jobs")
      .then((jobs) => active && setItems(jobs))
      .catch((reason) => active && setJobsError(String(reason)));
    if (typeof EventSource === "undefined") {
      return () => {
        active = false;
      };
    }
    const events = new EventSource("/api/jobs/events");
    events.addEventListener("jobs", (event) => {
      if (!active) return;
      try {
        setItems(JSON.parse((event as MessageEvent<string>).data));
      } catch {
        setJobsError("任务事件数据无效");
      }
    });
    events.onerror = () => setJobsError("任务实时流已断开，正在由浏览器重连");
    return () => {
      active = false;
      events.close();
    };
  }, []);
  return (
    <section className="job-feed">
      <div className="section-head">
        <div>
          <p className="eyebrow">DURABLE OPERATIONS</p>
          <h2>最近任务</h2>
        </div>
        <span className="feed-live">SSE / LIVE</span>
      </div>
      {jobsError ? (
        <div className="error" role="alert">
          {jobsError}
        </div>
      ) : null}
      <div className="job-table" role="list">
        {items.map((item) => (
          <article className="job-row" key={item.ID} role="listitem">
            <div className="job-code">
              <span>{item.Type}</span>
              <small>{item.ID.slice(0, 8)}</small>
            </div>
            <div className="job-stage">
              <b>{item.Stage || "queued"}</b>
              <small>{item.Error || item.Message || "等待后台执行"}</small>
            </div>
            <div className="job-progress" aria-label={`进度 ${item.Percent}%`}>
              <i style={{ width: `${Math.max(0, item.Percent)}%` }} />
            </div>
            <span className={`job-state ${item.Status}`}>{item.Status}</span>
          </article>
        ))}
        {items.length === 0 ? <div className="empty">尚无后台任务</div> : null}
      </div>
    </section>
  );
}

const VPK_CHUNK_SIZE = 8 * 1024 * 1024;
const DEFAULT_PLUGIN_REPOSITORY =
  "PencilMario/L4D2-Not0721Here-CoopSvPlugins";
const DEFAULT_PLUGIN_ASSET_PATTERN =
  "^L4D2-Not0721Here-CoopSvPlugins-compiled\\.zip$";

function ContentPage({
  instances,
  packages,
  reloadPackages,
  queue,
}: {
  instances: Instance[];
  packages: PackageVersion[];
  reloadPackages: () => Promise<void>;
  queue: (path: string, body: any) => Promise<void>;
}) {
  const [vpks, setVpks] = useState<any[]>([]);
  const [selected, setSelected] = useState(instances[0]?.id || "");
  const [contentError, setContentError] = useState("");
  const [vpkUploadStatus, setVPKUploadStatus] = useState("");
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const [sources, setSources] = useState<GitHubSource[]>([]);
  const [sourceEditor, setSourceEditor] = useState<GitHubSource | null>(null);
  const loadVPK = () => api<any[]>("/api/content/vpk").then(setVpks);
  const loadSources = () => api<GitHubSource[]>("/api/github-sources").then((items) => setSources(Array.isArray(items) ? items : []));
  useEffect(() => {
    Promise.all([loadVPK(), reloadPackages(), loadSources()]).catch((reason) =>
      setContentError(errorMessage(reason)),
    );
  }, []);
  const uploadVPK = async (file: File) => {
    const hash = sha256.create();
    for (let offset = 0; offset < file.size; offset += VPK_CHUNK_SIZE) {
      const end = Math.min(offset + VPK_CHUNK_SIZE, file.size);
      const chunk = file.slice(offset, end);
      hash.update(new Uint8Array(await chunk.arrayBuffer()));
      setVPKUploadStatus(
        `正在计算 VPK 校验 · ${Math.round((end / file.size) * 100)}%`,
      );
    }
    const digest = hash.digest();
    const sha = [...digest]
      .map((x) => x.toString(16).padStart(2, "0"))
      .join("");
    const session = await api<any>("/api/content/vpk/uploads", {
      method: "POST",
      body: JSON.stringify({ name: file.name, size: file.size, sha256: sha }),
    });
    for (let offset = 0; offset < file.size; offset += VPK_CHUNK_SIZE) {
      const end = Math.min(offset + VPK_CHUNK_SIZE, file.size);
      const chunk = file.slice(offset, end);
      await api(
        `/api/content/vpk/uploads/${session.id ?? session.ID}?offset=${offset}`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/octet-stream" },
          body: chunk,
        },
      );
      setVPKUploadStatus(
        `正在上传 VPK · ${Math.round((end / file.size) * 100)}%`,
      );
    }
    await api(`/api/content/vpk/uploads/${session.id ?? session.ID}/complete`, {
      method: "POST",
      body: "{}",
    });
    await loadVPK();
    setVPKUploadStatus("VPK 上传完成 · 100%");
  };
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
  const runContentAction = (operation: () => Promise<unknown>) => {
    setContentError("");
    void operation().catch((reason) => setContentError(errorMessage(reason)));
  };
  return (
    <div className="content-layout">
      {contentError && (
        <div className="error" role="alert">
          {contentError}
        </div>
      )}
      {vpkUploadStatus && (
        <div className="operation-status" role="status">
          {vpkUploadStatus}
        </div>
      )}
      <label className="content-instance-selector">
        更新目标实例
        <select value={selected} onChange={(event) => setSelected(event.target.value)}>
          {instances.map((instance) => (
            <option key={instance.id} value={instance.id}>{instance.name}</option>
          ))}
        </select>
      </label>
      <Panel
        title="共享 VPK"
        action={
          <FileButton
            label="上传 VPK"
            accept=".vpk"
            onFile={(file) => runContentAction(() => uploadVPK(file))}
          />
        }
      >
        {vpks.map((x) => (
          <div className="data-row" key={x.name}>
            <div>
              <b>{x.name}</b>
              <small>
                {formatBytes(x.size)} · {String(x.hash).slice(0, 12)}
              </small>
            </div>
            <div className="inline-actions">
              <a
                aria-label={`下载 ${x.name}`}
                download
                href={`/api/content/vpk/${encodeURIComponent(x.name)}/download`}
              >
                下载
              </a>
              <button onClick={() => runContentAction(() => renameVPK(x.name))}>
                重命名
              </button>
              <button
                className="danger"
                onClick={() => runContentAction(() => deleteVPK(x.name))}
              >
                删除
              </button>
            </div>
          </div>
        ))}
        {vpks.length === 0 ? <div className="empty">暂无共享 VPK</div> : null}
      </Panel>
      <Panel
        title="插件包"
        action={
          <div className="inline-actions">
            <button onClick={() => setSourceEditor({ id: "", name: "", repository: "", asset_pattern: "" })}>添加 GitHub 源</button>
            <FileButton label="上传 ZIP" accept=".zip" onFile={(file) => runContentAction(() => uploadPackage(file))} />
          </div>
        }
      >
        {sourceEditor ? (
          <form className="release-source" onSubmit={(event) => {
            event.preventDefault();
            runContentAction(async () => {
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
            <div className="inline-actions"><button className="create">保存源</button><button type="button" onClick={() => setSourceEditor(null)}>取消</button></div>
          </form>
        ) : null}
        <div className="source-grid">
          {sources.map((source) => (
            <article className="source-card" key={source.id}>
              <div><b>{source.name}</b><small>{source.repository}</small><code>{source.asset_pattern}</code></div>
              <div className="inline-actions">
                <button aria-label={`检查更新 ${source.name}`} onClick={() => runContentAction(() => queue(`/api/github-sources/${source.id}/check`, {}))}>检查更新</button>
                <button onClick={() => setSourceEditor(source)}>编辑</button>
                <button className="danger" onClick={() => { if (window.confirm(`删除源 ${source.name}？已下载插件包会保留。`)) runContentAction(async () => { await api(`/api/github-sources/${source.id}`, { method: "DELETE" }); await loadSources(); }); }}>删除</button>
              </div>
            </article>
          ))}
        </div>
        {packages.map((x) => (
          <div className="data-row" key={x.id}>
            <div>
              <b>
                {x.filename} · {x.version}
              </b>
              <small>
                {formatBytes(x.size)} ·{" "}
                {x.hot_compatible ? "支持热更新" : "需要完整更新"}
              </small>
            </div>
            <div className="inline-actions">
              {x.hot_compatible && (
                <button
                  disabled={!selected}
                  onClick={() =>
                    runContentAction(() =>
                      queue(`/api/instances/${selected}/updates`, {
                        package_id: x.id,
                        mode: "hot",
                      }),
                    )
                  }
                >
                  热更新
                </button>
              )}
              <button
                disabled={!selected}
                onClick={() =>
                  setConfirmation({
                    title: `完整更新 ${x.filename}？`,
                    description:
                      "完整更新会停止服务器并替换插件包；失败时后台任务会保留诊断记录。",
                    confirmLabel: "确认完整更新",
                    confirm: () =>
                      runContentAction(() =>
                        queue(`/api/instances/${selected}/updates`, {
                          package_id: x.id,
                          mode: "full",
                          confirm: true,
                        }),
                      ),
                  })
                }
              >
                完整更新
              </button>
            </div>
          </div>
        ))}
        {!packages.length && <div className="empty">暂无插件包</div>}
      </Panel>
      {confirmation && (
        <ConfirmationDialog
          title={confirmation.title}
          description={confirmation.description}
          confirmLabel={confirmation.confirmLabel}
          close={() => setConfirmation(null)}
          onConfirm={() => {
            confirmation.confirm();
            setConfirmation(null);
          }}
        />
      )}
    </div>
  );
}
type ScheduledTask = {
  id: string;
  type: string;
  cron: string;
  timezone: string;
  enabled: boolean;
};

const normalizeScheduledTask = (value: any): ScheduledTask => ({
  id: value.id ?? value.ID,
  type: value.type ?? value.Type,
  cron: value.cron ?? value.Cron,
  timezone: value.timezone ?? value.Timezone,
  enabled: value.enabled ?? value.Enabled,
});

function SchedulesPage({ instances }: { instances: Instance[] }) {
  const [tasks, setTasks] = useState<ScheduledTask[]>([]);
  const [scheduleError, setScheduleError] = useState("");
  const [scheduleStatus, setScheduleStatus] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [taskType, setTaskType] = useState("game_update");
  const [sources, setSources] = useState<GitHubSource[]>([]);
  const releaseTask = ["release_check", "release_hot", "release_full"].includes(taskType);
  const needsInstance = !["release_check", "cleanup"].includes(taskType);
  const load = async () => {
    const [items, sourceItems] = await Promise.all([
      api<any[]>("/api/schedules"),
      api<GitHubSource[]>("/api/github-sources"),
    ]);
    setTasks(items.map(normalizeScheduledTask));
    setSources(Array.isArray(sourceItems) ? sourceItems : []);
  };
  useEffect(() => {
    load().catch((reason) => setScheduleError(errorMessage(reason)));
  }, []);
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    setScheduleError("");
    setScheduleStatus("正在保存计划…");
    setSubmitting(true);
    try {
      await api("/api/schedules", {
        method: "POST",
        body: JSON.stringify({
          instance_id: needsInstance ? data.get("instance") : "",
          type: taskType,
          cron: data.get("cron"),
          timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
          online_policy: needsInstance ? data.get("policy") : "force",
          enabled: true,
          payload: releaseTask
            ? JSON.stringify({ source_id: data.get("source") })
            : "{}",
        }),
      });
      await load();
      setScheduleStatus("计划已保存");
    } catch (reason) {
      setScheduleStatus("");
      setScheduleError(errorMessage(reason));
    } finally {
      setSubmitting(false);
    }
  };
  return (
    <div className="schedule-layout">
      <form className="control-form" onSubmit={submit}>
        <p className="eyebrow">NEW SCHEDULE</p>
        <h2>添加维护窗口</h2>
        <label>
          任务
          <select
            aria-label="任务"
            name="type"
            value={taskType}
            onChange={(event) => setTaskType(event.target.value)}
          >
            <option value="game_update">游戏更新</option>
            <option value="package_hot">插件热更新</option>
            <option value="package_full">插件完整更新</option>
            <option value="release_check">仅同步 GitHub 源</option>
            <option value="release_hot">GitHub Release 热更新</option>
            <option value="release_full">GitHub Release 完整更新</option>
            <option value="backup">备份</option>
            <option value="cleanup">清理</option>
          </select>
        </label>
        {needsInstance ? (
          <label>
            实例
            <select name="instance">
              {instances.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
            </select>
          </label>
        ) : null}
        {releaseTask ? (
          <label>
            GitHub 源
            <select aria-label="GitHub 源" name="source" required>
              {sources.map((source) => <option key={source.id} value={source.id}>{source.name}</option>)}
            </select>
          </label>
        ) : null}
        <label>
          Cron
          <input name="cron" defaultValue="0 4 * * *" />
        </label>
        {needsInstance ? (
          <label>
            在线玩家策略
            <select name="policy"><option value="skip">跳过</option><option value="wait">等待</option><option value="force">强制执行</option></select>
          </label>
        ) : null}
        {scheduleError && (
          <div className="error" role="alert">
            {scheduleError}
          </div>
        )}
        {scheduleStatus && (
          <div className="operation-status" role="status">
            {scheduleStatus}
          </div>
        )}
        <button className="create" disabled={submitting || (needsInstance && !instances.length) || (releaseTask && !sources.length)}>
          保存计划
        </button>
      </form>
      <Panel title="执行计划">
        {tasks.map((x) => (
          <Row
            key={x.id}
            name={x.type}
            meta={`${x.cron} · ${x.timezone} · ${x.enabled ? "启用" : "停用"}`}
          />
        ))}
        {!tasks.length && <div className="empty">暂无计划任务</div>}
      </Panel>
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
      confirm: () => {
        setPlayersError("");
        void queue(
          `/api/instances/${instance.id}/players/${player.user_id}/actions`,
          {
            action,
            ...(kick ? {} : { minutes: 0 }),
            confirm: true,
          },
        ).catch((reason) => setPlayersError(errorMessage(reason)));
      },
    });
  };
  return (
    <>
      <div className="modal-wrap">
        <div className="modal players-modal" role="dialog" aria-modal="true" aria-labelledby="players-title">
          <div className="section-head">
            <div>
              <p className="eyebrow">ONLINE PLAYERS</p>
              <h2 id="players-title">{instance.name}</h2>
            </div>
            <button aria-label="关闭玩家列表" onClick={close}>
              <X />
            </button>
          </div>
          {playersError && (
            <div className="error" role="alert">
              {playersError}
            </div>
          )}
          {snapshot?.match ? <MatchSummary match={snapshot.match} /> : null}
          {snapshot?.players?.length ? (
            <div className="player-operations-wrap">
              <table className="player-operations">
                <thead>
                  <tr><th>UserID</th><th>名称</th><th>UniqueID</th><th>已连接</th><th>Ping</th><th>Loss</th><th>分数</th><th>操作</th></tr>
                </thead>
                <tbody>
                  {snapshot.players.map((player) => (
                    <tr key={`${player.name}-${player.user_id}`}>
                      <td data-label="UserID"><b>{player.user_id || "未映射"}</b></td>
                      <td data-label="名称"><b>{player.name}</b></td>
                      <td data-label="UniqueID"><code>{player.unique_id || "--"}</code></td>
                      <td data-label="已连接">{player.connected || "--"}</td>
                      <td data-label="Ping">{player.ping === undefined ? "--" : `${player.ping} ms`}</td>
                      <td data-label="Loss">{player.loss === undefined ? "--" : `${player.loss}%`}</td>
                      <td data-label="分数">{player.score ?? "--"}</td>
                      <td data-label="操作">
                        {player.user_id > 0 ? (
                          <div className="inline-actions">
                            <button onClick={() => requestAction(player, "kick")}>踢出</button>
                            <button className="danger" onClick={() => requestAction(player, "ban")}>永久封禁</button>
                          </div>
                        ) : "--"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
          {snapshot && !snapshot.players?.length && (
            <div className="empty">当前没有在线玩家</div>
          )}
        </div>
      </div>
      {confirmation && (
        <ConfirmationDialog
          {...confirmation}
          close={() => setConfirmation(null)}
          onConfirm={() => {
            confirmation.confirm();
            setConfirmation(null);
          }}
        />
      )}
    </>
  );
}

function MatchSummary({ match }: { match: PlayerMatch }) {
  const secure = match.secure === null ? "--" : match.secure ? "安全" : "不安全";
  const value = (input: string) => input || "--";
  return (
    <section className="match-summary" aria-label="对局摘要">
      <div className="match-summary-grid">
        <div className="match-primary"><small>地图</small><b>{value(match.map)}</b></div>
        <div><small>真人玩家</small><b>{match.humans} / {match.max_players}</b></div>
        <div><small>主机 / 系统</small><b>{value(match.hostname)} · {value(match.os)}</b></div>
        <div><small>版本</small><b>{value(match.version)} · {secure}</b></div>
      </div>
      <div className="match-addresses">
        <span>内网 <code>{value(match.private_address)}</code></span>
        <span>公网 <code>{value(match.public_address)}</code></span>
      </div>
    </section>
  );
}

function SettingsPage() {
  const [steam, setSteam] = useState(false);
  const [github, setGithub] = useState(false);
  const [settingsError, setSettingsError] = useState("");
  useEffect(() => {
    api<any>("/api/settings/steam")
      .then((x) => setSteam(x.configured))
      .catch((reason) => setSettingsError(errorMessage(reason)));
    api<any>("/api/settings/github-token")
      .then((x) => setGithub(x.configured))
      .catch((reason) => setSettingsError(errorMessage(reason)));
  }, []);
  const saveSteam = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    setSettingsError("");
    try {
      await api("/api/settings/steam", {
        method: "PUT",
        body: JSON.stringify({
          username: data.get("username"),
          password: data.get("password"),
        }),
      });
      setSteam(true);
      e.currentTarget.reset();
    } catch (reason) {
      setSettingsError(errorMessage(reason));
    }
  };
  const saveGithub = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    setSettingsError("");
    try {
      await api("/api/settings/github-token", {
        method: "PUT",
        body: JSON.stringify({ token: data.get("token") }),
      });
      setGithub(true);
      e.currentTarget.reset();
    } catch (reason) {
      setSettingsError(errorMessage(reason));
    }
  };
  return (
    <div className="content-layout">
      {settingsError && (
        <div className="error" role="alert">
          {settingsError}
        </div>
      )}
      <form className="control-form" onSubmit={saveSteam}>
        <p className="eyebrow">STEAMCMD LICENSE</p>
        <h2>Steam 安装凭据</h2>
        <p>
          {steam
            ? "已加密配置；匿名首装仍可用"
            : "匿名首装已支持；仅许可账号需要配置凭据"}
        </p>
        <label>
          用户名
          <input name="username" autoComplete="username" required />
        </label>
        <label>
          密码
          <input
            name="password"
            type="password"
            autoComplete="current-password"
            required
          />
        </label>
        <button className="create">加密保存</button>
      </form>
      <form className="control-form" onSubmit={saveGithub}>
        <p className="eyebrow">GITHUB RELEASES</p>
        <h2>GitHub Token</h2>
        <p>{github ? "已加密配置" : "未配置；公开仓库仍可有限访问"}</p>
        <label>
          Token
          <input name="token" type="password" required />
        </label>
        <button className="create">加密保存</button>
      </form>
    </div>
  );
}

function FileButton({
  label,
  accept,
  onFile,
}: {
  label: string;
  accept: string;
  onFile: (f: File) => void;
}) {
  return (
    <label className="create file-button">
      <Plus />
      {label}
      <input
        type="file"
        accept={accept}
        onChange={(e) => e.target.files?.[0] && onFile(e.target.files[0])}
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
function Row({ name, meta }: { name: string; meta: string }) {
  return (
    <div className="data-row">
      <div>
        <b>{name}</b>
        <small>{meta}</small>
      </div>
      <ChevronRight />
    </div>
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
  onConfirm: () => void;
}) {
  const dialog = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
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
  }, [close]);
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
          <button onClick={close}>取消</button>
          <button
            autoFocus
            className="danger"
            aria-label={confirmLabel}
            onClick={onConfirm}
          >
            {confirmLabel}
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
  onConfirm: (game: boolean, packageOption: boolean) => void;
}) {
	const hasPackage = Boolean(instance.package_id);
  const [game, setGame] = useState(true);
  const [packageOption, setPackageOption] = useState(hasPackage);
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
        <h2 id="reinstall-title">重新安装实例组件</h2>
        <p>{instance.name} 将在重新安装期间停止，并在完成后恢复原有运行状态。</p>
        <fieldset className="reinstall-options">
          <label>
            <input
              aria-label="重新安装游戏本体"
              type="checkbox"
              checked={game}
              onChange={(event) => setGame(event.target.checked)}
            />
            <span><b>重新安装游戏本体</b><small>强制运行 SteamCMD 校验 App 222860</small></span>
          </label>
          <label>
            <input
              aria-label="重新安装实例插件包"
              type="checkbox"
              checked={packageOption}
              disabled={!hasPackage}
              onChange={(event) => setPackageOption(event.target.checked)}
            />
            <span><b>重新安装实例插件包</b><small>{hasPackage ? "完整部署当前选中的插件包" : "该实例尚未选择插件包"}</small></span>
          </label>
        </fieldset>
        <div>
          <button onClick={close}>取消</button>
          <button
            className="danger"
            aria-label="确认重新安装"
            disabled={!game && !packageOption}
            onClick={() => onConfirm(game, packageOption)}
          >
            确认重新安装
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
  note,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  note: string;
}) {
  return (
    <article className="metric">
      <span>{icon}</span>
      <div>
        <small>{label}</small>
        <b>{value}</b>
        <em>{note}</em>
      </div>
    </article>
  );
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
