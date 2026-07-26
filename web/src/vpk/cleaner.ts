import { createVPKUploadID } from "./id";

export type VPKCleanResult = { blob: Blob; removed: number };
export type VPKCleanProgress = { phase: "reading" | "loading" | "repacking" | "writing"; loaded: number; total: number };

function readBlob(file: Blob, onProgress: (progress: VPKCleanProgress) => void): Promise<ArrayBuffer> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onprogress = (event) => onProgress({ phase: "reading", loaded: event.loaded, total: event.total || file.size });
    reader.onerror = () => reject(reader.error || new Error("读取 VPK 失败"));
    reader.onload = () => resolve(reader.result as ArrayBuffer);
    reader.readAsArrayBuffer(file);
  });
}

export async function cleanVPK(file: Blob, onProgress: (progress: VPKCleanProgress) => void = () => {}): Promise<VPKCleanResult> {
  const worker = new Worker(new URL("./cleaner.worker.ts", import.meta.url));
  const id = createVPKUploadID();
  try {
    const data = await readBlob(file, onProgress);
    return await new Promise<VPKCleanResult>((resolve, reject) => {
      worker.onmessage = (event) => {
        if (event.data.id !== id) return;
        if (event.data.type === "progress") {
          onProgress({ phase: event.data.phase, loaded: event.data.loaded || 0, total: event.data.total || file.size });
          return;
        }
        if (event.data.error) reject(new Error(event.data.error));
        else resolve({ blob: new Blob([event.data.data], { type: "application/octet-stream" }), removed: event.data.removed });
      };
      worker.onerror = (event) => reject(new Error(event.message));
      worker.postMessage({ id, data }, [data]);
    });
  } finally {
    worker.terminate();
  }
}
