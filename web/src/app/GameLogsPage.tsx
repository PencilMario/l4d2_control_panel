import { useEffect, useRef, useState } from 'react';
import { Download, Menu, RefreshCw, X } from 'lucide-react';
import { api as defaultApi, apiBlob as defaultBlobApi } from '../api/client';
import { HighlightedLog } from './logHighlight';

export { DISPLAY_PREVIEW_LIMIT, truncateForDisplay } from './logHighlight';

export const PREVIEW_LIMIT = 10 * 1024 * 1024;

type LogKind = 'game' | 'sourcemod';
type Entry = { kind: LogKind; path: string; size: number; modified_at: string };
type Preview = { text: string; truncated?: boolean; size?: number; modified_at?: string };
type Node = { name: string; path: string; directory: boolean; children: Node[] };

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
  return root;
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`;
  return `${(size / 1024).toFixed(1)} KiB`;
}

export function GameLogsPage({
  instanceID,
  api = defaultApi,
  blobApi = defaultBlobApi,
}: {
  instanceID: string;
  api?: typeof defaultApi;
  blobApi?: typeof defaultBlobApi;
}) {
  const [entries, setEntries] = useState<Entry[]>([]);
  const [selected, setSelected] = useState<Entry | null>(null);
  const [preview, setPreview] = useState<Preview>();
  const [loading, setLoading] = useState(true);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [error, setError] = useState('');
  const [previewError, setPreviewError] = useState('');
  const [downloadError, setDownloadError] = useState('');
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [rotated, setRotated] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const treeAbort = useRef<AbortController | null>(null);
  const previewAbort = useRef<AbortController | null>(null);
  const treeSequence = useRef(0);
  const previewSequence = useRef(0);

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
        setPreviewError(value.message || 'Failed');
      }
    } finally {
      if (sequence === previewSequence.current) setPreviewLoading(false);
    }
  };

  const loadTree = async (refreshPreview = false) => {
    treeAbort.current?.abort();
    const controller = new AbortController();
    const sequence = ++treeSequence.current;
    treeAbort.current = controller;
    setLoading(true);
    setError('');
    try {
      const data = await api<Entry[]>(`/api/instances/${instanceID}/game-logs/tree`, { signal: controller.signal });
      if (sequence !== treeSequence.current) return;
      setEntries(data);
      if (selected) {
        const current = data.find((item) => item.kind === selected.kind && item.path === selected.path) || null;
        setSelected(current);
        if (refreshPreview && current) await loadPreview(current);
      }
    } catch (cause) {
      const value = cause as Error;
      if (value.name !== 'AbortError' && sequence === treeSequence.current) setError(value.message || 'Failed');
    } finally {
      if (sequence === treeSequence.current) setLoading(false);
    }
  };

  useEffect(() => {
    void loadTree();
    return () => {
      treeAbort.current?.abort();
      previewAbort.current?.abort();
    };
  }, [instanceID]);

  const select = (entry: Entry) => {
    setSelected(entry);
    setDownloadError('');
    setDrawerOpen(false);
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
      setDownloadError((cause as Error).message || 'Download failed');
    }
  };

  const renderNode = (node: Node, kind: LogKind, depth = 0): React.ReactNode => {
    const key = `${kind}/${node.path}`;
    if (node.directory) {
      return <li key={key} style={{ marginLeft: depth * 12 }}>
        <button onClick={() => setOpen((value) => ({ ...value, [key]: !value[key] }))} aria-label={`Toggle ${key}`}>
          {open[key] ? '▾' : '▸'} {node.name}
        </button>
        {open[key] ? <ul>{node.children.map((child) => renderNode(child, kind, depth + 1))}</ul> : null}
      </li>;
    }
    const entry = entries.find((item) => item.kind === kind && item.path === node.path)!;
    return <li key={key} style={{ marginLeft: depth * 12 }}><button aria-label={node.path} onClick={() => select(entry)}>{node.path}</button></li>;
  };

  const tree = <>{loading ? <p>Loading...</p> : null}{!loading && !error && !entries.length ? <p>Empty</p> : null}{error ? <p role="alert">{error}</p> : null}{(['game', 'sourcemod'] as const).map((kind) => <section key={kind}><h4>{kind}</h4><ul>{build(entries, kind).children.map((node) => renderNode(node, kind))}</ul></section>)}</>;

  return <div className="game-logs-shell">
    <button className="game-logs-tree-trigger" aria-label="Open log tree" aria-controls="game-logs-drawer" aria-expanded={drawerOpen} onClick={() => setDrawerOpen(true)}><Menu /></button>
    <div className="game-logs-layout">
      <aside className="game-logs-tree"><button onClick={() => void loadTree(true)} aria-label="Refresh"><RefreshCw size={16} /></button>{tree}</aside>
      <main>{selected ? <>
        <div className="game-log-file-head"><div><b>{selected.path}</b><small>{formatBytes(selected.size)} · {new Date(selected.modified_at).toLocaleString()}</small></div><button aria-label="Download" onClick={() => void download()}><Download size={16} /></button></div>
        {downloadError ? <p role="alert">{downloadError}</p> : null}
        {previewLoading ? <p role="status">正在读取日志</p> : null}
        {rotated ? <p role="status">Log rotated or deleted</p> : previewError && !previewLoading ? <p role="alert">{previewError}</p> : preview ? <><HighlightedLog text={preview.text} />{preview.truncated ? <p>Tail truncated to {PREVIEW_LIMIT} bytes</p> : null}</> : null}
      </> : null}</main>
    </div>
    <div id="game-logs-drawer" className={`game-logs-drawer ${drawerOpen ? 'open' : ''}`} role="dialog" aria-modal="true" aria-label="Log tree" aria-hidden={!drawerOpen}>
      <div><b>Log tree</b><button aria-label="Close log tree" onClick={() => setDrawerOpen(false)}><X /></button></div>
      {drawerOpen ? tree : null}
    </div>
  </div>;
}
