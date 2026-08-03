import { describe, expect, it } from "vitest";
import { cleanResultBytes } from "./cleaner.worker";

describe("cleanResultBytes", () => {
  it("accepts typed-array views returned by the WASM bridge", () => {
    const buffer = new Uint8Array([1, 2, 3, 4]);
    expect([...cleanResultBytes({ data: buffer.subarray(1, 3) })]).toEqual([2, 3]);
  });

  it("rejects missing cleaner output", () => {
    expect(() => cleanResultBytes({})).toThrow("VPK 清理器未返回结果");
  });
});
