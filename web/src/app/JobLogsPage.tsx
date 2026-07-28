import {
  useDeferredValue,
  useEffect,
  useMemo,
  useState,
  type UIEvent,
} from "react";
import {
  ArrowLeft,
  Copy,
  Download,
  Search,
  StepForward,
} from "lucide-react";
import {
  api,
  apiBlob,
  type Job,
  type JobLogLevel,
  type JobLogPage,
  type JobLogRecord,
} from "../api/client";
import { useConsoleFollow } from "./useConsoleFollow";

type Props = {
  job: Job;
  onBack: () => void;
};

export function JobLogsPage({ job, onBack }: Props) {
  const [records, setRecords] = useState<JobLogRecord[]>([]);
  const [query, setQuery] = useState("");
  const [level, setLevel] = useState<JobLogLevel | "all">("all");
  const [truncated, setTruncated] = useState(false);
  const [status, setStatus] = useState("正在读取日志");
  const [unread, setUnread] = useState(0);
  const deferredQuery = useDeferredValue(query.trim().toLowerCase());
  const follow = useConsoleFollow(records.at(-1)?.seq || 0);

  useEffect(() => {
    let active = true;
    let events: EventSource | null = null;
    api<JobLogPage>(`/api/jobs/${job.ID}/logs?limit=1000`)
      .then((page) => {
        if (!active) return;
        setRecords(page.records || []);
        setTruncated(Boolean(page.truncated));
        setStatus("实时连接中");
        const after = page.next_seq || 0;
        events = new EventSource(
          `/api/jobs/${job.ID}/logs/stream?after_seq=${after}`,
        );
        events.addEventListener("job-log", (event) => {
          if (!active) return;
          try {
            const record = JSON.parse(
              (event as MessageEvent<string>).data,
            ) as JobLogRecord;
            setRecords((current) =>
              current.some((item) => item.seq === record.seq)
                ? current
                : [...current, record],
            );
            if (!follow.isFollowing()) setUnread((count) => count + 1);
          } catch {
            setStatus("收到无效日志记录");
          }
        });
        events.onerror = () => setStatus("实时流已断开，浏览器正在重连");
      })
      .catch((reason) => {
        if (active) setStatus(reason instanceof Error ? reason.message : String(reason));
      });
    return () => {
      active = false;
      events?.close();
    };
  }, [job.ID]);

  const visible = useMemo(
    () =>
      records.filter((record) => {
        if (level !== "all" && record.level !== level) return false;
        return !deferredQuery || record.message.toLowerCase().includes(deferredQuery) || record.source.toLowerCase().includes(deferredQuery);
      }),
    [deferredQuery, level, records],
  );

  const onScroll = (event: UIEvent<HTMLPreElement>) => {
    follow.onScroll(event);
    if (follow.isFollowing()) setUnread(0);
  };
  const resume = () => {
    setUnread(0);
    follow.forceFollow();
  };
  const text = visible
    .map((record) => `${record.timestamp} [${record.source}/${record.level}] ${record.message}`)
    .join("\n");
  const copy = async () => navigator.clipboard.writeText(text);
  const download = async () => {
    const blob = await apiBlob(`/api/jobs/${job.ID}/logs/download`);
    const href = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = href;
    anchor.download = `job-${job.ID}.jsonl`;
    anchor.click();
    URL.revokeObjectURL(href);
  };

  return (
    <section className="task-log-page">
      <div className="task-log-head">
        <div className="task-log-heading-group">
          <button className="task-log-back" type="button" aria-label="返回任务列表" onClick={onBack}><ArrowLeft aria-hidden="true" /><span>返回任务列表</span></button>
          <div>
            <h2 aria-label={`任务日志: ${job.Type || "unknown"}`}>任务日志: {job.Type || "unknown"}<span className="task-log-id">{job.ID.slice(0, 8)}</span></h2>
            <p>底层运维过程的详细事件流与诊断数据输出</p>
          </div>
        </div>
        <div className="task-log-head-actions">
          <button type="button" aria-label="复制日志" onClick={() => void copy()}><Copy aria-hidden="true" /><span>复制日志</span></button>
          <button className="task-log-download" type="button" aria-label="下载完整日志" onClick={() => void download()}><Download aria-hidden="true" /><span>下载完整日志</span></button>
        </div>
      </div>

      <div className="task-log-toolbar">
        <div className="task-log-levels" role="group" aria-label="日志级别筛选">
          <span>级别筛选:</span>
          {([['all', '全部'], ['info', '信息'], ['warn', '警告'], ['error', '错误']] as const).map(([value, label]) => <button className={level === value ? 'active' : ''} type="button" key={value} onClick={() => setLevel(value)}>{label}</button>)}
        </div>
        <label className="task-log-search">
          <Search aria-hidden="true" />
          <input aria-label="搜索任务日志" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索日志消息..." />
        </label>
      </div>

      <div className="task-log-status" role="status"><span>{status}</span><span>{visible.length} / {records.length} 条</span></div>
      {truncated ? <div className="task-log-truncated">早期日志已因 10 MiB 上限截断</div> : null}
      <div className="task-log-output-wrap">
        <pre className="task-log-output" ref={follow.outputRef} onScroll={onScroll} tabIndex={0}>
          {visible.map((record) => (
            <span className={`task-log-line ${record.level}`} key={record.seq}>
              <span>{record.seq}</span>
              <time>{formatTime(record.timestamp)}</time>
              <b>{record.source}</b>
              <code>{record.message}</code>
            </span>
          ))}
          {visible.length === 0 ? <span className="task-log-empty">没有符合条件的日志</span> : null}
        </pre>
        {unread > 0 ? (
          <button className="task-log-resume" type="button" onClick={resume}>
            <StepForward aria-hidden="true" />恢复跟随 · {unread} 条新日志
          </button>
        ) : null}
      </div>
    </section>
  );
}

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "--:--:--";
  return new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false }).format(date);
}
