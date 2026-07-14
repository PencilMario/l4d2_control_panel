import {
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
  Gauge,
  Map,
  Play,
  Plus,
  RefreshCw,
  Server,
  Settings,
  ShieldCheck,
  TerminalSquare,
  Users,
  X,
} from "lucide-react";
import { api, normalizeInstance, type Job } from "../api/client";
import "../styles/app.css";
export type Instance = {
  id: string;
  name: string;
  actual_state: string;
  game_port: number;
  start_map: string;
  game_mode: string;
  max_players: number;
  players: number;
  cpu: number;
  memory: number;
};
type Props = {
  initialInstances?: Instance[];
  onAction?: (id: string, action: string) => void;
};
type Page = "overview" | "content" | "schedules" | "settings";

export function App({ initialInstances, onAction }: Props) {
  const injected = initialInstances !== undefined;
  const [auth, setAuth] = useState(injected ? "yes" : "checking");
  const [instances, setInstances] = useState<Instance[]>(
    initialInstances || [],
  );
  const [pending, setPending] = useState<Instance | null>(null);
  const [page, setPage] = useState<Page>("overview");
  const [terminal, setTerminal] = useState<Instance | null>(null);
  const [playersTarget, setPlayersTarget] = useState<Instance | null>(null);
  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState("");
  const loadInstances = async () => {
    const base = (await api<any[]>("/api/instances")).map(normalizeInstance);
    const enriched = await Promise.all(
      base.map(async (instance) => {
        if (instance.actual_state !== "running") return instance;
        const [resources, players] = await Promise.all([
          api<any>(`/api/instances/${instance.id}/resources`).catch(() => null),
          api<any>(`/api/instances/${instance.id}/players`).catch(() => null),
        ]);
        return {
          ...instance,
          cpu: resources?.cpu_percent ?? 0,
          memory: resources?.memory_bytes
            ? resources.memory_bytes / (1 << 30)
            : 0,
          players: players?.players?.length ?? 0,
        };
      }),
    );
    setInstances(enriched);
  };
  useEffect(() => {
    if (injected) return;
    api("/api/session")
      .then(() => {
        setAuth("yes");
        return loadInstances();
      })
      .catch(() => setAuth("no"));
  }, []);
  const queue = async (path: string, body: any) => {
    const created = await api<Job>(path, {
      method: "POST",
      body: JSON.stringify(body),
    });
    setJob(created);
    pollJob(created.ID);
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
      setError(String(e));
    }
  };
  const pollJob = (id: string) => {
    const timer = setInterval(async () => {
      try {
        const next = await api<Job>(`/api/jobs/${id}`);
        setJob(next);
        if (["succeeded", "failed"].includes(next.Status)) {
          clearInterval(timer);
          loadInstances();
        }
      } catch {
        clearInterval(timer);
      }
    }, 800);
  };
  if (auth === "checking")
    return <div className="splash">正在连接控制节点…</div>;
  if (auth === "no")
    return (
      <Login
        onSuccess={() => {
          setAuth("yes");
          loadInstances();
        }}
      />
    );
  const running = instances.filter((x) => x.actual_state === "running").length;
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
            active={page === "content"}
            onClick={() => setPage("content")}
            icon={<Box />}
          >
            内容仓库
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
          <div className="node">
            <i></i>
            <span>
              控制节点在线<small>Docker API 正常</small>
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
                : page === "content"
                  ? "内容仓库"
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
            running={running}
            setPending={setPending}
            action={action}
            setTerminal={setTerminal}
            setPlayers={setPlayersTarget}
            queue={queue}
            reload={loadInstances}
          />
        )}{" "}
        {page === "content" && (
          <ContentPage instances={instances} queue={queue} />
        )}
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
      setError(String(err));
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
  running,
  setPending,
  action,
  setTerminal,
  setPlayers,
  queue,
  reload,
}: {
  instances: Instance[];
  running: number;
  setPending: (v: Instance) => void;
  action: (id: string, a: string) => void;
  setTerminal: (v: Instance) => void;
  setPlayers: (v: Instance) => void;
  queue: (path: string, body: any) => Promise<void>;
  reload: () => void;
}) {
  const [creating, setCreating] = useState(false);
  return (
    <>
      <section className="metrics">
        <Metric
          icon={<Activity />}
          label="运行实例"
          value={`${running} / ${instances.length}`}
          note="实时期望状态"
        />
        <Metric
          icon={<Users />}
          label="在线玩家"
          value={String(instances.reduce((n, x) => n + x.players, 0))}
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
          {instances.map((x) => (
            <article className={`card ${x.actual_state}`} key={x.id}>
              <div className="card-top">
                <span className="status">
                  <i></i>
                  {stateLabel(x.actual_state)}
                </span>
                <button className="icon-btn">
                  <ChevronRight />
                </button>
              </div>
              <h3>{x.name}</h3>
              <p className="endpoint">LOCAL-01 : {x.game_port}</p>
              <div className="map">
                <Map />
                <span>
                  <small>启动地图</small>
                  <b>{x.start_map}</b>
                </span>
                <em>{x.game_mode.toUpperCase()}</em>
              </div>
              <div className="stats">
                <span>
                  <small>玩家</small>
                  <b>
                    {x.players} / {x.max_players}
                  </b>
                </span>
                <span>
                  <small>CPU</small>
                  <b>{x.cpu.toFixed(1)}%</b>
                </span>
                <span>
                  <small>内存</small>
                  <b>{x.memory.toFixed(2)} GB</b>
                </span>
              </div>
              <div className="bar">
                <i
                  style={{
                    width: x.actual_state === "running" ? "100%" : "2%",
                  }}
                />
              </div>
              <div className="actions">
                {x.actual_state === "running" ? (
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
                  onClick={() =>
                    queue(`/api/instances/${x.id}/game-update`, {
                      confirm: true,
                    })
                  }
                >
                  <RefreshCw />
                  更新
                </button>
              </div>
            </article>
          ))}
        </div>
        {instances.length === 0 && (
          <div className="empty">尚无实例。创建第一个 Host 网络服务器。</div>
        )}
      </section>
      {creating && (
        <CreateInstance
          close={() => setCreating(false)}
          done={() => {
            setCreating(false);
            reload();
          }}
        />
      )}
    </>
  );
}
function CreateInstance({
  close,
  done,
}: {
  close: () => void;
  done: () => void;
}) {
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    await api("/api/instances", {
      method: "POST",
      body: JSON.stringify({
        name: data.get("name"),
        game_port: Number(data.get("port")),
        start_map: data.get("map"),
        game_mode: data.get("mode"),
        tickrate: 100,
        max_players: Number(data.get("players")),
      }),
    });
    done();
  };
  return (
    <div className="modal-wrap">
      <form className="modal form" onSubmit={submit}>
        <p className="eyebrow">NEW INSTANCE</p>
        <h2>创建游戏实例</h2>
        <label>
          名称
          <input name="name" required />
        </label>
        <label>
          游戏端口
          <input name="port" type="number" defaultValue="27015" />
        </label>
        <label>
          启动地图
          <input name="map" defaultValue="c2m1_highway" />
        </label>
        <label>
          模式
          <select name="mode">
            <option value="coop">合作</option>
            <option value="realism">写实</option>
          </select>
        </label>
        <label>
          最大玩家
          <input name="players" type="number" defaultValue="8" />
        </label>
        <div>
          <button type="button" onClick={close}>
            取消
          </button>
          <button className="create">创建</button>
        </div>
      </form>
    </div>
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
      <pre>{lines.join("")}</pre>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (input) {
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
function ContentPage({
  instances,
  queue,
}: {
  instances: Instance[];
  queue: (path: string, body: any) => Promise<void>;
}) {
  const [vpks, setVpks] = useState<any[]>([]);
  const [packages, setPackages] = useState<any[]>([]);
  const [selected, setSelected] = useState(instances[0]?.id || "");
  const [privatePath, setPrivatePath] = useState("cfg/server.cfg");
  const [privateText, setPrivateText] = useState("");
  const [contentError, setContentError] = useState("");
  const load = () =>
    Promise.all([
      api<any[]>("/api/content/vpk").then(setVpks),
      api<any[]>("/api/packages").then(setPackages),
    ]);
  useEffect(() => {
    load().catch(() => {});
  }, []);
  const uploadVPK = async (file: File) => {
    const hash = await crypto.subtle.digest(
      "SHA-256",
      await file.arrayBuffer(),
    );
    const sha = [...new Uint8Array(hash)]
      .map((x) => x.toString(16).padStart(2, "0"))
      .join("");
    const session = await api<any>("/api/content/vpk/uploads", {
      method: "POST",
      body: JSON.stringify({ name: file.name, size: file.size, sha256: sha }),
    });
    await api(`/api/content/vpk/uploads/${session.id ?? session.ID}?offset=0`, {
      method: "PATCH",
      headers: { "Content-Type": "application/octet-stream" },
      body: file,
    });
    await api(`/api/content/vpk/uploads/${session.id ?? session.ID}/complete`, {
      method: "POST",
      body: "{}",
    });
    await load();
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
    await load();
  };
  const runContentAction = (operation: () => Promise<unknown>) => {
    setContentError("");
    void operation().catch((reason) => setContentError(String(reason)));
  };
  return (
    <div className="content-layout">
      {contentError && (
        <div className="error" role="alert">
          {contentError}
        </div>
      )}
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
          <Row
            key={x.name}
            name={x.name}
            meta={`${formatBytes(x.size)} · ${String(x.hash).slice(0, 12)}`}
          />
        ))}
        {!vpks.length && <div className="empty">暂无共享 VPK</div>}
      </Panel>
      <Panel
        title="插件包"
        action={
          <FileButton
            label="上传 ZIP"
            accept=".zip"
            onFile={(file) => runContentAction(() => uploadPackage(file))}
          />
        }
      >
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
                  runContentAction(() =>
                    queue(`/api/instances/${selected}/updates`, {
                      package_id: x.id,
                      mode: "full",
                      confirm: true,
                    }),
                  )
                }
              >
                完整更新
              </button>
            </div>
          </div>
        ))}
        {!packages.length && <div className="empty">暂无插件包</div>}
      </Panel>
      <form
        className="control-form"
        onSubmit={(e) => {
          e.preventDefault();
          runContentAction(async () => {
            await api(`/api/instances/${selected}/private/${privatePath}`, {
              method: "PUT",
              headers: { "Content-Type": "text/plain; charset=utf-8" },
              body: privateText,
            });
            await queue(`/api/instances/${selected}/private/apply`, {});
          });
        }}
      >
        <p className="eyebrow">PRIVATE OVERLAY</p>
        <h2>实例私有覆盖</h2>
        <label>
          目标实例
          <select
            value={selected}
            onChange={(e) => setSelected(e.target.value)}
          >
            {instances.map((x) => (
              <option key={x.id} value={x.id}>
                {x.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          相对路径
          <input
            value={privatePath}
            onChange={(e) => setPrivatePath(e.target.value)}
          />
        </label>
        <label>
          文本内容
          <textarea
            rows={10}
            value={privateText}
            onChange={(e) => setPrivateText(e.target.value)}
          />
        </label>
        <button className="create" disabled={!selected}>
          保存并立即应用
        </button>
      </form>
    </div>
  );
}
function SchedulesPage({ instances }: { instances: Instance[] }) {
  const [tasks, setTasks] = useState<any[]>([]);
  const load = () => api<any[]>("/api/schedules").then(setTasks);
  useEffect(() => {
    load().catch(() => {});
  }, []);
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    await api("/api/schedules", {
      method: "POST",
      body: JSON.stringify({
        instance_id: data.get("instance"),
        type: data.get("type"),
        cron: data.get("cron"),
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        online_policy: data.get("policy"),
        enabled: true,
        payload: "{}",
      }),
    });
    load();
  };
  return (
    <div className="schedule-layout">
      <form className="control-form" onSubmit={submit}>
        <p className="eyebrow">NEW SCHEDULE</p>
        <h2>添加维护窗口</h2>
        <label>
          实例
          <select name="instance">
            {instances.map((x) => (
              <option value={x.id}>{x.name}</option>
            ))}
          </select>
        </label>
        <label>
          任务
          <select name="type">
            <option value="game_update">游戏更新</option>
            <option value="package_hot">插件热更新</option>
            <option value="backup">备份</option>
            <option value="cleanup">清理</option>
          </select>
        </label>
        <label>
          Cron
          <input name="cron" defaultValue="0 4 * * *" />
        </label>
        <label>
          在线玩家策略
          <select name="policy">
            <option value="skip">跳过</option>
            <option value="wait">等待</option>
            <option value="force">强制执行</option>
          </select>
        </label>
        <button className="create">保存计划</button>
      </form>
      <Panel title="执行计划">
        {tasks.map((x) => (
          <Row
            key={x.ID}
            name={x.Type}
            meta={`${x.Cron} · ${x.Timezone} · ${x.Enabled ? "启用" : "停用"}`}
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
  const [snapshot, setSnapshot] = useState<any>(null);
  useEffect(() => {
    api(`/api/instances/${instance.id}/players`)
      .then(setSnapshot)
      .catch(() => setSnapshot({ players: [] }));
  }, [instance.id]);
  return (
    <div className="modal-wrap">
      <div className="modal players-modal">
        <div className="section-head">
          <div>
            <p className="eyebrow">ONLINE PLAYERS</p>
            <h2>{instance.name}</h2>
          </div>
          <button onClick={close}>
            <X />
          </button>
        </div>
        {snapshot?.players?.map((player: any) => (
          <div className="data-row" key={`${player.name}-${player.user_id}`}>
            <div>
              <b>{player.name}</b>
              <small>
                UserID {player.user_id || "未映射"} · 分数 {player.score}
              </small>
            </div>
            {player.user_id > 0 && (
              <div className="inline-actions">
                <button
                  onClick={() =>
                    queue(
                      `/api/instances/${instance.id}/players/${player.user_id}/actions`,
                      { action: "kick", confirm: true },
                    )
                  }
                >
                  踢出
                </button>
                <button
                  className="danger"
                  onClick={() =>
                    queue(
                      `/api/instances/${instance.id}/players/${player.user_id}/actions`,
                      { action: "ban", minutes: 0, confirm: true },
                    )
                  }
                >
                  永久封禁
                </button>
              </div>
            )}
          </div>
        ))}
        {snapshot && !snapshot.players?.length && (
          <div className="empty">当前没有在线玩家</div>
        )}
      </div>
    </div>
  );
}

function SettingsPage() {
  const [steam, setSteam] = useState(false);
  const [github, setGithub] = useState(false);
  useEffect(() => {
    api<any>("/api/settings/steam")
      .then((x) => setSteam(x.configured))
      .catch(() => {});
    api<any>("/api/settings/github-token")
      .then((x) => setGithub(x.configured))
      .catch(() => {});
  }, []);
  const saveSteam = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    await api("/api/settings/steam", {
      method: "PUT",
      body: JSON.stringify({
        username: data.get("username"),
        password: data.get("password"),
      }),
    });
    setSteam(true);
    e.currentTarget.reset();
  };
  const saveGithub = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    await api("/api/settings/github-token", {
      method: "PUT",
      body: JSON.stringify({ token: data.get("token") }),
    });
    setGithub(true);
    e.currentTarget.reset();
  };
  return (
    <div className="content-layout">
      <form className="control-form" onSubmit={saveSteam}>
        <p className="eyebrow">STEAMCMD LICENSE</p>
        <h2>Steam 安装凭据</h2>
        <p>
          {steam ? "已加密配置" : "未配置；匿名账号可能无法安装 App 222860"}
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
function JobStrip({ job }: { job: Job }) {
  return (
    <section className="activity">
      <div>
        <p className="eyebrow">LIVE JOB</p>
        <h2>{job.Stage || job.Status}</h2>
        <p>{job.Error || "后台任务持久化执行中"}</p>
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
const stateLabel = (s: string) =>
  ({
    running: "运行中",
    stopped: "已停止",
    uninstalled: "未安装",
    faulted: "故障",
    orphaned: "孤立",
  })[s] || s;
const formatBytes = (v: number) =>
  v > 1 << 30
    ? `${(v / (1 << 30)).toFixed(1)} GB`
    : `${(v / (1 << 20)).toFixed(1)} MB`;
