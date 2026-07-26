export function createVPKUploadID(): string {
  if (typeof globalThis.crypto?.randomUUID === "function") return globalThis.crypto.randomUUID();
  const bytes = new Uint8Array(16);
  if (typeof globalThis.crypto?.getRandomValues === "function") globalThis.crypto.getRandomValues(bytes);
  else {
    const seed = `${Date.now()}-${performance.now()}-${Math.random()}`;
    for (let index = 0; index < bytes.length; index += 1) bytes[index] = seed.charCodeAt(index % seed.length) ^ Math.floor(Math.random() * 256);
  }
  return [...bytes].map((value) => value.toString(16).padStart(2, "0")).join("");
}
