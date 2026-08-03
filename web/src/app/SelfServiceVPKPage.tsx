import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Clock, LockKeyhole, MapPinned, RefreshCw, ShieldCheck, Upload } from "lucide-react";
import { VPKUploadQueue } from "./VPKUploadQueue";
import { cancelVPKUpload, enqueueVPKUploads, retryVPKUpload, startVPKUploadQueue, type VPKUploadMode, type VPKUploadTask } from "../vpk/uploadQueue";

type Status = { enabled: boolean; password_required: boolean; authorized: boolean; auto_delete: boolean };
type Item = { name: string; size: number; uploaded_at: string; expires_at: string };
type Page = { items: Item[]; total: number; limit: number; offset: number; auto_delete: boolean };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { credentials: "same-origin", headers: { "Content-Type": "application/json", ...(init?.headers || {}) }, ...init });
  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try { const body = await response.json(); message = body?.error?.message || message; } catch {}
    throw Object.assign(new Error(message), { status: response.status });
  }
  return response.status === 204 ? undefined as T : response.json();
}

const bytes = (value: number) => value < 1024 ? `${value} B` : value < 1024 ** 2 ? `${(value / 1024).toFixed(1)} KB` : `${(value / 1024 ** 2).toFixed(1)} MB`;
const date = (value: string) => new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));

export function SelfServiceVPKPage() {
  const [status, setStatus] = useState<Status | null>(null);
  const [authorized, setAuthorized] = useState(false);
  const [password, setPassword] = useState("");
  const [page, setPage] = useState<Page | null>(null);
  const [offset, setOffset] = useState(0);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState<VPKUploadMode>("direct");
  const [tasks, setTasks] = useState<VPKUploadTask[]>([]);
  const [dragActive, setDragActive] = useState(false);

  const loadList = useCallback(async (nextOffset: number) => {
    try {
      const result = await request<Page>(`/api/self-service/vpk?limit=20&offset=${nextOffset}`);
      setPage(result); setOffset(nextOffset); setAuthorized(true); setError("");
    } catch (reason) {
      if ((reason as { status?: number }).status === 401) { setAuthorized(false); setPage(null); return; }
      setError(reason instanceof Error ? reason.message : String(reason));
    }
  }, []);

  useEffect(() => {
    let active = true;
    void request<Status>("/api/self-service/vpk/status").then(async (next) => {
		if (!active) return;
		setStatus(next);
		if (next.enabled && next.authorized) await loadList(0);
    }).catch((reason) => active && setError(reason instanceof Error ? reason.message : String(reason)));
    return () => { active = false; };
  }, [loadList]);

  useEffect(() => {
    if (!authorized) return;
    let stop: (() => void) | undefined;
    void startVPKUploadQueue(setTasks, () => { void loadList(0); }).then((cleanup) => { stop = cleanup; });
    return () => stop?.();
  }, [authorized, loadList]);

  async function unlock(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError("");
    try { await request<void>("/api/self-service/vpk/authorize", { method: "POST", body: JSON.stringify({ password }) }); await loadList(0); setPassword(""); }
    catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { setBusy(false); }
  }

  if (!status && !error) return <main className="self-vpk-state"><RefreshCw className="spin" /><p>正在读取自助传图状态</p></main>;
  if (status && !status.enabled) return <main className="self-vpk-state"><MapPinned /><h1>自助传图暂未开放</h1><p>请联系服务器管理员开启此入口。</p></main>;
  if (status?.password_required && !authorized) return <main className="self-vpk-state"><LockKeyhole /><h1>自助传图</h1><p>输入访问密码以查看并上传地图。</p><form className="self-vpk-unlock" onSubmit={unlock}><label>访问密码<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoFocus /></label><button type="submit" disabled={busy}>{busy ? "验证中" : "进入自助传图"}</button></form>{error ? <p role="alert" className="error-text">{error}</p> : null}</main>;

  return <main className="self-vpk-page">
    <header className="self-vpk-header"><div><span><ShieldCheck /> 社区入口</span><h1>自助传图</h1><p>上传完成后会进入服务器共享 VPK 仓库。</p></div><button type="button" className="icon-button" title="刷新列表" aria-label="刷新自助 VPK 列表" onClick={() => void loadList(offset)}><RefreshCw /></button></header>
    {error ? <p role="alert" className="self-vpk-error">{error}</p> : null}
    <section className="self-vpk-upload" aria-labelledby="self-vpk-upload-title"><div className="self-vpk-section-head"><div><h2 id="self-vpk-upload-title">上传地图</h2><p>同名文件会被拒绝，不会覆盖仓库内容。</p></div><div className="self-vpk-mode" aria-label="VPK 上传模式"><button type="button" aria-pressed={mode === "direct"} onClick={() => setMode("direct")}>直接上传</button><button type="button" aria-pressed={mode === "clean"} onClick={() => setMode("clean")}>上传前清理</button></div></div><label className={`self-vpk-drop ${dragActive ? "dragging" : ""}`} onDragEnter={(event) => { event.preventDefault(); setDragActive(true); }} onDragOver={(event) => { event.preventDefault(); setDragActive(true); }} onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragActive(false); }} onDrop={(event) => { event.preventDefault(); setDragActive(false); const files = Array.from(event.dataTransfer.files); if (files.length) void enqueueVPKUploads(files.map((file) => ({ file, mode }))); }}><Upload /><strong>选择 VPK 文件</strong><small>支持多选、分片上传与断点恢复</small><input aria-label="选择 VPK 文件" type="file" accept=".vpk" multiple onChange={(event) => { const files = Array.from(event.target.files || []); if (files.length) void enqueueVPKUploads(files.map((file) => ({ file, mode }))); event.target.value = ""; }} /></label><VPKUploadQueue tasks={tasks} onRetry={(task) => void retryVPKUpload(task)} onCancel={(task) => void cancelVPKUpload(task)} /></section>
    <section className="self-vpk-list" aria-labelledby="self-vpk-list-title"><div className="self-vpk-section-head"><div><h2 id="self-vpk-list-title">已上传 VPK</h2><p>{page?.total || 0} 个自助上传文件</p></div>{page && !page.auto_delete ? <span className="self-vpk-paused"><Clock /> 自动删除已暂停</span> : null}</div>{page?.items.length ? <div className="self-vpk-table"><div className="self-vpk-row self-vpk-row-head"><span>文件</span><span>大小</span><span>上传时间</span><span>到期时间</span></div>{page.items.map((item) => <div className="self-vpk-row" key={item.name}><strong>{item.name}</strong><span>{bytes(item.size)}</span><time>{date(item.uploaded_at)}</time><time>{date(item.expires_at)}{!page.auto_delete ? <small>已暂停</small> : null}</time></div>)}</div> : <div className="self-vpk-empty"><MapPinned /><p>还没有通过此入口上传的 VPK</p></div>}<footer className="self-vpk-pagination"><button disabled={!page || offset === 0} onClick={() => void loadList(Math.max(0, offset - 20))}>上一页</button><span>{page ? `${Math.floor(offset / 20) + 1} / ${Math.max(1, Math.ceil(page.total / 20))}` : "1 / 1"}</span><button disabled={!page || offset + 20 >= page.total} onClick={() => void loadList(offset + 20)}>下一页</button></footer></section>
  </main>;
}
