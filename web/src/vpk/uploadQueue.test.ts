import { describe, expect, it } from "vitest";
import { cleanupStrategy, isValidSHA256, responseError } from "./uploadQueue";

describe("responseError", () => {
  it("uses the API error message for failed uploads", async () => {
    const response = new Response(JSON.stringify({ error: { code: "upload_incomplete", message: "target VPK already exists" } }), { status: 422, headers: { "Content-Type": "application/json" } });
    await expect(responseError(response)).resolves.toEqual(new Error("target VPK already exists"));
  });
});

describe("isValidSHA256", () => {
  it("accepts exactly 64 hexadecimal characters", () => {
    expect(isValidSHA256("ab".repeat(32))).toBe(true);
    expect(isValidSHA256("ab".repeat(31))).toBe(false);
    expect(isValidSHA256("z".repeat(64))).toBe(false);
    expect(isValidSHA256(undefined)).toBe(false);
  });
});

describe("cleanupStrategy", () => {
  it("routes large VPK files to server cleanup to avoid exhausting browser WASM memory", () => {
    expect(cleanupStrategy(256 * 1024 * 1024)).toBe("local");
    expect(cleanupStrategy(736.1 * 1024 * 1024)).toBe("server");
  });
});
