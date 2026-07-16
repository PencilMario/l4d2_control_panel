import { useCallback, useEffect, useState } from "react";
import { ChevronDown, Clock3, ExternalLink, RotateCw } from "lucide-react";
import { api, type Job, type JobEvent } from "../api/client";

const TERMINAL_STATUSES = new Set(["succeeded", "failed", "interrupted"]);
const STATUS_LABELS: Record<string, string> = {
  pending: "排队中",
  running: "执行中",
  succeeded: "成功",
  failed: "失败",
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

export function JobsPage({ onOpenLogs }: { onOpenLogs?: (job: Job) => void }) {
  const [items, setItems] = useState<Job[]>([]);
  const [jobsError, setJobsError] = useState("");
  const [expandedID, setExpandedID] = useState("");
  const [details, setDetails] = useState<Record<string, Job>>({});
  const [detailErrors, setDetailErrors] = useState<Record<string, string>>({});
  const [loadingID, setLoadingID] = useState("");

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
    if (expandedID === item.ID) {
      setExpandedID("");
      return;
    }
    setExpandedID(item.ID);
    if (!details[item.ID] || !TERMINAL_STATUSES.has(item.Status)) {
      void loadDetails(item);
    }
  };

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
        {items.map((item) => {
          const type = item.Type || "unknown";
          const expanded = expandedID === item.ID;
          const detail = details[item.ID];
          const panelID = `job-log-${item.ID}`;
          return (
            <article
              className={`job-entry ${expanded ? "expanded" : ""}`}
              key={item.ID}
              role="listitem"
            >
              <button
                className="job-row job-row-toggle"
                type="button"
                aria-expanded={expanded}
                aria-controls={panelID}
                aria-label={`查看 ${type} 任务日志，任务 ID ${item.ID}`}
                onClick={() => toggle(item)}
              >
                <span className="job-code">
                  <span>{type}</span>
                  <small>{item.ID.slice(0, 8)}</small>
                </span>
                <span className="job-stage">
                  <b>{item.Stage || "queued"}</b>
                  <small>{item.Error || item.Message || "等待后台执行"}</small>
                </span>
                <span className="job-time">
                  <small>
                    <Clock3 aria-hidden="true" />
                    {formatTimestamp(item.CreatedAt)}
                  </small>
                  <b>{durationSummary(item)}</b>
                </span>
                <span
                  className="job-progress"
                  aria-label={`进度 ${item.Percent || 0}%`}
                >
                  <i
                    style={{ width: `${Math.max(0, item.Percent || 0)}%` }}
                  />
                </span>
                <span className={`job-state ${item.Status}`}>
                  {STATUS_LABELS[item.Status] || item.Status}
                  <small>{item.Status}</small>
                </span>
                <ChevronDown className="job-chevron" aria-hidden="true" />
              </button>
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
                    <>
                      {onOpenLogs ? (
                        <div className="job-log-actions">
                          <button type="button" onClick={() => onOpenLogs(detail)}>
                            <ExternalLink aria-hidden="true" />
                            打开完整日志
                          </button>
                        </div>
                      ) : null}
                      <JobLog detail={detail} />
                    </>
                  ) : (
                    <div className="job-log-loading">暂无任务日志</div>
                  )}
                </div>
              ) : null}
            </article>
          );
        })}
        {items.length === 0 ? <div className="empty">尚无后台任务</div> : null}
      </div>
    </section>
  );
}

function JobLog({ detail }: { detail: Job }) {
  const events = detail.Events || [];
  return (
    <>
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
