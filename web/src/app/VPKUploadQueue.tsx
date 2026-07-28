import { useEffect, useState } from "react";
import type { VPKUploadTask } from "../vpk/uploadQueue";

export function VPKUploadQueue({ tasks, onRetry, onCancel }: { tasks: VPKUploadTask[]; onRetry: (task: VPKUploadTask) => void; onCancel: (task: VPKUploadTask) => void }) {
  const [, setClock] = useState(0);
  useEffect(() => { const timer = window.setInterval(() => setClock((value) => value + 1), 1000); return () => window.clearInterval(timer); }, []);
  if (!tasks.length) return null;
  const totalBytes = tasks.reduce((sum, task) => sum + task.size, 0);
  const uploadedBytes = tasks.reduce((sum, task) => sum + (task.status === "completed" ? task.size : task.status === "uploading" ? task.offset : 0), 0);
  const totalSpeed = tasks.reduce((sum, task) => sum + (task.status === "uploading" ? task.speed || 0 : 0), 0);
  return <section className="vpk-upload-queue" aria-label="VPK 上传队列">
    <header className="vpk-upload-queue-head"><h3>正在处理的上传队列 ({tasks.length})</h3><small>{formatBytes(uploadedBytes)} / {formatBytes(totalBytes)}{totalSpeed > 0 ? ` · ${formatSpeed(totalSpeed)}` : ""}</small></header>
    <div className="vpk-upload-queue-list" role="list" aria-label="上传任务">{tasks.map((task) => <UploadTask key={task.id} task={task} onRetry={onRetry} onCancel={onCancel} />)}</div>
  </section>;
}

function UploadTask({ task, onRetry, onCancel }: { task: VPKUploadTask; onRetry: (task: VPKUploadTask) => void; onCancel: (task: VPKUploadTask) => void }) {
  const labels = { queued: "等待处理", cleaning: "本地清理中", hashing: "计算校验中", uploading: "上传中", failed: "失败", completed: "完成" };
  const processing = task.status === "cleaning" || task.status === "hashing";
  const progressValue = processing ? task.processedBytes || 0 : task.status === "completed" ? task.size : task.offset;
  const progressMax = processing ? task.processTotal || task.sourceSize || 1 : task.size || 1;
  const percent = Math.max(0, Math.min(100, Math.round((progressValue / progressMax) * 100)));
  const elapsed = task.startedAt ? Math.max(0, Math.floor((Date.now() - task.startedAt) / 1000)) : 0;
  const phase = task.status === "completed" || task.status === "failed" ? "" : task.phase;
  return <div className="vpk-upload-task" role="listitem">
    <div className="vpk-upload-task-head"><div className="vpk-upload-task-name"><b>{task.name}</b><small>{task.mode === "clean" ? "上传前清理" : "直接上传"} · {labels[task.status]}{phase ? ` · ${phase} · 已用时 ${formatDuration(elapsed)}` : ""}</small></div><strong>{percent}%</strong></div>
    <div className="vpk-upload-progress-track" role="progressbar" aria-label={`${task.name} 上传进度`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}><i style={{ width: `${percent}%` }} /></div>
    <div className="vpk-upload-task-foot"><div className="vpk-upload-progress-meta"><small>{formatBytes(progressValue)} / {formatBytes(progressMax)}</small>{task.status === "uploading" ? <small>实时 {formatSpeed(task.speed || 0)} · 平均 {formatSpeed(task.averageSpeed || 0)}{task.etaSeconds != null ? ` · 剩余 ${formatDuration(Math.ceil(task.etaSeconds))}` : ""}</small> : null}{task.mode === "clean" && task.removed != null ? <small>移除 {task.removed} 项 · {formatBytes(task.sourceSize)} → {formatBytes(task.size)} · 节省 {savingPercent(task)}%</small> : null}{task.error ? <small className="error-text">{task.error}</small> : null}</div><div className="inline-actions">{task.status === "failed" ? <button onClick={() => onRetry(task)}>重试</button> : null}<button onClick={() => onCancel(task)}>移除</button></div></div>
  </div>;
}

export function formatBytes(bytes: number) { if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(2)} GiB`; if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(1)} MiB`; if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KiB`; return `${bytes} B`; }
export function formatSpeed(bytes: number) { return `${formatBytes(bytes)}/s`; }
export function formatDuration(seconds: number) { if (seconds < 60) return `${seconds} 秒`; const minutes = Math.floor(seconds / 60); const rest = seconds % 60; return `${minutes} 分 ${rest} 秒`; }
function savingPercent(task: VPKUploadTask) { return task.sourceSize > 0 ? Math.max(0, Math.round((1 - task.size / task.sourceSize) * 100)) : 0; }
