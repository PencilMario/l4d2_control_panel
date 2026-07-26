import { useState } from "react";
import type { VPKUploadMode } from "../vpk/uploadQueue";

export function VPKUploadDialog({ files, onCancel, onConfirm }: { files: File[]; onCancel: () => void; onConfirm: (items: Array<{ file: File; mode: VPKUploadMode }>) => void }) {
  const [modes, setModes] = useState<Record<string, VPKUploadMode>>(() => Object.fromEntries(files.map((file, index) => [`${index}:${file.name}`, "clean"])));
  const setAll = (mode: VPKUploadMode) => setModes(Object.fromEntries(files.map((file, index) => [`${index}:${file.name}`, mode])));
  return <div className="modal-wrap"><section className="modal vpk-upload-dialog" role="dialog" aria-modal="true" aria-labelledby="vpk-upload-title">
    <div className="vpk-upload-dialog-head"><div><small>上传前确认</small><h2 id="vpk-upload-title">已选择 {files.length} 个 VPK</h2></div><button onClick={onCancel} aria-label="关闭 VPK 上传确认">×</button></div>
    <p>默认先在本机清理资源，再上传处理后的文件。刷新页面后上传任务会自动恢复。</p>
    <div className="inline-actions"><button onClick={() => setAll("clean")}>全部上传前清理</button><button onClick={() => setAll("direct")}>全部直接上传</button></div>
    <div className="vpk-selection-list">{files.map((file, index) => { const key = `${index}:${file.name}`; return <div className="vpk-selection-row" key={key}><div><b>{file.name}</b><small>{formatSize(file.size)}</small></div><select aria-label={`${file.name} 处理方式`} value={modes[key]} onChange={(event) => setModes((current) => ({ ...current, [key]: event.target.value as VPKUploadMode }))}><option value="clean">上传前清理</option><option value="direct">直接上传</option></select></div>; })}</div>
    <div className="modal-actions"><button onClick={onCancel}>取消</button><button className="primary" onClick={() => onConfirm(files.map((file, index) => ({ file, mode: modes[`${index}:${file.name}`] })))}>加入上传队列</button></div>
  </section></div>;
}
function formatSize(bytes: number) { return bytes >= 1024 * 1024 ? `${(bytes / 1024 / 1024).toFixed(1)} MiB` : `${(bytes / 1024).toFixed(1)} KiB`; }
