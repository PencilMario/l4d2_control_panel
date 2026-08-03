declare function importScripts(...urls: string[]): void;
declare class Go { importObject: WebAssembly.Imports; run(instance: WebAssembly.Instance): Promise<void>; }
let ready: Promise<void> | null = null;
export function cleanResultBytes(result: any): Uint8Array {
  if (!result || result.error) throw new Error(result?.error || "VPK 清理器未返回结果");
  const value = result.data;
  if (value instanceof Uint8Array) return value;
  if (value instanceof ArrayBuffer) return new Uint8Array(value);
  if (value && typeof value.byteLength === "number") return new Uint8Array(value.buffer || value, value.byteOffset || 0, value.byteLength);
  throw new Error("VPK 清理器未返回结果");
}
function load() { if (!ready) ready = (async () => { importScripts("/wasm_exec.js"); const go = new Go(); const response = await fetch("/vpk-cleaner.wasm"); const module = await WebAssembly.instantiateStreaming(response, go.importObject); void go.run(module.instance); while (typeof (self as any).cleanVPKBytes !== "function") await new Promise((resolve) => setTimeout(resolve, 0)); })(); return ready; }
self.onmessage = async (event: MessageEvent<{ id: string; data: ArrayBuffer }>) => { try { self.postMessage({ id: event.data.id, type: "progress", phase: "loading", loaded: 0, total: event.data.data.byteLength }); await load(); self.postMessage({ id: event.data.id, type: "progress", phase: "repacking", loaded: 0, total: event.data.data.byteLength }); const result = await Promise.resolve((self as any).cleanVPKBytes(new Uint8Array(event.data.data))); const data = cleanResultBytes(result).slice().buffer; self.postMessage({ id: event.data.id, type: "progress", phase: "writing", loaded: data.byteLength, total: data.byteLength }); self.postMessage({ id: event.data.id, data, removed: result.removed || 0 }, { transfer: [data] }); } catch (error) { self.postMessage({ id: event.data.id, error: error instanceof Error ? error.message : String(error) }); } };
