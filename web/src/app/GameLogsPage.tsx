import { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronDown, ChevronRight, Download, File, Folder, Menu, RefreshCw, X } from 'lucide-react';
import { api as defaultApi, apiBlob as defaultBlobApi } from '../api/client';
import { HighlightedLog } from './logHighlight';

export { DISPLAY_PREVIEW_LIMIT, truncateForDisplay } from './logHighlight';

export const PREVIEW_LIMIT = 10 * 1024 * 1024;

type LogKind = 'game' | 'sourcemod';
type LogInstance = { id: string; name: string };
type Entry = { kind: LogKind; path: string; size: number; modified_at: string };
type Preview = { text: string; truncated?: boolean; size?: number; modified_at?: string };
type Node = { name: string; path: string; directory: boolean; children: Node[] };

const LOG_GROUPS: Array<{ kind: LogKind; label: string }> = [
  { kind: 'game', label: '游戏日志' },
  { kind: 'sourcemod', label: 'SourceMod 日志' },
];

function build(entries: Entry[], kind: LogKind) {
  const root: Node = { name: kind, path: kind, directory: true, children: [] };
  for (const entry of entries.filter((item) => item.kind === kind)) {
    const parts = entry.path.split('/').filter(Boolean);
    let parent = root;
    for (let index = 0; index < parts.length; index++) {
      const path = parts.slice(0, index + 1).join('/');
      let child = parent.children.find((item) => item.name === parts[index]);
      if (!child) {
        child = { name: parts[index], path, directory: index < parts.length - 1, children: [] };
        parent.children.push(child);
      }
      parent = child;
    }
  }
  const sort = (node: Node) => {
    node.children.sort((left, right) => left.directory === right.directory
      ? left.name.localeCompare(right.name)
      : left.directory ? -1 : 1);
    node.children.forEach(sort);
  };
  sort(root);
  return root;
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`;
  return `${(size / 1024).toFixed(1)} KiB`;
}

export function GameLogsPage({
  instances,
  api = defaultApi,
  blobApi = defaultBlobApi,
}: {
  instances: LogInstance[];
  api?: typeof defaultApi;
  blobApi?: typeof defaultBlobApi;
}) {
  const [instanceID, setInstanceID] = useState(instances[0]?.id ?? '');
  const [entries, setEntries] = useState<Entry[]>([]);
  const [selected, setSelected] = useState<Entry | null>(null);
  const [preview, setPreview] = useState<Preview>();
  const [loading, setLoading] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [error, setError] = useState('');
  const [previewError, setPreviewError] = useState('');
  const [downloadError, setDownloadError] = useState('');
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [rotated, setRotated] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const treeAbort = useRef<AbortController | null>(null);
  const previewAbort = useRef<AbortController | null>(null);
  const drawerRef = useRef<HTMLDivElement>(null);
  const drawerTriggerRef = useRef<HTMLButtonElement>(null);
  const treeSequence = useRef(0);
  const previewSequence = useRef(0);

  useEffect(() => {
    if (!instances.some((item) => item.id === instanceID)) {
      setInstanceID(instances[0]?.id ?? '');
    }
  }, [instanceID, instances]);

  const loadPreview = async (entry: Entry) => {
    previewAbort.current?.abort();
    const controller = new AbortController();
    const sequence = ++previewSequence.current;
    previewAbort.current = controller;
    setPreview(undefined);
    setRotated(false);
    setPreviewError('');
    setPreviewLoading(true);
    try {
      const data = await api<Preview>(`/api/instances/${instanceID}/game-logs/preview?kind=${entry.kind}&path=${encodeURIComponent(entry.path)}`, { signal: controller.signal });
      if (sequence === previewSequence.current) setPreview(data);
    } catch (cause) {
      const value = cause as Error;
      if (value.name !== 'AbortError' && sequence === previewSequence.current) {
        setRotated(/404|not found/i.test(value.message));
        setPreviewError(value.message || '读取日志失败');
      }
    } finally {
      if (sequence === previewSequence.current) setPreviewLoading(false);
    }
  };

  const loadTree = async (refreshPreview = false, currentSelection: Entry | null = selected) => {
    treeAbort.current?.abort();
    const controller = new AbortController();
    const sequence = ++treeSequence.current;
    treeAbort.current = controller;
    if (!instanceID) {
      setEntries([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    setError('');
    try {
      const data = await api<Entry[]>(`/api/instances/${instanceID}/game-logs/tree`, { signal: controller.signal });
      if (sequence !== treeSequence.current) return;
      setEntries(data);
      if (currentSelection) {
        const current = data.find((item) => item.kind === currentSelection.kind && item.path === currentSelection.path) || null;
        setSelected(current);
        if (refreshPreview && current) await loadPreview(current);
      }
    } catch (cause) {
      const value = cause as Error;
      if (value.name !== 'AbortError' && sequence === treeSequence.current) setError(value.message || '读取日志目录失败');
    } finally {
      if (sequence === treeSequence.current) setLoading(false);
    }
  };

  useEffect(() => {
    treeAbort.current?.abort();
    previewAbort.current?.abort();
    treeSequence.current++;
    previewSequence.current++;
    setEntries([]);
    setSelected(null);
    setPreview(undefined);
    setLoading(false);
    setPreviewLoading(false);
    setError('');
    setPreviewError('');
    setDownloadError('');
    setRotated(false);
    void loadTree(false, null);
    return () => {
      treeAbort.current?.abort();
      previewAbort.current?.abort();
    };
  }, [instanceID]);

  const closeDrawer = useCallback(() => setDrawerOpen(false), []);

  useEffect(() => {
    if (!drawerOpen) return;
    const focusables = () => Array.from(drawerRef.current?.querySelectorAll<HTMLElement>('button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])') ?? []);
    focusables()[0]?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeDrawer();
        return;
      }
      if (event.key !== 'Tab') return;
      const items = focusables();
      if (!items.length) return;
      const first = items[0];
      const last = items.at(-1)!;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      drawerTriggerRef.current?.focus();
    };
  }, [closeDrawer, drawerOpen]);

  const select = (entry: Entry) => {
    setSelected(entry);
    setDownloadError('');
    if (drawerOpen) closeDrawer();
    void loadPreview(entry);
  };

  const download = async () => {
    if (!selected) return;
    setDownloadError('');
    try {
      const blob = await blobApi(`/api/instances/${instanceID}/game-logs/download?kind=${selected.kind}&path=${encodeURIComponent(selected.path)}`);
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = selected.path.split('/').pop() || 'download.log';
      anchor.click();
      URL.revokeObjectURL(url);
    } catch (cause) {
      setDownloadError((cause as Error).message || '下载日志失败');
    }
  };

  const renderNode = (node: Node, kind: LogKind, depth = 1): React.ReactNode => {
    const key = `${kind}/${node.path}`;
    if (node.directory) {
      const expanded = Boolean(open[key]);
      return <div key={key} role="none">
        <div className="private-tree-row" role="none">
          <button className="private-tree-select" role="treeitem" aria-label={`Toggle ${key}`} aria-level={depth} aria-expanded={expanded} onClick={() => setOpen((value) => ({ ...value, [key]: !value[key] }))}>
            {expanded ? <ChevronDown /> : <ChevronRight />}<Folder /><span>{node.name}</span>
          </button>
        </div>
        {expanded ? <div role="group">{node.children.map((child) => renderNode(child, kind, depth + 1))}</div> : null}
      </div>;
    }
    const entry = entries.find((item) => item.kind === kind && item.path === node.path)!;
    return <div key={key} role="none">
      <div className={`private-tree-row ${selected?.kind === kind && selected.path === node.path ? 'selected' : ''}`} role="none">
        <button className="private-tree-select" role="treeitem" aria-label={node.path} aria-level={depth} aria-selected={selected?.kind === kind && selected.path === node.path} onClick={() => select(entry)}>
          <span className="tree-spacer" /><File /><span>{node.name}</span>
        </button>
      </div>
    </div>;
  };

  const tree = <div className="private-tree game-log-tree" role="tree" aria-label="游戏日志树">
    {LOG_GROUPS.map(({ kind, label }) => <section className="game-log-tree-group" key={kind} aria-label={label}>
      <h3>{label}</h3>
      {build(entries, kind).children.map((node) => renderNode(node, kind))}
      {!loading && !error && !entries.some((entry) => entry.kind === kind) ? <div className="private-tree-empty">暂无日志</div> : null}
    </section>)}
  </div>;

  return <section className="private-files-page game-logs-page" aria-labelledby="game-logs-title">
    <div className="private-page-head">
      <div><p className="eyebrow">INSTANCE / GAME LOGS</p><h2 id="game-logs-title">游戏日志</h2></div>
      <label>目标实例<select value={instanceID} disabled={!instances.length} onChange={(event) => setInstanceID(event.target.value)}>
        {!instances.length ? <option value="">暂无实例</option> : null}
        {instances.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
      </select></label>
    </div>

    <div className="private-toolbar" role="toolbar" aria-label="游戏日志工具栏">
      <button title="刷新" onClick={() => void loadTree(true)} disabled={!instanceID || loading}><RefreshCw />刷新</button>
      <button title="下载" aria-label="Download" onClick={() => void download()} disabled={!selected}><Download />下载</button>
      <button ref={drawerTriggerRef} className="private-tree-trigger" aria-controls="game-logs-drawer" aria-expanded={drawerOpen} onClick={() => setDrawerOpen(true)}><Menu />打开文件树</button>
    </div>

    {loading ? <div className="operation-status" role="status">正在读取游戏日志…</div> : null}
    {error ? <div className="error" role="alert">{error}<button onClick={() => void loadTree(false, null)}>重试</button></div> : null}

    <div className="private-files-layout">
      <aside className="private-tree-pane" aria-label="游戏日志目录">{tree}</aside>
      <div className="private-workspace game-log-workspace" aria-label="日志查看区">
        {!instanceID ? <div className="empty">暂无可查看实例</div> : null}
        {instanceID && !loading && !error && entries.length === 0 ? <div className="empty">该实例暂无游戏日志</div> : null}
        {!selected && entries.length > 0 ? <div className="empty">选择日志文件</div> : null}
        {selected ? <>
          <div className="private-file-preview game-log-file-head">
            <File /><div><b>{selected.path}</b><small>{formatBytes(selected.size)} · {new Date(selected.modified_at).toLocaleString('zh-CN')}</small></div>
          </div>
          {downloadError ? <p className="error" role="alert">{downloadError}</p> : null}
          {previewLoading ? <p className="operation-status" role="status">正在读取日志…</p> : null}
          {rotated ? <p className="operation-status" role="status">日志已轮转或删除，请刷新目录</p> : null}
          {previewError && !previewLoading && !rotated ? <p className="error" role="alert">{previewError}</p> : null}
          {preview ? <div className="game-log-preview"><HighlightedLog text={preview.text} />{preview.truncated ? <p className="game-log-truncated">仅显示文件末尾 {PREVIEW_LIMIT} 字节</p> : null}</div> : null}
        </> : null}
      </div>
    </div>

    <div id="game-logs-drawer" ref={drawerRef} className={`private-tree-drawer ${drawerOpen ? 'open' : ''}`} role="dialog" aria-modal="true" aria-label="游戏日志目录" aria-hidden={!drawerOpen}>
      <div className="private-drawer-head"><b>游戏日志目录</b><button aria-label="关闭文件树" onClick={closeDrawer}><X /></button></div>
      {drawerOpen ? tree : null}
    </div>
  </section>;
}
