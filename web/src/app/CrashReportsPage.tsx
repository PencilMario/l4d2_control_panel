import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  Clock3,
  Code2,
  Download,
  FileArchive,
  FileCode2,
  FileText,
  Filter,
  LoaderCircle,
  RefreshCw,
  Search,
  ServerCrash,
  X,
} from "lucide-react";
import { api, apiBlob, apiText } from "../api/client";
import { CrashAnalysisReader } from "./CrashAnalysisReader";

export type CrashAnalysisStatus = "queued" | "running" | "succeeded" | "failed" | "unconfigured" | "";

export type CrashModule = {
  index: number;
  debug_file: string;
  debug_identifier: string;
  code_identifier?: string;
  platform?: string;
  architecture?: string;
  decision?: string;
  symbol_artifact?: string;
  binary_artifact?: string;
};

export type CrashFrame = {
  module_index: number;
  offset: string;
};

export type CrashSignature = {
  version: number;
  timestamp?: string;
  platform?: string;
  architecture?: string;
  crashed?: string;
  crash_reason?: string;
  crash_address?: string;
  requesting_thread?: number;
  modules?: CrashModule[];
  frames?: CrashFrame[];
};

export type CrashReport = {
  id: string;
  instance_id?: string;
  received_at: string;
  updated_at: string;
  minidump_size: number;
  metadata_size: number;
  sha256: string;
  user_id?: string;
  game_directory?: string;
  extension_version?: string;
  server_id?: string;
  crash_signature?: string;
  presubmit_token?: string;
  parsed_signature?: CrashSignature;
  modules?: CrashModule[];
  stackwalk_status?: CrashAnalysisStatus;
  stackwalk_error?: string;
  stackwalk_tool?: string;
  stackwalk_at?: string;
  ai_status?: CrashAnalysisStatus;
  ai_error?: string;
  ai_model?: string;
  ai_input_sha256?: string;
  ai_analysis?: string;
  ai_started_at?: string;
  ai_completed_at?: string;
};

export type CrashReportDetails = CrashReport & { metadata: string };

type InstanceOption = { id: string; name: string };
type APIRequest = (path: string, init?: RequestInit) => Promise<unknown>;

type Props = {
  instances: InstanceOption[];
  apiRequest?: APIRequest;
  textRequest?: (path: string, init?: RequestInit) => Promise<string>;
  blobRequest?: (path: string, init?: RequestInit) => Promise<Blob>;
};

const terminalStatuses = new Set<CrashAnalysisStatus>(["succeeded", "failed", "unconfigured"]);

const statusLabel = (status: CrashAnalysisStatus | undefined) => {
  switch (status) {
    case "queued":
      return "排队中";
    case "running":
      return "分析中";
    case "succeeded":
      return "已完成";
    case "failed":
      return "失败";
    case "unconfigured":
      return "未配置";
    default:
      return "未分析";
  }
};

const statusClass = (status: CrashAnalysisStatus | undefined) =>
  status ? `crash-status-${status}` : "crash-status-empty";

const formatBytes = (value: number | undefined) => {
  if (!Number.isFinite(value)) return "--";
  if ((value || 0) >= 1024 * 1024 * 1024) return `${((value || 0) / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  if ((value || 0) >= 1024 * 1024) return `${((value || 0) / (1024 * 1024)).toFixed(1)} MB`;
  if ((value || 0) >= 1024) return `${((value || 0) / 1024).toFixed(1)} KB`;
  return `${value || 0} B`;
};

const formatDate = (value: string | undefined) => {
  if (!value) return "--";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
};

const shortID = (value: string) => `${value.slice(0, 10)}…${value.slice(-8)}`;

export function CrashReportsPage({ instances, apiRequest, textRequest, blobRequest }: Props) {
  const request = useMemo<APIRequest>(() => apiRequest || ((path, init) => api<unknown>(path, init)), [apiRequest]);
  const readText = useMemo(() => textRequest || ((path: string, init?: RequestInit) => apiText(path, init)), [textRequest]);
  const readBlob = useMemo(() => blobRequest || ((path: string, init?: RequestInit) => apiBlob(path, init)), [blobRequest]);
  const [reports, setReports] = useState<CrashReport[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [details, setDetails] = useState<CrashReportDetails | null>(null);
  const [stackwalk, setStackwalk] = useState("");
  const [instanceFilter, setInstanceFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [signatureFilter, setSignatureFilter] = useState("");
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [analysisBusy, setAnalysisBusy] = useState(false);
  const [analysisReaderOpen, setAnalysisReaderOpen] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const instanceNames = useMemo(() => new Map(instances.map((item) => [item.id, item.name])), [instances]);
  const filteredReports = useMemo(() => {
    const needle = signatureFilter.trim().toLowerCase();
    return reports.filter((report) => {
      if (instanceFilter && report.instance_id !== instanceFilter) return false;
      if (statusFilter && report.stackwalk_status !== statusFilter && report.ai_status !== statusFilter) return false;
      return !needle || (report.crash_signature || "").toLowerCase().includes(needle);
    });
  }, [instanceFilter, reports, signatureFilter, statusFilter]);

  const loadReports = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const value = await request("/api/crash-reports");
      const next = Array.isArray(value) ? value as CrashReport[] : [];
      setReports(next);
      setSelectedID((current) => current && next.some((item) => item.id === current) ? current : next[0]?.id || "");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setLoading(false);
    }
  }, [request]);

  useEffect(() => {
    void loadReports();
  }, [loadReports]);

  /* Keep the selected report stable when a background refresh returns the same list. */
  useEffect(() => {
    if (!filteredReports.length) {
      setSelectedID("");
      return;
    }
    if (!filteredReports.some((report) => report.id === selectedID)) setSelectedID(filteredReports[0].id);
  }, [filteredReports, selectedID]);

  useEffect(() => {
    if (!selectedID) {
      setDetails(null);
      setStackwalk("");
      setAnalysisReaderOpen(false);
      return;
    }
    let active = true;
    setDetailLoading(true);
    setDetails(null);
    setStackwalk("");
    setAnalysisReaderOpen(false);
    setError("");
    void request(`/api/crash-reports/${selectedID}`)
      .then(async (value) => {
        if (!active) return;
        const next = value as CrashReportDetails;
        setDetails(next);
        if (next.stackwalk_status !== "succeeded") return;
        try {
          const text = await readText(`/api/crash-reports/${selectedID}/download?file=stackwalk`);
          if (active) setStackwalk(text);
        } catch {
          if (active) setStackwalk("");
        }
      })
      .catch((reason) => {
        if (active) setError(reason instanceof Error ? reason.message : String(reason));
      })
      .finally(() => {
        if (active) setDetailLoading(false);
      });
    return () => { active = false; };
  }, [readText, request, selectedID]);

  const download = async (file: string, artifact?: string) => {
    if (!selectedID) return;
    setError("");
    try {
      const query = new URLSearchParams({ file });
      if (artifact) query.set("artifact", artifact);
      const blob = await readBlob(`/api/crash-reports/${selectedID}/download?${query.toString()}`);
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `${selectedID}.${file}`;
      anchor.click();
      URL.revokeObjectURL(url);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    }
  };

  const analyze = async () => {
    if (!selectedID || analysisBusy) return;
    setAnalysisBusy(true);
    setError("");
    setNotice("");
    try {
      await request(`/api/crash-reports/${selectedID}/analyze`, {
        method: "POST",
        body: JSON.stringify({ ai: true }),
      });
      setNotice("分析任务已提交");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setAnalysisBusy(false);
    }
  };

  const selectedSummary = reports.find((report) => report.id === selectedID) || details;
  const selectedModules = details?.modules || details?.parsed_signature?.modules || [];
  const canAnalyze = Boolean(details && terminalStatuses.has(details.ai_status || "") || details && details.ai_status !== "running" && details.ai_status !== "queued");

  if (analysisReaderOpen && details?.ai_analysis) {
    return (
      <CrashAnalysisReader
        analysis={details.ai_analysis}
        instanceName={instanceNames.get(details.instance_id || "") || details.instance_id || "未关联实例"}
        reportID={details.id}
        onBack={() => setAnalysisReaderOpen(false)}
      />
    );
  }

  return (
    <div className="crash-reports-page">
      <header className="crash-page-head">
        <div>
          <p className="eyebrow">ACCELERATOR RECEIVER</p>
          <h1>崩溃报告</h1>
          <p>集中查看 Accelerator 转储、符号解析结果与 AI 诊断记录。</p>
        </div>
        <button className="icon-btn crash-refresh" type="button" aria-label="刷新崩溃报告" disabled={loading} onClick={() => void loadReports()}>
          <RefreshCw />
        </button>
      </header>
      {error ? <div className="error-banner" role="alert">{error}<button type="button" aria-label="关闭错误" onClick={() => setError("")}><X /></button></div> : null}
      <div className="crash-toolbar" role="toolbar" aria-label="崩溃报告筛选">
        <div className="crash-filter-label"><Filter /><span>筛选</span></div>
        <label>
          <span className="sr-only">筛选实例</span>
          <select aria-label="筛选实例" value={instanceFilter} onChange={(event) => setInstanceFilter(event.target.value)}>
            <option value="">全部实例</option>
            {instances.map((instance) => <option key={instance.id} value={instance.id}>{instance.name}</option>)}
          </select>
        </label>
        <label>
          <span className="sr-only">筛选状态</span>
          <select aria-label="筛选状态" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
            <option value="">全部状态</option>
            <option value="queued">排队中</option>
            <option value="running">分析中</option>
            <option value="succeeded">已完成</option>
            <option value="failed">失败</option>
            <option value="unconfigured">未配置</option>
          </select>
        </label>
        <label className="crash-search">
          <Search aria-hidden="true" />
          <span className="sr-only">搜索崩溃签名</span>
          <input type="search" aria-label="搜索崩溃签名" placeholder="搜索崩溃签名..." value={signatureFilter} onChange={(event) => setSignatureFilter(event.target.value)} />
        </label>
        <span className="crash-count">{filteredReports.length} / {reports.length} 条</span>
      </div>
      <div className="crash-layout">
        <section className="crash-list-panel" aria-label="崩溃报告列表">
          <div className="crash-panel-head"><div><span className="eyebrow">REPORT QUEUE</span><h2>最近上传</h2></div><ServerCrash /></div>
          {loading ? <div className="crash-empty"><LoaderCircle className="spin" />正在读取报告…</div> : null}
          {!loading && !filteredReports.length ? <div className="crash-empty"><FileArchive />暂无匹配的崩溃报告</div> : null}
          <ul className="crash-report-list">
            {filteredReports.map((report) => (
              <li key={report.id}>
                <button type="button" className={`crash-report-row${report.id === selectedID ? " selected" : ""}`} onClick={() => setSelectedID(report.id)}>
                  <span className="crash-row-mark"><AlertTriangle /></span>
                  <span className="crash-row-main">
                    <strong>{instanceNames.get(report.instance_id || "") || report.instance_id || "未关联实例"}</strong>
                    <small>{formatDate(report.received_at)} · {formatBytes(report.minidump_size)}</small>
                    <code>{report.parsed_signature?.crash_reason || report.crash_signature || shortID(report.id)}</code>
                  </span>
                  <span className={`crash-row-status ${statusClass(report.ai_status || report.stackwalk_status)}`}>{statusLabel(report.ai_status || report.stackwalk_status)}</span>
                </button>
              </li>
            ))}
          </ul>
        </section>
        <section className="crash-detail-panel" aria-label="崩溃报告详情">
          {!selectedSummary ? <div className="crash-detail-empty"><ServerCrash /><h2>选择一份报告</h2><p>上传的转储和诊断结果会显示在这里。</p></div> : null}
          {selectedSummary && detailLoading ? <div className="crash-detail-empty"><LoaderCircle className="spin" /><p>正在加载报告详情…</p></div> : null}
          {details ? (
            <>
              <header className="crash-detail-head">
                <div className="crash-detail-title"><span className="crash-detail-icon"><AlertTriangle /></span><div><p className="eyebrow">CRASH REPORT</p><h2>{instanceNames.get(details.instance_id || "") || details.instance_id || "未关联实例"}</h2><code>{shortID(details.id)}</code></div></div>
                <div className="crash-detail-actions">
                  <button type="button" className="command-primary" disabled={!canAnalyze || analysisBusy} aria-busy={analysisBusy} onClick={() => void analyze()}>{analysisBusy ? <LoaderCircle className="spin" /> : <Bot />}<span>{analysisBusy ? "提交中…" : "重新分析"}</span></button>
                  <button type="button" className="icon-btn" aria-label="下载转储" onClick={() => void download("minidump")}><Download /></button>
                </div>
              </header>
              {notice ? <p className="crash-notice" role="status">{notice}</p> : null}
              <div className="crash-summary-grid">
                <CrashDatum label="上传时间" value={formatDate(details.received_at)} icon={<Clock3 />} />
                <CrashDatum label="崩溃原因" value={details.parsed_signature?.crash_reason || "未知"} icon={<AlertTriangle />} />
                <CrashDatum label="平台 / 架构" value={`${details.parsed_signature?.platform || "--"} / ${details.parsed_signature?.architecture || "--"}`} icon={<Code2 />} />
                <CrashDatum label="扩展版本" value={details.extension_version || "--"} icon={<FileCode2 />} />
              </div>
              <div className="crash-artifact-actions">
                <button type="button" onClick={() => void download("metadata")}><FileText />metadata</button>
                <button type="button" onClick={() => void download("stackwalk")} disabled={details.stackwalk_status !== "succeeded"}><Code2 />stackwalk</button>
                <button type="button" onClick={() => void download("ai")} disabled={details.ai_status !== "succeeded"}><Bot />AI 分析</button>
              </div>
              <div className="crash-analysis-grid">
                <AnalysisCard title="Stackwalk" icon={<Code2 />} status={details.stackwalk_status} error={details.stackwalk_error}>
                  {stackwalk ? <pre className="crash-code-view">{stackwalk}</pre> : <div className="crash-inline-empty">暂无 stackwalk 输出</div>}
                </AnalysisCard>
                <AnalysisCard title="AI 诊断" icon={<Bot />} status={details.ai_status} error={details.ai_error}>
                  {details.ai_analysis ? <div className="crash-ai-action"><p>AI 已生成诊断报告，可在独立阅读页查看完整 Markdown 内容。</p><button type="button" onClick={() => setAnalysisReaderOpen(true)}><Bot />查看 AI 分析</button></div> : <div className="crash-inline-empty">暂无 AI 诊断结果</div>}
                </AnalysisCard>
              </div>
              <div className="crash-data-grid">
                <section className="crash-subpanel"><div className="crash-subpanel-head"><h3>上传元数据</h3><small>{formatBytes(details.metadata_size)}</small></div><pre className="crash-metadata-view">{details.metadata || "暂无 metadata"}</pre></section>
                <section className="crash-subpanel"><div className="crash-subpanel-head"><h3>崩溃模块</h3><small>{selectedModules.length} 个模块</small></div><ModuleTable modules={selectedModules} onDownload={(artifact) => void download("binary", artifact)} /></section>
              </div>
              <dl className="crash-detail-meta"><div><dt>SHA-256</dt><dd><code>{details.sha256}</code></dd></div><div><dt>Server ID</dt><dd><code>{details.server_id || "--"}</code></dd></div><div><dt>游戏目录</dt><dd><code>{details.game_directory || "--"}</code></dd></div><div><dt>用户标识</dt><dd><code>{details.user_id || "--"}</code></dd></div></dl>
            </>
          ) : null}
        </section>
      </div>
    </div>
  );
}

function CrashDatum({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return <div className="crash-datum"><span>{icon}</span><div><small>{label}</small><strong>{value}</strong></div></div>;
}

function AnalysisCard({ title, icon, status, error, children }: { title: string; icon: ReactNode; status?: CrashAnalysisStatus; error?: string; children: ReactNode }) {
  return <section className="crash-analysis-card"><header><h3>{icon}{title}</h3><span className={`crash-status ${statusClass(status)}`}>{statusLabel(status)}</span></header>{error ? <p className="crash-analysis-error">{error}</p> : null}{children}</section>;
}

function ModuleTable({ modules, onDownload }: { modules: CrashModule[]; onDownload: (artifact: string) => void }) {
  if (!modules.length) return <div className="crash-inline-empty">签名未包含模块信息</div>;
  return <div className="crash-module-table-wrap"><table className="crash-module-table" aria-label="崩溃模块"><thead><tr><th>模块</th><th>Debug ID</th><th>符号</th><th>二进制</th></tr></thead><tbody>{modules.map((module) => <tr key={`${module.index}-${module.debug_file}`}><td><strong>{module.debug_file || "--"}</strong><small>{module.platform || "--"} · {module.architecture || "--"}</small></td><td><code>{module.debug_identifier || "--"}</code></td><td><span className="crash-module-state">{module.symbol_artifact ? "已匹配" : "未匹配"}</span></td><td>{module.binary_artifact ? <button type="button" className="icon-btn" aria-label={`下载 ${module.debug_file} 二进制`} onClick={() => onDownload(module.binary_artifact!)}><Download /></button> : <span className="crash-module-state">--</span>}</td></tr>)}</tbody></table></div>;
}
