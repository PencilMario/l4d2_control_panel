import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type RefObject,
} from "react";
import { sha256 } from "@noble/hashes/sha2.js";
import {
  ArchiveRestore,
  ChevronDown,
  ChevronRight,
  Download,
  Edit3,
  File,
  FilePlus2,
  Folder,
  FolderPlus,
  History,
  Menu,
  Move,
  RefreshCw,
  Save,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import { api, type Job } from "../api/client";
import type { Instance } from "./App";

export type PrivateEntry = {
  path: string;
  kind: "file" | "directory";
  hash?: string;
  size?: number;
  updated_at: string;
};

export type PrivateDiff = {
  changes: Array<{ path: string; kind: "added" | "modified" | "deleted" }>;
  summary: { added: number; modified: number; deleted: number };
};

type PrivateSnapshot = {
  id: string;
  applied_at: string;
  summary: PrivateDiff["summary"];
};

type Props = {
  instances: Instance[];
  queue: (path: string, body: unknown) => Promise<void>;
  queueAndWait?: (path: string, body: unknown) => Promise<Job>;
};
type PrivateVersion = { path: string; size: number; hash?: string; updated_at?: string };
type ActiveUpload = { id: string; instanceID: string; path: string; size: number; offset: number; fingerprint: string; file?: File };
type UploadIdentity = ["private-upload-v2", string, string, string, number, number, string];

const EMPTY_DIFF: PrivateDiff = {
  changes: [],
  summary: { added: 0, modified: 0, deleted: 0 },
};
const UPLOAD_CHUNK_SIZE = 4 * 1024 * 1024;
const EDITOR_LIMIT = 1 << 20;
const TEXT_EXTENSIONS = new Set([
  "cfg",
  "txt",
  "json",
  "xml",
  "ini",
  "md",
  "log",
  "sp",
  "inc",
  "kv",
  "vdf",
  "nut",
  "smc",
]);

const encodeRelativePath = (path: string) =>
  path.split("/").map(encodeURIComponent).join("/");
const basename = (path: string) => path.split("/").at(-1) || path;
const parentPath = (path: string) => path.split("/").slice(0, -1).join("/");
const isTextFile = (path: string) => {
  const name = basename(path);
  const extension = name.includes(".") ? name.split(".").at(-1)!.toLowerCase() : "";
  return TEXT_EXTENSIONS.has(extension) || !name.includes(".");
};
const errorMessage = (reason: unknown) =>
  reason instanceof Error ? reason.message : String(reason);
const formatBytes = (size = 0) =>
  size < 1024 ? `${size} B` : `${(size / 1024).toFixed(1)} KiB`;
const uploadFingerprint = (
  instanceID: string,
  path: string,
  file: File,
  hash: string,
) => JSON.stringify([
  "private-upload-v2",
  instanceID,
  path,
  file.name,
  file.size,
  file.lastModified,
  hash,
] satisfies UploadIdentity);
const hasCurrentUploadFingerprint = (upload: ActiveUpload) => {
  try {
    const identity = JSON.parse(upload.fingerprint) as unknown;
    return Array.isArray(identity) &&
      identity.length === 7 &&
      identity[0] === "private-upload-v2" &&
      identity[1] === upload.instanceID &&
      identity[2] === upload.path &&
      identity[3] === basename(upload.path) &&
      identity[4] === upload.size &&
      typeof identity[5] === "number" &&
      typeof identity[6] === "string";
  } catch {
    return false;
  }
};

function buildChildren(entries: PrivateEntry[]) {
  const children = new Map<string, PrivateEntry[]>();
  for (const entry of entries) {
    const parent = parentPath(entry.path);
    const siblings = children.get(parent) || [];
    siblings.push(entry);
    children.set(parent, siblings);
  }
  for (const siblings of children.values()) {
    siblings.sort((a, b) =>
      a.kind === b.kind
        ? a.path.localeCompare(b.path)
        : a.kind === "directory"
          ? -1
          : 1,
    );
  }
  return children;
}

export function PrivateFilesPage({ instances, queue, queueAndWait }: Props) {
  const [instanceID, setInstanceID] = useState(instances[0]?.id ?? "");
  const [entries, setEntries] = useState<PrivateEntry[]>([]);
  const [diff, setDiff] = useState<PrivateDiff>(EMPTY_DIFF);
  const [snapshots, setSnapshots] = useState<PrivateSnapshot[]>([]);
  const [expanded, setExpanded] = useState(() => new Set<string>());
  const [selectedPath, setSelectedPath] = useState("");
  const [editor, setEditor] = useState("");
  const [editing, setEditing] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  const [uploadStatus, setUploadStatus] = useState("");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [snapshotsOpen, setSnapshotsOpen] = useState(false);
  const [diffOpen, setDiffOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [activeUpload, setActiveUpload] = useState<ActiveUpload | null>(null);
  const [history, setHistory] = useState<{ path: string; versions: PrivateVersion[] } | null>(null);
  const drawerRef = useRef<HTMLDivElement>(null);
  const drawerTriggerRef = useRef<HTMLButtonElement>(null);
  const snapshotsRef = useRef<HTMLElement>(null);
  const snapshotsTriggerRef = useRef<HTMLButtonElement>(null);
  const historyRef = useRef<HTMLElement>(null);
  const historyTriggerRef = useRef<HTMLButtonElement>(null);
  const instanceIDRef = useRef(instanceID);
  const loadRef = useRef<{ generation: number; controller?: AbortController }>({ generation: 0 });
  instanceIDRef.current = instanceID;

  useEffect(() => {
    if (!instances.some((item) => item.id === instanceID)) {
      setInstanceID(instances[0]?.id ?? "");
    }
  }, [instanceID, instances]);

  const reload = useCallback(async () => {
    const owner = instanceID;
    const generation = ++loadRef.current.generation;
    loadRef.current.controller?.abort();
    const controller = new AbortController();
    loadRef.current.controller = controller;
    if (!owner) {
      setEntries([]);
      setDiff(EMPTY_DIFF);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [nextEntries, nextDiff, nextSnapshots] = await Promise.all([
        api<PrivateEntry[]>(`/api/instances/${owner}/private/tree`, { signal: controller.signal }),
        api<PrivateDiff>(`/api/instances/${owner}/private/diff`, { signal: controller.signal }),
        api<PrivateSnapshot[]>(`/api/instances/${owner}/private/snapshots`, { signal: controller.signal }),
      ]);
      if (generation !== loadRef.current.generation || owner !== instanceIDRef.current) return;
      setEntries(nextEntries);
      setDiff(nextDiff);
      setSnapshots(nextSnapshots);
      setSelectedPath((current) =>
        current && nextEntries.some((entry) => entry.path === current)
          ? current
          : "",
      );
    } catch (reason) {
      if (!controller.signal.aborted && generation === loadRef.current.generation) setError(errorMessage(reason));
    } finally {
      if (generation === loadRef.current.generation) setLoading(false);
    }
  }, [instanceID]);

  useEffect(() => {
    setEntries([]);
    setDiff(EMPTY_DIFF);
    setSnapshots([]);
    setSelectedPath("");
    setEditor("");
    setEditing(false);
    setError("");
    setStatus("");
    setUploadStatus("");
    void reload();
    return () => loadRef.current.controller?.abort();
  }, [reload]);

  useEffect(() => {
    if (!instanceID) return;
    setActiveUpload(null);
    const stored = localStorage.getItem(`private-upload:${instanceID}`);
    if (stored) {
      try {
        const parsed = JSON.parse(stored) as ActiveUpload;
        if (parsed.instanceID === instanceID && hasCurrentUploadFingerprint(parsed)) {
          setActiveUpload(parsed);
          setUploadStatus("上传等待恢复 · 请重新选择原文件");
        } else {
          localStorage.removeItem(`private-upload:${instanceID}`);
        }
      } catch {
        localStorage.removeItem(`private-upload:${instanceID}`);
      }
    }
  }, [instanceID]);
  useEffect(() => {
    if (!instanceID) return;
    if (activeUpload?.instanceID === instanceID) {
      const { file: _file, ...metadata } = activeUpload;
      localStorage.setItem(`private-upload:${instanceID}`, JSON.stringify(metadata));
    } else {
      localStorage.removeItem(`private-upload:${instanceID}`);
    }
  }, [activeUpload, instanceID]);

  const closeDrawer = useCallback(() => setDrawerOpen(false), []);
  const closeSnapshots = useCallback(() => setSnapshotsOpen(false), []);
  const closeHistory = useCallback(() => setHistory(null), []);
  useModalFocus(drawerOpen, drawerRef, drawerTriggerRef, closeDrawer);
  useModalFocus(snapshotsOpen, snapshotsRef, snapshotsTriggerRef, closeSnapshots);
  useModalFocus(Boolean(history), historyRef, historyTriggerRef, closeHistory);

  const run = useCallback(async (operation: () => Promise<void>) => {
    if (busy) return;
    setBusy(true);
    setError("");
    setStatus("");
    try {
      await operation();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }, [busy]);

  const selectEntry = useCallback(
    (entry: PrivateEntry) => {
      setSelectedPath(entry.path);
      setEditing(false);
      setEditor("");
      if (entry.kind === "directory") {
        setExpanded((current) => {
          const next = new Set(current);
          next.has(entry.path) ? next.delete(entry.path) : next.add(entry.path);
          return next;
        });
      }
    },
    [],
  );

  const editFile = async (path: string) => {
    const response = await fetch(`/api/instances/${instanceID}/private/file/${encodeRelativePath(path)}`, { credentials: "same-origin" });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.byteLength > EDITOR_LIMIT) throw new Error("文件超过文本编辑大小限制");
    if (bytes.includes(0)) throw new Error("该文件不是可编辑的 UTF-8 文本");
    let text: string;
    try { text = new TextDecoder("utf-8", { fatal: true }).decode(bytes); }
    catch { throw new Error("该文件不是可编辑的 UTF-8 文本"); }
    setSelectedPath(path);
    setEditor(text);
    setEditing(true);
    setDrawerOpen(false);
  };

  const showFileHistory = async (path: string) => {
    const versions = await api<PrivateVersion[]>(`/api/instances/${instanceID}/private/history/${encodeRelativePath(path)}`);
    setHistory({ path, versions });
  };

  const saveText = async () => {
    await api(`/api/instances/${instanceID}/private/${encodeRelativePath(selectedPath)}`, {
      method: "PUT",
      headers: { "Content-Type": "text/plain; charset=utf-8" },
      body: editor,
    });
    setStatus("文件已保存到暂存区");
    await reload();
  };

  const makeDirectory = async () => {
    const path = window.prompt("新目录相对路径", selectedPath && parentPath(selectedPath));
    if (!path) return;
    await api(`/api/instances/${instanceID}/private/directories`, {
      method: "POST",
      body: JSON.stringify({ path }),
    });
    setStatus("目录已创建到暂存区");
    await reload();
  };

  const makeFile = async () => {
    const path = window.prompt("新文件相对路径", "cfg/new.cfg");
    if (!path) return;
    await api(`/api/instances/${instanceID}/private/${encodeRelativePath(path)}`, {
      method: "PUT",
      headers: { "Content-Type": "text/plain; charset=utf-8" },
      body: "",
    });
    setSelectedPath(path);
    setEditor("");
    setEditing(true);
    setStatus("文件已创建到暂存区");
    await reload();
  };

  const moveEntry = async (path: string) => {
    const to = window.prompt("移动到相对路径", path);
    if (!to || to === path) return;
    const overwrite = window.confirm("目标存在时覆盖？");
    await api(`/api/instances/${instanceID}/private/move`, {
      method: "POST",
      body: JSON.stringify({ from: path, to, overwrite, confirm: overwrite }),
    });
    setSelectedPath(to);
    setStatus("路径已移动到暂存区");
    await reload();
  };

  const deleteEntry = async (entry: PrivateEntry) => {
    const label = entry.kind === "directory" ? "目录及其中全部文件" : "文件";
    if (!window.confirm(`删除${label} ${entry.path}？`)) return;
    await api(
      `/api/instances/${instanceID}/private/file/${encodeRelativePath(entry.path)}?confirm=true`,
      { method: "DELETE" },
    );
    setEditing(false);
    setSelectedPath("");
    setStatus("路径已从暂存区删除");
    await reload();
  };

  const uploadFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    await run(async () => {
      const owner = instanceID;
      const selectedDirectory = selected?.kind === "directory" ? selected.path : "";
      const target = selectedDirectory ? `${selectedDirectory}/${file.name}` : file.name;
      const hasher = sha256.create();
      for (let offset = 0; offset < file.size; offset += UPLOAD_CHUNK_SIZE) {
        const chunk = file.slice(offset, Math.min(offset + UPLOAD_CHUNK_SIZE, file.size));
        hasher.update(new Uint8Array(await chunk.arrayBuffer()));
        if (owner !== instanceIDRef.current) return;
      }
      const hash = [...hasher.digest()].map((value) => value.toString(16).padStart(2, "0")).join("");
      const fingerprint = uploadFingerprint(owner, target, file, hash);
      if (activeUpload && activeUpload.fingerprint === fingerprint) {
        setActiveUpload({ ...activeUpload, file });
        await resumeUpload({ ...activeUpload, file });
        return;
      }
      setUploadStatus("准备上传 · 0%");
      const session = await api<{ id: string; offset: number }>(
        `/api/instances/${owner}/private/uploads`,
        {
          method: "POST",
          body: JSON.stringify({ path: target, size: file.size, sha256: hash }),
        },
      );
      const upload = { id: session.id, instanceID: owner, path: target, size: file.size, offset: session.offset || 0, fingerprint, file };
      setActiveUpload(upload);
      await continueUpload(upload);
    });
  };

  const continueUpload = async (upload: ActiveUpload) => {
      const owner = upload.instanceID;
      if (owner !== instanceIDRef.current) return;
      const file = upload.file;
      if (!file) {
        setUploadStatus("上传等待恢复 · 请重新选择原文件");
        return;
      }
      let offset = upload.offset;
      while (offset < file.size) {
        const end = Math.min(offset + UPLOAD_CHUNK_SIZE, file.size);
        const response = await fetch(
          `/api/instances/${owner}/private/uploads/${upload.id}`,
          {
            method: "PATCH",
            credentials: "same-origin",
            headers: {
              "Content-Type": "application/offset+octet-stream",
              "Upload-Offset": String(offset),
            },
            body: file.slice(offset, end),
          },
        );
        if (!response.ok) {
          if (owner === instanceIDRef.current) {
            setActiveUpload({ ...upload, offset });
            setUploadStatus(`上传可恢复 · ${offset}/${file.size} B · 会话 ${upload.id}`);
          }
          throw new Error(`上传中断 · HTTP ${response.status}`);
        }
        offset = Number(response.headers.get("Upload-Offset") || end);
        if (owner !== instanceIDRef.current) return;
        setUploadStatus(`正在上传 · ${Math.round((offset / file.size) * 100)}%`);
      }
      await api(
        `/api/instances/${owner}/private/uploads/${upload.id}/complete`,
        { method: "POST", body: "{}" },
      );
      setUploadStatus("上传完成 · 100%");
      setActiveUpload(null);
      await reload();
  };

  const resumeUpload = async (upload = activeUpload) => {
    if (!upload || upload.instanceID !== instanceIDRef.current) return;
    const owner = upload.instanceID;
    const recovered = await api<{ offset: number }>(`/api/instances/${owner}/private/uploads/${upload.id}`);
    if (owner !== instanceIDRef.current) return;
    const next = { ...upload, offset: recovered.offset };
    setActiveUpload(next);
    await continueUpload(next);
  };

  const restoreSnapshot = async (snapshot: PrivateSnapshot) => {
    if (!window.confirm("恢复快照将替换当前暂存工作区，继续？")) return;
    await api(
      `/api/instances/${instanceID}/private/snapshots/${encodeURIComponent(snapshot.id)}/restore`,
      { method: "POST", body: JSON.stringify({ confirm: true }) },
    );
    setSnapshotsOpen(false);
    setStatus("快照已恢复到暂存区");
    await reload();
  };

  const children = useMemo(() => buildChildren(entries), [entries]);
  const selected = entries.find((entry) => entry.path === selectedPath);
  const hasChanges = diff.changes.length > 0;
  const totalChanges = diff.summary.added + diff.summary.modified + diff.summary.deleted;

  const tree = (
    <PrivateTree
      children={children}
      expanded={expanded}
      selectedPath={selectedPath}
      disabled={busy}
      onSelect={selectEntry}
      onEdit={(path) => void run(() => editFile(path))}
      onMove={(path) => void run(() => moveEntry(path))}
      onDelete={(entry) => void run(() => deleteEntry(entry))}
    />
  );

  return (
    <section className="private-files-page" aria-labelledby="private-files-title">
      <div className="private-page-head">
        <div>
          <p className="eyebrow">INSTANCE / PRIVATE WORKSPACE</p>
          <h2 id="private-files-title">私有文件</h2>
        </div>
        <label>
          目标实例
          <select value={instanceID} onChange={(event) => setInstanceID(event.target.value)}>
            {instances.map((item) => (
              <option key={item.id} value={item.id}>{item.name}</option>
            ))}
          </select>
        </label>
      </div>

      <div className="private-toolbar" role="toolbar" aria-label="私有文件工具栏">
        <label className="private-icon-button" title="上传文件">
          <Upload aria-hidden="true" />
          <span>上传</span>
          <input aria-label="上传文件" type="file" onChange={uploadFile} disabled={!instanceID || busy} />
        </label>
        <button title="新建文件" onClick={() => void run(makeFile)} disabled={!instanceID || busy}><FilePlus2 />新建文件</button>
        <button title="新建目录" onClick={() => void run(makeDirectory)} disabled={!instanceID || busy}><FolderPlus />新建目录</button>
        <button title="刷新" onClick={() => void reload()} disabled={!instanceID || loading}><RefreshCw />刷新</button>
        <button ref={snapshotsTriggerRef} title="历史快照" onClick={() => setSnapshotsOpen(true)} disabled={!instanceID || busy}><History />历史快照</button>
        <button
          ref={drawerTriggerRef}
          className="private-tree-trigger"
          aria-controls="private-tree-drawer"
          aria-expanded={drawerOpen}
          onClick={() => setDrawerOpen(true)}
        ><Menu />打开文件树</button>
      </div>

      {loading ? <div className="operation-status" role="status">正在读取私有文件…</div> : null}
      {error ? <div className="error" role="alert">{error}<button onClick={() => void reload()}>重试</button></div> : null}
      {status ? <div className="operation-status" role="status">{status}</div> : null}
      {uploadStatus ? <div className="operation-status" role="status">{uploadStatus}</div> : null}
      {activeUpload ? <button className="private-resume-upload" disabled={busy || !activeUpload.file} onClick={() => void run(() => resumeUpload())}>恢复上传</button> : null}

      <div className="private-files-layout">
        <aside className="private-tree-pane" aria-label="私有文件目录">{tree}</aside>
        <div className="private-workspace">
          {!instanceID ? <div className="empty">暂无可管理实例</div> : null}
          {instanceID && !loading && entries.length === 0 ? <div className="empty">该实例的私有工作区为空</div> : null}
          {selected?.kind === "file" && !editing ? (
            <div className="private-file-preview">
              <File />
              <div><b>{selected.path}</b><small>{formatBytes(selected.size)} · {(selected.hash || "无校验值").slice(0, 12)}</small></div>
              <div className="private-file-actions">
                <a title="下载" aria-label={`下载 ${basename(selected.path)}`} href={`/api/instances/${instanceID}/private/file/${encodeRelativePath(selected.path)}`} download><Download /></a>
                {isTextFile(selected.path) ? <button title="编辑" aria-label={`编辑 ${basename(selected.path)}`} onClick={() => void run(() => editFile(selected.path))}><Edit3 /></button> : null}
                <button ref={historyTriggerRef} title="历史" aria-label={`历史 ${basename(selected.path)}`} disabled={busy} onClick={() => void run(() => showFileHistory(selected.path))}><History /></button>
                <button title="移动" aria-label={`移动 ${basename(selected.path)}`} disabled={busy} onClick={() => void run(() => moveEntry(selected.path))}><Move /></button>
                <button title="删除" aria-label={`删除 ${basename(selected.path)}`} disabled={busy} className="danger" onClick={() => void run(() => deleteEntry(selected))}><Trash2 /></button>
              </div>
            </div>
          ) : null}
          {editing ? (
            <div className="private-editor">
              <div className="private-editor-head"><b>{selectedPath}</b><span>UTF-8</span></div>
              <label htmlFor="private-editor-content">文件内容</label>
              <textarea id="private-editor-content" value={editor} onChange={(event) => setEditor(event.target.value)} spellCheck={false} />
              <button className="create" disabled={busy} onClick={() => void run(saveText)}><Save />保存到暂存区</button>
            </div>
          ) : null}
          {!selected && entries.length > 0 ? <div className="empty">选择目录或文件</div> : null}
        </div>
      </div>

      <div
        id="private-tree-drawer"
        ref={drawerRef}
        className={`private-tree-drawer ${drawerOpen ? "open" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-label="私有文件目录"
        aria-hidden={!drawerOpen}
      >
        <div className="private-drawer-head"><b>私有文件目录</b><button aria-label="关闭文件树" onClick={() => setDrawerOpen(false)}><X /></button></div>
        {drawerOpen ? tree : null}
      </div>

      {snapshotsOpen ? (
        <div className="private-snapshot-backdrop" role="presentation">
          <section ref={snapshotsRef} className="private-snapshot-dialog" role="dialog" aria-modal="true" aria-labelledby="private-snapshot-title">
            <div className="private-drawer-head"><h3 id="private-snapshot-title">历史快照</h3><button aria-label="关闭历史快照" onClick={() => setSnapshotsOpen(false)}><X /></button></div>
            {snapshots.map((snapshot) => (
              <div className="private-snapshot-row" key={snapshot.id}>
                <div><b>{new Date(snapshot.applied_at).toLocaleString("zh-CN")}</b><small>+{snapshot.summary.added} / ~{snapshot.summary.modified} / -{snapshot.summary.deleted}</small></div>
                <button disabled={busy} aria-label={`恢复 ${new Date(snapshot.applied_at).toLocaleDateString("zh-CN")}`} onClick={() => void run(() => restoreSnapshot(snapshot))}><ArchiveRestore />恢复</button>
              </div>
            ))}
            {!snapshots.length ? <div className="empty">暂无应用快照</div> : null}
          </section>
        </div>
      ) : null}

      {history ? (
        <div className="private-snapshot-backdrop" role="presentation">
          <section ref={historyRef} className="private-snapshot-dialog" role="dialog" aria-modal="true" aria-labelledby="private-history-title">
            <div className="private-drawer-head"><h3 id="private-history-title">文件历史 · {history.path}</h3><button aria-label="关闭文件历史" onClick={() => setHistory(null)}><X /></button></div>
            {history.versions.map((version, index) => <div className="private-snapshot-row" key={`${version.path}-${index}`}><div><b>{version.path}</b><small>{formatBytes(version.size)} · {(version.hash || "").slice(0, 12)}</small></div></div>)}
            {!history.versions.length ? <div className="empty">暂无文件历史</div> : null}
          </section>
        </div>
      ) : null}

      {diffOpen && hasChanges ? (
        <div className="private-diff-panel" role="region" aria-label="暂存差异">
          {diff.changes.map((change) => <span key={change.path}><b>{change.kind}</b>{change.path}</span>)}
        </div>
      ) : null}
      <footer className="private-change-bar" aria-label="暂存更改状态">
        <button className="private-diff-toggle" disabled={!hasChanges} title="查看差异" onClick={() => setDiffOpen((open) => !open)}>
          {hasChanges
            ? totalChanges === 1 && diff.summary.modified === 1
              ? "1 项修改未应用"
              : `${totalChanges} 项更改未应用`
            : "工作区与已应用版本一致"}
        </button>
        <div className="private-change-counts"><span>新增 {diff.summary.added}</span><span>修改 {diff.summary.modified}</span><span>删除 {diff.summary.deleted}</span></div>
        <button className="create" disabled={!hasChanges || !instanceID || busy} onClick={() => void run(async () => {
          if (queueAndWait) {
            const terminal = await queueAndWait(`/api/instances/${instanceID}/private/apply`, {});
            if (terminal.Status !== "succeeded") throw new Error(terminal.Error || "应用任务失败");
            await reload();
            setStatus("私有文件已应用");
          } else {
            await queue(`/api/instances/${instanceID}/private/apply`, {});
            setStatus("应用任务已加入队列");
          }
        })}>应用更改</button>
      </footer>
    </section>
  );
}

function PrivateTree({
  children,
  expanded,
  selectedPath,
  disabled,
  onSelect,
  onEdit,
  onMove,
  onDelete,
}: {
  children: Map<string, PrivateEntry[]>;
  expanded: Set<string>;
  selectedPath: string;
  disabled: boolean;
  onSelect: (entry: PrivateEntry) => void;
  onEdit: (path: string) => void;
  onMove: (path: string) => void;
  onDelete: (entry: PrivateEntry) => void;
}) {
  const renderLevel = (parent: string, level: number) =>
    (children.get(parent) || []).map((entry) => {
      const directory = entry.kind === "directory";
      const open = expanded.has(entry.path);
      const name = basename(entry.path);
      return (
        <div key={entry.path} role="none">
          <div className={`private-tree-row ${selectedPath === entry.path ? "selected" : ""}`} role="none">
            <button className="private-tree-select" role="treeitem" aria-label={name} aria-level={level} aria-expanded={directory ? open : undefined} aria-selected={selectedPath === entry.path} onClick={() => onSelect(entry)}>
              {directory ? (open ? <ChevronDown /> : <ChevronRight />) : <span className="tree-spacer" />}
              {directory ? <Folder /> : <File />}
              <span>{name}</span>
            </button>
            <span className="private-tree-actions">
              {!directory && isTextFile(entry.path) ? <button disabled={disabled} title="编辑" aria-label={`编辑 ${name}`} onClick={() => onEdit(entry.path)}><Edit3 /></button> : null}
              <button disabled={disabled} title="移动" aria-label={`移动 ${name}`} onClick={() => onMove(entry.path)}><Move /></button>
              <button disabled={disabled} title="删除" aria-label={`删除 ${name}`} onClick={() => onDelete(entry)}><Trash2 /></button>
            </span>
          </div>
          {directory && open ? <div role="group">{renderLevel(entry.path, level + 1)}{(children.get(entry.path) || []).length === 0 ? <div className="private-tree-empty">空目录</div> : null}</div> : null}
        </div>
      );
    });
  return <div className="private-tree" role="tree" aria-label="私有文件树">{renderLevel("", 1)}</div>;
}

function useModalFocus(
  open: boolean,
  containerRef: RefObject<HTMLElement | null>,
  triggerRef: RefObject<HTMLElement | null>,
  close: () => void,
) {
  useEffect(() => {
    if (!open) return;
    const focusables = () => Array.from(containerRef.current?.querySelectorAll<HTMLElement>("button:not([disabled]), [href], input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex='-1'])") || []);
    focusables()[0]?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); close(); return; }
      if (event.key !== "Tab") return;
      const items = focusables();
      if (!items.length) return;
      const first = items[0];
      const last = items.at(-1)!;
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => { document.removeEventListener("keydown", onKeyDown); triggerRef.current?.focus(); };
  }, [close, containerRef, open, triggerRef]);
}
