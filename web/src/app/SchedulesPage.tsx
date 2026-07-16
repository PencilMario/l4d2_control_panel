import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import { BookOpen, Pencil, Trash2, X } from "lucide-react";
import { api } from "../api/client";
import type { PackageVersion } from "./InstanceConfigModal";

type ScheduleTaskType =
  | "game_update"
  | "package_hot"
  | "package_full"
  | "release_check"
  | "release_hot"
  | "release_full"
  | "backup"
  | "cleanup";

type OnlinePolicy = "skip" | "wait" | "force";

export type ScheduledTask = {
  id: string;
  instance_id: string;
  type: ScheduleTaskType;
  cron: string;
  timezone: string;
  online_policy: OnlinePolicy;
  payload: string;
  enabled: boolean;
  last_run: string;
  next_run: string;
};

type ScheduleInstance = {
  id: string;
  name: string;
};

type GitHubSource = {
  id: string;
  name: string;
  repository: string;
  asset_pattern: string;
};

type TaskTypeDefinition = {
  label: string;
  needsInstance: boolean;
  usesPlayerPolicy: boolean;
  target: string;
  steps: string;
  interruption: string;
  parameters: string;
  caution: string;
};

const TASK_TYPE_ORDER: ScheduleTaskType[] = [
  "game_update",
  "package_hot",
  "package_full",
  "release_check",
  "release_hot",
  "release_full",
  "backup",
  "cleanup",
];

const TASK_TYPES: Record<ScheduleTaskType, TaskTypeDefinition> = {
  game_update: {
    label: "游戏更新",
    needsInstance: true,
    usesPlayerPolicy: true,
    target: "一个游戏实例。",
    steps:
      "先应用在线玩家策略；允许执行后读取实例状态，必要时停止实例，通过维护流程运行 SteamCMD 更新或校验游戏文件，重新应用私有文件覆盖，再按原期望状态决定是否启动实例。",
    interruption:
      "运行中或启动中的实例会停止。原本期望运行的实例在更新和健康检查成功后恢复运行；原本停止的实例保持停止。",
    parameters: "不需要额外任务参数。",
    caution:
      "耗时取决于 Steam 下载。失败会记录 scheduled_game_update Job；实例可能进入故障状态，需要检查任务日志。",
  },
  package_hot: {
    label: "插件热更新",
    needsInstance: true,
    usesPlayerPolicy: true,
    target: "一个游戏实例和内容仓库中的一个插件包。",
    steps:
      "先应用在线玩家策略，再读取指定插件包；仅部署热更新允许的配置、脚本和插件内容，重新应用私有覆盖，最后记录该包为已应用版本。",
    interruption: "不主动停止或重启实例。",
    parameters: "需要选择通过热更新兼容检查的插件包。",
    caution:
      "不适合需要替换游戏二进制或必须重启才能生效的内容。不兼容时任务失败，不会自动降级为完整更新。",
  },
  package_full: {
    label: "插件完整更新",
    needsInstance: true,
    usesPlayerPolicy: true,
    target: "一个游戏实例和内容仓库中的一个插件包。",
    steps:
      "先应用在线玩家策略；活动实例先停止，事务化部署插件包并重新应用私有覆盖，再按更新前状态启动并通过健康检查，成功后记录已应用版本。",
    interruption:
      "活动实例会停止并可能断开玩家；原本停止的实例保持停止。",
    parameters: "需要选择一个仍然存在的插件包。",
    caution:
      "部署、启动或健康检查失败时会执行回滚，并尝试恢复更新前内容和运行状态。",
  },
  release_check: {
    label: "仅同步 GitHub 源",
    needsInstance: false,
    usesPlayerPolicy: false,
    target: "一个 GitHub Release 源；这是全局任务，不绑定游戏实例。",
    steps:
      "按源的仓库和资源匹配规则检查最新 Release；需要时使用已配置的 GitHub Token；发现新资源后下载、校验并登记为内容仓库中的插件包。",
    interruption: "不停止、不更新任何游戏实例，也不自动应用下载的包。",
    parameters: "需要选择一个仍然存在的 GitHub 源。",
    caution: "不检查在线玩家；没有新版本时正常结束。",
  },
  release_hot: {
    label: "GitHub Release 热更新",
    needsInstance: true,
    usesPlayerPolicy: true,
    target: "一个游戏实例和一个 GitHub Release 源。",
    steps:
      "先检查并下载最新匹配 Release；只有发现新版本时才应用在线玩家策略，然后以热更新方式部署新包并记录已应用版本。",
    interruption: "不主动停止或重启实例。",
    parameters: "需要选择一个仍然存在的 GitHub 源。",
    caution:
      "Release 下载发生在玩家检查之前。没有新版本时不部署；新包不兼容热更新时任务失败，不会自动改为完整更新。",
  },
  release_full: {
    label: "GitHub Release 完整更新",
    needsInstance: true,
    usesPlayerPolicy: true,
    target: "一个游戏实例和一个 GitHub Release 源。",
    steps:
      "先检查并下载最新匹配 Release；只有发现新版本时才应用在线玩家策略，然后停止活动实例、事务化部署、重新应用私有覆盖、按原状态启动并检查健康。",
    interruption: "活动实例会停止并可能断开玩家。",
    parameters: "需要选择一个仍然存在的 GitHub 源。",
    caution:
      "Release 下载发生在玩家检查之前。没有新版本时正常结束；部署或重启失败时执行回滚。",
  },
  backup: {
    label: "备份",
    needsInstance: true,
    usesPlayerPolicy: true,
    target: "一个游戏实例。",
    steps:
      "先应用在线玩家策略，然后把当前私有文件工作区和已应用插件清单写入实例 backups 目录下的时间戳 .tar.gz；归档完成并同步后才原子发布。",
    interruption: "不停止或重启实例。",
    parameters: "不需要额外任务参数。",
    caution:
      "不包含完整游戏安装、容器镜像、Panel 数据库或历史备份。遇到符号链接或写盘失败时不会发布不完整归档。",
  },
  cleanup: {
    label: "清理",
    needsInstance: false,
    usesPlayerPolicy: false,
    target: "Panel 管理的数据根目录；这是全局任务。",
    steps:
      "删除保留期之前的实例 backups 文件，以及包上传目录中遗留的 .part 和 .upload 文件；当前私有工作区、内容包和数据库不在清理范围内。",
    interruption: "不停止或重启实例，也不检查在线玩家。",
    parameters: "需要设置保留天数；缺失或小于 1 时执行器按 30 天处理。",
    caution: "按文件修改时间判断且不可撤销；删除的备份不会进入回收站。",
  },
};

const POLICY_LABELS: Record<OnlinePolicy, string> = {
  skip: "有玩家时跳过",
  wait: "等待玩家离开",
  force: "强制执行",
};

const errorMessage = (reason: unknown) =>
  reason instanceof Error ? reason.message : String(reason);

const isTaskType = (value: unknown): value is ScheduleTaskType =>
  typeof value === "string" && TASK_TYPE_ORDER.includes(value as ScheduleTaskType);

const isOnlinePolicy = (value: unknown): value is OnlinePolicy =>
  value === "skip" || value === "wait" || value === "force";

export const normalizeScheduledTask = (value: any): ScheduledTask => {
  const typeValue = value.type ?? value.Type;
  const policyValue = value.online_policy ?? value.OnlinePolicy;
  return {
    id: String(value.id ?? value.ID ?? ""),
    instance_id: String(value.instance_id ?? value.InstanceID ?? ""),
    type: isTaskType(typeValue) ? typeValue : "game_update",
    cron: String(value.cron ?? value.Cron ?? ""),
    timezone: String(value.timezone ?? value.Timezone ?? "UTC"),
    online_policy: isOnlinePolicy(policyValue) ? policyValue : "skip",
    payload: String(value.payload ?? value.Payload ?? "{}"),
    enabled: Boolean(value.enabled ?? value.Enabled),
    last_run: String(value.last_run ?? value.LastRun ?? ""),
    next_run: String(value.next_run ?? value.NextRun ?? ""),
  };
};

function parsePayload(task: Pick<ScheduledTask, "payload">) {
  try {
    const value = JSON.parse(task.payload || "{}");
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

function isUnsetTime(value: string) {
  return !value || value.startsWith("0001-01-01");
}

function formatTime(value: string, fallback: string) {
  if (isUnsetTime(value)) return fallback;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return fallback;
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function DialogFrame({
  title,
  label,
  close,
  closeButtonLabel,
  className = "",
  children,
}: {
  title: string;
  label: string;
  close: () => void;
  closeButtonLabel?: string;
  className?: string;
  children: ReactNode;
}) {
  const dialog = useRef<HTMLDivElement | null>(null);
  const titleID = useId();
  useEffect(() => {
    const closeOrContainFocus = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        close();
        return;
      }
      if (event.key !== "Tab" || !dialog.current) return;
      const focusable = Array.from(
        dialog.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), input:not([disabled]), select:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
        ),
      );
      if (!focusable.length) return;
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
    window.addEventListener("keydown", closeOrContainFocus);
    return () => window.removeEventListener("keydown", closeOrContainFocus);
  }, [close]);
  return (
    <div className="modal-wrap">
      <div
        ref={dialog}
        className={`modal ${className}`.trim()}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        aria-label={label}
      >
        <div className="schedule-dialog-head">
          <div>
            <p className="eyebrow">SCHEDULE CONTROL</p>
            <h2 id={titleID}>{title}</h2>
          </div>
          <button
            autoFocus
            className="icon-btn"
            aria-label={closeButtonLabel || `关闭${label}`}
            title="关闭"
            onClick={close}
          >
            <X />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

function TaskHelpDialog({ close }: { close: () => void }) {
  return (
    <DialogFrame
      title="计划任务类型说明"
      label="计划任务类型说明"
      closeButtonLabel="关闭任务说明"
      className="schedule-help-dialog"
      close={close}
    >
      <p className="schedule-help-intro">
        下列说明按实际执行器顺序描述。删除计划只阻止以后触发，不会取消已经排队或正在执行的任务。
      </p>
      <div className="schedule-help-list">
        {TASK_TYPE_ORDER.map((type) => {
          const item = TASK_TYPES[type];
          return (
            <section key={type} className="schedule-help-item">
              <div className="schedule-help-title">
                <h3>{item.label}</h3>
                <code>{type}</code>
              </div>
              <dl>
                <div><dt>执行目标</dt><dd>{item.target}</dd></div>
                <div><dt>实际步骤</dt><dd>{item.steps}</dd></div>
                <div><dt>实例中断</dt><dd>{item.interruption}</dd></div>
                <div><dt>任务参数</dt><dd>{item.parameters}</dd></div>
                <div><dt>注意事项</dt><dd>{item.caution}</dd></div>
              </dl>
              <p className="schedule-policy-note">
                在线玩家策略：{item.usesPlayerPolicy ? "在实际维护前按所选策略处理。" : "不适用，不检查玩家。"}
              </p>
            </section>
          );
        })}
      </div>
    </DialogFrame>
  );
}

function payloadSummary(
  task: ScheduledTask,
  packages: PackageVersion[],
  sources: GitHubSource[],
) {
  const payload = parsePayload(task) as Record<string, unknown>;
  if (task.type === "package_hot" || task.type === "package_full") {
    const id = String(payload.package_id ?? "");
    const item = packages.find((candidate) => candidate.id === id);
    return item ? `${item.filename} · ${item.version}` : `插件包引用已失效${id ? ` · ${id}` : ""}`;
  }
  if (task.type.startsWith("release_")) {
    const id = String(payload.source_id ?? "");
    const item = sources.find((candidate) => candidate.id === id);
    return item ? `${item.name} · ${item.repository}` : `GitHub 源引用已失效${id ? ` · ${id}` : ""}`;
  }
  if (task.type === "cleanup") {
    const days = Number(payload.retention_days);
    return `保留 ${Number.isFinite(days) && days >= 1 ? days : 30} 天`;
  }
  return "无额外参数";
}

export function SchedulesPage({
  instances,
  packages,
}: {
  instances: ScheduleInstance[];
  packages: PackageVersion[];
}) {
  const [tasks, setTasks] = useState<ScheduledTask[]>([]);
  const [sources, setSources] = useState<GitHubSource[]>([]);
  const [scheduleError, setScheduleError] = useState("");
  const [scheduleStatus, setScheduleStatus] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [taskType, setTaskType] = useState<ScheduleTaskType>("game_update");
  const [instanceID, setInstanceID] = useState(instances[0]?.id || "");
  const [sourceID, setSourceID] = useState("");
  const [packageID, setPackageID] = useState("");
  const [retentionDays, setRetentionDays] = useState(30);
  const [cron, setCron] = useState("0 4 * * *");
  const [policy, setPolicy] = useState<OnlinePolicy>("skip");
  const [enabled, setEnabled] = useState(true);
  const [editing, setEditing] = useState<ScheduledTask | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ScheduledTask | null>(null);
  const [showHelp, setShowHelp] = useState(false);

  const definition = TASK_TYPES[taskType];
  const releaseTask = taskType.startsWith("release_");
  const packageTask = taskType === "package_hot" || taskType === "package_full";
  const availablePackages = useMemo(
    () =>
      taskType === "package_hot"
        ? packages.filter((item) => item.hot_compatible)
        : packages,
    [packages, taskType],
  );

  const load = useCallback(async () => {
    const [taskItems, sourceItems] = await Promise.all([
      api<any[]>("/api/schedules"),
      api<GitHubSource[]>("/api/github-sources"),
    ]);
    setTasks((Array.isArray(taskItems) ? taskItems : []).map(normalizeScheduledTask));
    setSources(Array.isArray(sourceItems) ? sourceItems : []);
  }, []);

  useEffect(() => {
    load().catch((reason) => setScheduleError(errorMessage(reason)));
  }, [load]);

  useEffect(() => {
    if (!instanceID && instances[0]) setInstanceID(instances[0].id);
  }, [instanceID, instances]);

  useEffect(() => {
    if (!sourceID && sources[0]) setSourceID(sources[0].id);
  }, [sourceID, sources]);

  useEffect(() => {
    if (!editing && availablePackages.length && !availablePackages.some((item) => item.id === packageID)) {
      setPackageID(availablePackages[0].id);
    }
  }, [availablePackages, editing, packageID]);

  const resetCreate = () => {
    setEditing(null);
    setTaskType("game_update");
    setInstanceID(instances[0]?.id || "");
    setCron("0 4 * * *");
    setPolicy("skip");
    setEnabled(true);
    setRetentionDays(30);
  };

  const beginEdit = (task: ScheduledTask) => {
    setEditing(task);
    setTaskType(task.type);
    setCron(task.cron);
    setPolicy(task.online_policy);
    setEnabled(task.enabled);
    setScheduleError("");
    setScheduleStatus("");
  };

  const createPayload = () => {
    if (packageTask) return JSON.stringify({ package_id: packageID });
    if (releaseTask) return JSON.stringify({ source_id: sourceID });
    if (taskType === "cleanup") return JSON.stringify({ retention_days: retentionDays });
    return "{}";
  };

  const canSubmit = editing
    ? true
    : (!definition.needsInstance || Boolean(instanceID)) &&
      (!releaseTask || Boolean(sourceID)) &&
      (!packageTask || Boolean(packageID));

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setScheduleError("");
    setScheduleStatus(editing ? "正在更新计划…" : "正在保存计划…");
    setSubmitting(true);
    try {
      const body: Record<string, unknown> = editing
        ? {
            id: editing.id,
            instance_id: editing.instance_id,
            type: editing.type,
            cron,
            timezone: editing.timezone,
            online_policy: TASK_TYPES[editing.type].usesPlayerPolicy ? policy : "force",
            payload: editing.payload,
            enabled,
          }
        : {
            instance_id: definition.needsInstance ? instanceID : "",
            type: taskType,
            cron,
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
            online_policy: definition.usesPlayerPolicy ? policy : "force",
            payload: createPayload(),
            enabled,
          };
      if (editing?.last_run) body.last_run = editing.last_run;
      await api("/api/schedules", {
        method: "POST",
        body: JSON.stringify(body),
      });
      const wasEditing = Boolean(editing);
      await load();
      if (wasEditing) resetCreate();
      setScheduleStatus(wasEditing ? "计划已更新" : "计划已保存");
    } catch (reason) {
      setScheduleStatus("");
      setScheduleError(errorMessage(reason));
    } finally {
      setSubmitting(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    setScheduleError("");
    try {
      await api(`/api/schedules/${deleteTarget.id}`, { method: "DELETE" });
      if (editing?.id === deleteTarget.id) resetCreate();
      setDeleteTarget(null);
      await load();
      setScheduleStatus("计划已删除");
    } catch (reason) {
      setScheduleError(errorMessage(reason));
    } finally {
      setDeleting(false);
    }
  };

  const instanceName = (id: string) =>
    instances.find((item) => item.id === id)?.name || `实例引用已失效 · ${id || "未设置"}`;

  return (
    <div className="schedule-page">
      <div className="section-head schedule-page-head">
        <div>
          <p className="eyebrow">AUTOMATION</p>
          <h2>计划任务</h2>
        </div>
        <button className="schedule-help-command" onClick={() => setShowHelp(true)}>
          <BookOpen />
          任务说明
        </button>
      </div>
      <div className="schedule-layout">
        <form
          className="control-form schedule-form"
          aria-label={editing ? "编辑计划" : "新建计划"}
          onSubmit={submit}
        >
          <p className="eyebrow">{editing ? "EDIT SCHEDULE" : "NEW SCHEDULE"}</p>
          <h2>{editing ? `编辑${TASK_TYPES[editing.type].label}` : "添加维护窗口"}</h2>
          <label>
            任务
            <select
              aria-label="任务"
              name="type"
              value={taskType}
              disabled={Boolean(editing)}
              onChange={(event) => {
                const next = event.target.value as ScheduleTaskType;
                setTaskType(next);
                setScheduleError("");
              }}
            >
              {TASK_TYPE_ORDER.map((type) => (
                <option key={type} value={type}>{TASK_TYPES[type].label}</option>
              ))}
            </select>
          </label>
          {definition.needsInstance ? (
            <label>
              实例
              <select
                aria-label="实例"
                value={editing ? editing.instance_id : instanceID}
                disabled={Boolean(editing)}
                onChange={(event) => setInstanceID(event.target.value)}
              >
                {editing && !instances.some((item) => item.id === editing.instance_id) ? (
                  <option value={editing.instance_id}>{instanceName(editing.instance_id)}</option>
                ) : null}
                {instances.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
              </select>
            </label>
          ) : null}
          {releaseTask && !editing ? (
            <label>
              GitHub 源
              <select
                aria-label="GitHub 源"
                value={sourceID}
                onChange={(event) => setSourceID(event.target.value)}
                required
              >
                {sources.map((source) => (
                  <option key={source.id} value={source.id}>{source.name}</option>
                ))}
              </select>
            </label>
          ) : null}
          {packageTask && !editing ? (
            <label>
              插件包
              <select
                aria-label="插件包"
                value={packageID}
                onChange={(event) => setPackageID(event.target.value)}
                required
              >
                {availablePackages.map((item) => (
                  <option key={item.id} value={item.id}>{item.filename} · {item.version}</option>
                ))}
              </select>
            </label>
          ) : null}
          {taskType === "cleanup" && !editing ? (
            <label>
              保留天数
              <input
                aria-label="保留天数"
                type="number"
                min="1"
                max="3650"
                value={retentionDays}
                onChange={(event) => setRetentionDays(Number(event.target.value))}
                required
              />
            </label>
          ) : null}
          {editing ? (
            <div className="schedule-readonly">
              <span>时区</span>
              <b aria-label="任务时区">{editing.timezone}</b>
              <span>固定任务参数</span>
              <b>{payloadSummary(editing, packages, sources)}</b>
            </div>
          ) : null}
          <label>
            Cron
            <input
              aria-label="Cron"
              value={cron}
              onChange={(event) => setCron(event.target.value)}
              required
            />
          </label>
          {definition.usesPlayerPolicy ? (
            <label>
              在线玩家策略
              <select
                aria-label="在线玩家策略"
                value={policy}
                onChange={(event) => setPolicy(event.target.value as OnlinePolicy)}
              >
                <option value="skip">跳过</option>
                <option value="wait">等待</option>
                <option value="force">强制执行</option>
              </select>
            </label>
          ) : null}
          <label className="schedule-enabled">
            <input
              aria-label="启用计划"
              type="checkbox"
              checked={enabled}
              onChange={(event) => setEnabled(event.target.checked)}
            />
            <span>启用计划</span>
          </label>
          {scheduleError ? <div className="error" role="alert">{scheduleError}</div> : null}
          {scheduleStatus ? <div className="operation-status" role="status">{scheduleStatus}</div> : null}
          <div className="schedule-form-actions">
            {editing ? (
              <button type="button" onClick={resetCreate}>取消编辑</button>
            ) : null}
            <button className="create" disabled={submitting || !canSubmit}>
              {editing ? "保存修改" : "保存计划"}
            </button>
          </div>
        </form>
        <section className="data-panel schedule-list" aria-label="执行计划">
          <div className="panel-title">
            <h2>执行计划</h2>
            <small>{tasks.length} 项</small>
          </div>
          {tasks.map((task) => {
            const item = TASK_TYPES[task.type];
            return (
              <article className="schedule-row" key={task.id}>
                <div className="schedule-row-main">
                  <div className="schedule-row-title">
                    <b>{item.label}</b>
                    <span className={task.enabled ? "enabled" : "disabled"}>
                      {task.enabled ? "启用" : "停用"}
                    </span>
                  </div>
                  <strong>{item.needsInstance ? instanceName(task.instance_id) : "全局"}</strong>
                  <small>{payloadSummary(task, packages, sources)}</small>
                </div>
                <div className="schedule-row-meta">
                  <code>{task.cron}</code>
                  <span>{task.timezone}</span>
                  <span>{item.usesPlayerPolicy ? POLICY_LABELS[task.online_policy] : "不检查玩家"}</span>
                </div>
                <div className="schedule-row-times">
                  <span><small>上次执行</small><b>{formatTime(task.last_run, "尚未执行")}</b></span>
                  <span><small>下次执行</small><b>{task.enabled ? formatTime(task.next_run, "未安排") : "已停用"}</b></span>
                </div>
                <div className="schedule-row-actions">
                  <button
                    className="icon-btn"
                    aria-label={`编辑 ${item.label}`}
                    title="编辑计划"
                    onClick={() => beginEdit(task)}
                  >
                    <Pencil />
                  </button>
                  <button
                    className="icon-btn danger"
                    aria-label={`删除 ${item.label}`}
                    title="删除计划"
                    onClick={() => setDeleteTarget(task)}
                  >
                    <Trash2 />
                  </button>
                </div>
              </article>
            );
          })}
          {!tasks.length ? <div className="empty">暂无计划任务</div> : null}
        </section>
      </div>
      {showHelp ? <TaskHelpDialog close={() => setShowHelp(false)} /> : null}
      {deleteTarget ? (
        <DialogFrame
          title={`删除${TASK_TYPES[deleteTarget.type].label}计划？`}
          label={`删除${TASK_TYPES[deleteTarget.type].label}计划？`}
          className="schedule-delete-dialog"
          close={() => setDeleteTarget(null)}
        >
          <p>
            {deleteTarget.cron} · {TASK_TYPES[deleteTarget.type].needsInstance ? instanceName(deleteTarget.instance_id) : "全局"}
          </p>
          <p>删除后不会再按此计划触发新任务；已经排队或正在执行的任务不会被取消。</p>
          <div className="schedule-dialog-actions">
            <button onClick={() => setDeleteTarget(null)}>取消</button>
            <button
              className="danger"
              aria-label="确认删除计划"
              disabled={deleting}
              onClick={() => void confirmDelete()}
            >
              确认删除计划
            </button>
          </div>
        </DialogFrame>
      ) : null}
    </div>
  );
}
