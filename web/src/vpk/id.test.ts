import { describe, expect, it, vi } from "vitest";
import { createVPKUploadID } from "./id";

describe("createVPKUploadID", () => {
  it("falls back when crypto.randomUUID is unavailable", () => {
    const original = globalThis.crypto;
    vi.stubGlobal("crypto", { getRandomValues: (bytes: Uint8Array) => { bytes.fill(0xab); return bytes; } });
    expect(createVPKUploadID()).toMatch(/^[0-9a-f]{32}$/);
    vi.stubGlobal("crypto", original);
  });
});
