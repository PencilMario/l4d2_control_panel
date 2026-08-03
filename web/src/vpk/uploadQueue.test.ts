import { describe, expect, it } from "vitest";
import { cleanupStrategy, isValidSHA256, responseError, selfServiceVPKUploadConfiguration } from "./uploadQueue";

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
  it("always uses browser cleanup regardless of VPK size", () => {
    expect(cleanupStrategy(0)).toBe("local");
    expect(cleanupStrategy(4 * 1024 * 1024 * 1024)).toBe("local");
  });
});

describe("selfServiceVPKUploadConfiguration", () => {
  it("isolates persistence and targets every self-service upload endpoint", () => {
    const config = selfServiceVPKUploadConfiguration;
    expect(config.databaseName).toBe("l4d2-panel-self-service-vpk-uploads");
    expect(config.endpoints.begin).toBe("/api/self-service/vpk/uploads");
    expect(config.endpoints.session("abc")).toBe("/api/self-service/vpk/uploads/abc");
    expect(config.endpoints.complete("abc")).toBe("/api/self-service/vpk/uploads/abc/complete");
    expect(config.completeBody(true)).toEqual({ clean: true });
    expect(config.cleanupAfterComplete).toBe(false);
  });
});
