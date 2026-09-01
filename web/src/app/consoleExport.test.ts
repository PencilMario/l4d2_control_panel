import { describe, expect, it } from "vitest";
import { consoleDownloadFilename } from "./consoleExport";

describe("consoleDownloadFilename", () => {
  it("replaces illegal filename characters and trims trailing separators", () => {
    expect(consoleDownloadFilename("night:/raid?. ")).toBe("night__raid_-console.txt");
  });

  it("uses a safe fallback for an empty name", () => {
    expect(consoleDownloadFilename("...   ")).toBe("game-instance-console.txt");
  });
});
