import { sha256 } from "@noble/hashes/sha2.js";
import { cleanVPK } from "./cleaner";
import { createVPKUploadID } from "./id";

export type VPKUploadMode = "clean" | "direct";
export type VPKUploadTask = {
  id: string; name: string; mode: VPKUploadMode; sourceSize: number; size: number;
  blob: Blob; status: "queued" | "cleaning" | "hashing" | "uploading" | "failed" | "completed";
  offset: number; hash?: string; sessionID?: string; removed?: number; error?: string;
  phase?: string; processedBytes?: number; processTotal?: number; startedAt?: number;
  uploadStartedAt?: number; speed?: number; averageSpeed?: number; etaSeconds?: number;
  serverCleanupPending?: boolean;
};

export type VPKUploadConfiguration = {
  databaseName: string;
  endpoints: { begin: string; session: (id: string) => string; complete: (id: string) => string; clean: (name: string) => string };
  completeBody: (serverClean: boolean) => Record<string, unknown>;
  cleanupAfterComplete: boolean;
};
const adminVPKUploadConfiguration: VPKUploadConfiguration = {
  databaseName: "l4d2-panel-vpk-uploads",
  endpoints: { begin: "/api/content/vpk/uploads", session: (id) => `/api/content/vpk/uploads/${id}`, complete: (id) => `/api/content/vpk/uploads/${id}/complete`, clean: (name) => `/api/content/vpk/${encodeURIComponent(name)}/clean` },
  completeBody: () => ({}), cleanupAfterComplete: true,
};
export const selfServiceVPKUploadConfiguration: VPKUploadConfiguration = {
  databaseName: "l4d2-panel-self-service-vpk-uploads",
  endpoints: { begin: "/api/self-service/vpk/uploads", session: (id) => `/api/self-service/vpk/uploads/${id}`, complete: (id) => `/api/self-service/vpk/uploads/${id}/complete`, clean: () => "" },
  completeBody: (serverClean) => ({ clean: serverClean }), cleanupAfterComplete: false,
};
let configuration = adminVPKUploadConfiguration;
const STORE = "tasks";
const CHUNK = 8 * 1024 * 1024;
let running = false;
let listener: ((tasks: VPKUploadTask[]) => void) | undefined;
let completedListener: (() => void) | undefined;
const memoryStores = new Map<string, Map<string, VPKUploadTask>>();
let memoryTasks = new Map<string, VPKUploadTask>();
const useMemoryStore = typeof navigator !== "undefined" && /jsdom/i.test(navigator.userAgent);

function database(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(configuration.databaseName, 1);
    request.onupgradeneeded = () => request.result.createObjectStore(STORE, { keyPath: "id" });
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}
async function transact<T>(mode: IDBTransactionMode, action: (store: IDBObjectStore) => IDBRequest<T>) {
  const db = await database();
  return new Promise<T>((resolve, reject) => {
    const request = action(db.transaction(STORE, mode).objectStore(STORE));
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  }).finally(() => db.close());
}
export const listVPKUploads = () => useMemoryStore ? Promise.resolve([...memoryTasks.values()]) : transact<VPKUploadTask[]>("readonly", (store) => store.getAll());
const save = (task: VPKUploadTask) => { if (useMemoryStore) { memoryTasks.set(task.id, task); return Promise.resolve(task.id); } return transact<IDBValidKey>("readwrite", (store) => store.put(task)); };
const remove = (id: string) => { if (useMemoryStore) { memoryTasks.delete(id); return Promise.resolve(); } return transact<undefined>("readwrite", (store) => store.delete(id) as IDBRequest<undefined>); };
async function publish() { listener?.((await listVPKUploads()).sort((a, b) => a.name.localeCompare(b.name))); }
async function publishTransient(task: VPKUploadTask) {
  const tasks = await listVPKUploads();
  const index = tasks.findIndex((item) => item.id === task.id);
  if (index >= 0) tasks[index] = task; else tasks.push(task);
  listener?.(tasks.sort((a, b) => a.name.localeCompare(b.name)));
}
export function isValidSHA256(value: unknown): value is string { return typeof value === "string" && /^[0-9a-f]{64}$/i.test(value); }
export function cleanupStrategy(_size: number): "local" | "server" { return "local"; }
export function configureVPKUploadQueue(next: VPKUploadConfiguration) {
  if (running || listener) throw new Error("VPK upload queue is already active");
  configuration = next;
  memoryTasks = memoryStores.get(next.databaseName) || new Map<string, VPKUploadTask>();
  memoryStores.set(next.databaseName, memoryTasks);
}

export async function enqueueVPKUploads(files: Array<{ file: File; mode: VPKUploadMode }>) {
  for (const item of files) {
    await save({ id: createVPKUploadID(), name: item.file.name, mode: item.mode, sourceSize: item.file.size, size: item.file.size, blob: item.file, status: "queued", offset: 0 });
  }
  await publish(); void run();
}
export async function cancelVPKUpload(task: VPKUploadTask) {
  if (task.sessionID) await fetch(configuration.endpoints.session(task.sessionID), { method: "DELETE", credentials: "same-origin" });
  await remove(task.id); await publish();
}
export async function retryVPKUpload(task: VPKUploadTask) { await save({ ...task, status: "queued", error: undefined }); await publish(); void run(); }
export async function startVPKUploadQueue(onChange: (tasks: VPKUploadTask[]) => void, onCompleted: () => void) {
  listener = onChange; completedListener = onCompleted; await publish(); void run();
  return () => { listener = undefined; completedListener = undefined; };
}

async function digest(blob: Blob, onProgress: (loaded: number) => void) {
  const hash = sha256.create();
  for (let offset = 0; offset < blob.size; offset += CHUNK) {
    const end = Math.min(offset + CHUNK, blob.size);
    hash.update(new Uint8Array(await blob.slice(offset, end).arrayBuffer()));
    onProgress(end);
  }
  return [...hash.digest()].map((value) => value.toString(16).padStart(2, "0")).join("");
}
async function requestJSON(path: string, init: RequestInit = {}) {
  const response = await fetch(path, { credentials: "same-origin", headers: { "Content-Type": "application/json", ...(init.headers || {}) }, ...init });
  if (!response.ok) throw await responseError(response);
  return response.status === 204 ? undefined : response.json();
}
export async function responseError(response: Response): Promise<Error> {
  try {
    const body = await response.json();
    const message = body?.error?.message || body?.message;
    if (message) return new Error(message);
  } catch {}
  return new Error(`HTTP ${response.status}`);
}
async function processTask(task: VPKUploadTask) {
  try {
    task.startedAt ||= Date.now();
    const useServerCleanup = task.mode === "clean" && cleanupStrategy(task.sourceSize) === "server";
    if (task.mode === "clean" && !useServerCleanup && task.removed == null) {
      task = { ...task, status: "cleaning", phase: "读取 VPK", processedBytes: 0, processTotal: task.sourceSize }; await save(task); await publish();
      const result = await cleanVPK(task.blob, (progress) => {
        const labels = { reading: "读取 VPK", loading: "加载 Go WASM", repacking: "重打包 VPK", writing: "写入清理结果" };
        task = { ...task, phase: labels[progress.phase], processedBytes: progress.loaded, processTotal: progress.total };
        void publishTransient(task);
      });
      task = { ...task, blob: result.blob, size: result.blob.size, removed: result.removed, processedBytes: result.blob.size, processTotal: result.blob.size };
    }
    if (!task.serverCleanupPending && !isValidSHA256(task.hash)) {
      task = { ...task, status: "hashing", phase: "计算 SHA-256", processedBytes: 0, processTotal: task.size }; await save(task); await publish();
      const computedHash: unknown = await digest(task.blob, (loaded) => { task = { ...task, processedBytes: loaded }; void publishTransient(task); });
      if (!isValidSHA256(computedHash)) throw new Error("SHA-256 计算结果无效");
      task = { ...task, hash: computedHash };
      await save(task); await publish();
    }
    if (!task.serverCleanupPending && task.sessionID) {
      const response = await fetch(configuration.endpoints.session(task.sessionID), { credentials: "same-origin" });
      if (response.ok) task.offset = Number((await response.json()).offset || 0);
      else { task.sessionID = undefined; task.offset = 0; }
    }
    if (!task.serverCleanupPending && !task.sessionID) {
      const session = await requestJSON(configuration.endpoints.begin, { method: "POST", body: JSON.stringify({ name: task.name, size: task.size, sha256: task.hash }) });
      task.sessionID = session.id ?? session.ID; task.offset = session.offset || 0;
    }
    if (!task.serverCleanupPending) {
      task.status = "uploading"; task.phase = task.offset ? "断点续传" : "上传 VPK"; task.uploadStartedAt ||= Date.now(); await save(task); await publish();
      const recentSpeeds: number[] = [];
      while (task.offset < task.size) {
      const chunkStarted = performance.now();
      const previousOffset = task.offset;
      const response = await fetch(configuration.endpoints.session(task.sessionID!), { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/octet-stream", "Upload-Offset": String(task.offset) }, body: task.blob.slice(task.offset, task.offset + CHUNK) });
      if (!response.ok) throw await responseError(response);
      task.offset = Number(response.headers.get("Upload-Offset") || Math.min(task.offset + CHUNK, task.size));
      const seconds = Math.max((performance.now() - chunkStarted) / 1000, 0.001);
      recentSpeeds.push((task.offset - previousOffset) / seconds); if (recentSpeeds.length > 4) recentSpeeds.shift();
      task.speed = recentSpeeds.reduce((sum, value) => sum + value, 0) / recentSpeeds.length;
      task.averageSpeed = task.offset / Math.max((Date.now() - task.uploadStartedAt) / 1000, 0.001);
      task.etaSeconds = task.speed > 0 ? (task.size - task.offset) / task.speed : undefined;
      task.processedBytes = task.offset; task.processTotal = task.size;
      await save(task); await publish();
      }
      await requestJSON(configuration.endpoints.complete(task.sessionID!), { method: "POST", body: JSON.stringify(configuration.completeBody(useServerCleanup)) });
      task = { ...task, sessionID: undefined, serverCleanupPending: useServerCleanup && configuration.cleanupAfterComplete };
      await save(task); await publish();
    }
    if (task.serverCleanupPending) {
      task = { ...task, status: "cleaning", phase: "服务器清理 VPK", processedBytes: task.size, processTotal: task.size };
      await save(task); await publish();
      const cleaned = await requestJSON(configuration.endpoints.clean(task.name), { method: "POST", body: JSON.stringify({ confirm: true }) });
      task = { ...task, size: Number(cleaned.after_size ?? task.size), removed: Number(cleaned.removed ?? 0), serverCleanupPending: false };
    }
    task.status = "completed"; await save(task); await publish(); completedListener?.();
  } catch (error) { task.status = "failed"; task.error = error instanceof Error ? error.message : String(error); await save(task); await publish(); }
}
async function run() {
  if (running) return; running = true;
  try { for (;;) { const task = (await listVPKUploads()).find((item) => item.status === "queued" || item.status === "cleaning" || item.status === "hashing" || item.status === "uploading"); if (!task) break; await processTask(task); } }
  finally { running = false; }
}
