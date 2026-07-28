import { useCallback, useEffect, useState } from "react";
import { CheckCircle2, ChevronDown, Clock, FileText, RotateCw, Search, XCircle } from "lucide-react";
import { api, type Job, type JobEvent } from "../api/client";

const TERMINAL_STATUSES = new Set(["succeeded", "failed", "interrupted"]);
const STATUS_LABELS: Record<string, string> = {
  pending: "排队中",
  running: "执行中",
  succeeded: "已成功",
  failed: "已失败",
  interrupted: "已中断",
};
const EVENT_LABELS: Record<string, string> = {
  queued: "进入队列",
  started: "开始执行",
  progress: "执行进度",
  succeeded: "任务成功",
  failed: "任务失败",
  interrupted: "任务中断",
  snapshot: "历史快照",
};
const STATUS_ICONS = {
  pending: Clock,
  running: Clock,
  succeeded: CheckCircle2,
  failed: XCircle,
  interrupted: XCircle,
};

export function JobsPage({ onOpenLogs }: { onOpenLogs?: (job: Job) => void }) {
  const [items, setItems] = useState<Job[]>([]);
  const [jobsError, setJobsError] = useState("");
  const [expandedID, setExpandedID] = useState("");
  const [details, setDetails] = useState<Record<string, Job>>({});
  const [detailErrors, setDetailErrors] = useState<Record<string, string>>({});
  const [loadingID, setLoadingID] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [query, setQuery] = useState("");

  useEffect(() => {
    let active = true;
    api<Job[]>("/api/jobs")
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
    events.onerror = () =>
      setJobsError("任务实时流已断开，正在由浏览器重连");
    return () => {
      active = false;
      events.close();
    };
  }, []);

  const loadDetails = useCallback(async (item: Job) => {
    setLoadingID(item.ID);
    setDetailErrors((current) => ({ ...current, [item.ID]: "" }));
    try {
      const detail = await api<Job>(`/api/jobs/${item.ID}`);
      setDetails((current) => ({ ...current, [item.ID]: detail }));
    } catch (reason) {
      setDetailErrors((current) => ({
        ...current,
        [item.ID]: reason instanceof Error ? reason.message : String(reason),
      }));
    } finally {
      setLoadingID((current) => (current === item.ID ? "" : current));
    }
  }, []);

  useEffect(() => {
    if (!expandedID || loadingID === expandedID) return;
    const item = items.find((candidate) => candidate.ID === expandedID);
    const detail = details[expandedID];
    if (
      item &&
      detail &&
      item.UpdatedAt &&
      item.UpdatedAt !== detail.UpdatedAt
    ) {
      void loadDetails(item);
    }
  }, [details, expandedID, items, loadDetails, loadingID]);

  const toggle = (item: Job) => {
    if (item.ID.startsWith("vpk-restart:")) {
      return;
    }
    if (expandedID === item.ID) {
      setExpandedID("");
      return;
    }
    setExpandedID(item.ID);
    if (!details[item.ID] || !TERMINAL_STATUSES.has(item.Status)) {
      void loadDetails(item);
    }
  };
  const normalizedQuery = query.trim().toLowerCase();
  const filteredItems = items.filter((item) => {
    if (statusFilter !== "all" && item.Status !== statusFilter) return false;
    if (!normalizedQuery) return true;
    return [
      item.ID,
      item.Type,
      item.InstanceID,
      item.Stage,
      item.Message,
      item.Error,
    ]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(normalizedQuery));
  });

  return (
    <section className="job-feed">
      <div className="job-filters">
        <div className="job-status-filters" role="group" aria-label="任务状态筛选">
          <span>状态筛选</span>
          {[
            ["all", "全部"],
            ["running", "执行中"],
            ["pending", "排队中"],
            ["succeeded", "已成功"],
            ["failed", "已失败"],
          ].map(([value, label]) => (
            <button
              className={statusFilter === value ? "active" : ""}
              key={value}
              type="button"
              onClick={() => setStatusFilter(value)}
            >
              {label}
            </button>
          ))}
        </div>
        <label className="job-search">
          <Search aria-hidden="true" />
          <input
            aria-label="搜索任务"
            type="search"
            placeholder="搜索任务类型或编号..."
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
      </div>
      {jobsError ? (
        <div className="error" role="alert">
          {jobsError}
        </div>
      ) : null}
      <div className="job-table" role="table" aria-label="后台任务">
        <div className="job-table-head" role="row">
          {["任务编号", "任务类型", "目标对象", "阶段 / 进度", "状态", "创建时间", "操作"].map((column) => (
            <span key={column} role="columnheader">{column}</span>
          ))}
        </div>
        {filteredItems.map((item) => {
          const type = item.Type || "unknown";
          const expanded = expandedID === item.ID;
          const detail = details[item.ID];
          const panelID = `job-log-${item.ID}`;
          const StatusIcon = STATUS_ICONS[item.Status as keyof typeof STATUS_ICONS] || Clock;
          return (
            <article
              className={`job-entry ${expanded ? "expanded" : ""}`}
              key={item.ID}
              role="rowgroup"
            >
              <div className="job-row" role="row">
                <span className="job-code">
                  <span>{item.ID.slice(0, 8)}</span>
                </span>
                <span className="job-type">{type}</span>
                <span className="job-target">{item.InstanceID ? item.InstanceID.slice(0, 8) : "全局共享"}</span>
                <span className="job-stage">
                  <b>{item.Stage || "queued"}</b>
                  {item.Status === "running" ? (
                    <span className="job-progress-track" role="progressbar" aria-label={`${type} 任务进度`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={item.Percent || 0}>
                      <i style={{ width: `${item.Percent || 0}%` }} />
                    </span>
                  ) : null}
                  <small>{item.Error || item.Message || "等待后台执行"}</small>
                  <small>{durationSummary(item)}</small>
                </span>
                <span className={`job-state ${item.Status}`}>
                  <StatusIcon aria-hidden="true" />
                  <span>{STATUS_LABELS[item.Status] || item.Status}</span>
                  <small>{item.Status}</small>
                </span>
                <span className="job-time">{formatTimestamp(item.CreatedAt)}</span>
                <span className="job-operation">
                  <button type="button" className="job-event-toggle" aria-expanded={expanded} aria-controls={panelID} aria-label={`查看 ${type} 任务日志，任务 ID ${item.ID}`} onClick={() => toggle(item)}>
                    <span>事件</span><ChevronDown className="job-chevron" aria-hidden="true" />
                  </button>
                  {onOpenLogs ? <button type="button" className="job-full-log" aria-label={`打开 ${type} 完整任务日志`} onClick={() => onOpenLogs(item)}><FileText aria-hidden="true" /><span>任务日志</span></button> : null}
                </span>
              </div>
              {expanded ? (
                <div
                  className="job-log-panel"
                  id={panelID}
                  role="region"
                  aria-label={`${type} 任务日志`}
                >
                  {loadingID === item.ID && !detail ? (
                    <div className="job-log-loading">正在读取任务日志…</div>
                  ) : detailErrors[item.ID] ? (
                    <div className="job-log-error" role="alert">
                      <span>{detailErrors[item.ID]}</span>
                      <button type="button" onClick={() => void loadDetails(item)}>
                        <RotateCw aria-hidden="true" />
                        重试
                      </button>
                    </div>
                  ) : detail ? (
                    <JobLog detail={detail} />
                  ) : (
                    <div className="job-log-loading">暂无任务日志</div>
                  )}
                </div>
              ) : null}
            </article>
          );
        })}
        {filteredItems.length === 0 ? <div className="empty">尚无匹配的后台任务</div> : null}
      </div>
    </section>
  );
}

function JobLog({ detail }: { detail: Job }) {
  const events = detail.Events || [];
  return (
    <>
      <b className="job-log-title">任务事件链:</b>
      {detail.Error ? (
        <div className="job-log-failure">
          <b>{detail.Status === "interrupted" ? "中断原因" : "失败原因"}</b>
          <span>{detail.Error}</span>
        </div>
      ) : null}
      <div className="job-log-meta">
        <span>发起时间 {formatTimestamp(detail.CreatedAt)}</span>
        <span>排队耗时 {queueDuration(detail)}</span>
        <span>执行用时 {executionDuration(detail)}</span>
      </div>
      {events.length ? (
        <ol className="job-log-events">
          {events.map((event, index) => (
            <JobEventRow
              event={event}
              key={event.ID || `${event.Kind}-${event.CreatedAt}-${index}`}
            />
          ))}
        </ol>
      ) : (
        <div className="job-log-empty">此任务没有可显示的结构化事件</div>
      )}
    </>
  );
}

function JobEventRow({ event }: { event: JobEvent }) {
  const terminalError =
    event.Kind === "failed" || event.Kind === "interrupted";
  return (
    <li className={terminalError ? "error-event" : ""}>
      <time dateTime={event.CreatedAt}>{formatEventTime(event.CreatedAt)}</time>
      <div>
        <b>{EVENT_LABELS[event.Kind] || event.Kind}</b>
        <span>
          {event.Stage ? `${event.Stage} · ` : ""}
          {event.Kind === "progress" ? `${event.Percent}% · ` : ""}
          {event.Message || "无附加消息"}
        </span>
      </div>
    </li>
  );
}

function validDate(value?: string | null) {
  if (!value) return null;
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

export function formatTimestamp(value?: string | null) {
  const date = validDate(value);
  if (!date) return "--";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}

function formatEventTime(value?: string | null) {
  const date = validDate(value);
  if (!date) return "--:--:--";
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
}

function formatDuration(milliseconds: number | null) {
  if (milliseconds === null || milliseconds < 0) return "--";
  const seconds = Math.floor(milliseconds / 1000);
  if (seconds < 60) return `${seconds}秒`;
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  if (minutes < 60) return rest ? `${minutes}分${rest}秒` : `${minutes}分`;
  const hours = Math.floor(minutes / 60);
  const minuteRest = minutes % 60;
  return minuteRest ? `${hours}小时${minuteRest}分` : `${hours}小时`;
}

function elapsed(from?: string | null, to?: string | null) {
  const start = validDate(from);
  const end = validDate(to);
  return start && end ? end.getTime() - start.getTime() : null;
}

function executionDuration(job: Job) {
  if (!job.StartedAt) {
    if (TERMINAL_STATUSES.has(job.Status)) return "未执行";
    return job.Status === "pending" ? "尚未开始" : "--";
  }
  const end = job.FinishedAt || new Date().toISOString();
  return formatDuration(elapsed(job.StartedAt, end));
}

function queueDuration(job: Job) {
  if (!job.StartedAt) {
    return TERMINAL_STATUSES.has(job.Status)
      ? formatDuration(elapsed(job.CreatedAt, job.FinishedAt))
      : "排队中";
  }
  return formatDuration(elapsed(job.CreatedAt, job.StartedAt));
}

function durationSummary(job: Job) {
  if (!job.StartedAt) {
    return TERMINAL_STATUSES.has(job.Status) ? "未执行" : "排队中";
  }
  return executionDuration(job);
}
