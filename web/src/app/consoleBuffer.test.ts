import { describe, expect, it } from "vitest";
import { appendConsoleOutput } from "./consoleBuffer";

describe("appendConsoleOutput", () => {
  it("keeps the newest 1000 lines from one oversized frame", () => {
    const incoming = Array.from({ length: 1001 }, (_, index) => `line-${index + 1}`).join("\n");

    const output = appendConsoleOutput("", incoming, 1000);

    expect(output.split("\n")).toHaveLength(1000);
    expect(output).not.toContain("line-1\n");
    expect(output.startsWith("line-2\n")).toBe(true);
    expect(output.endsWith("line-1001")).toBe(true);
  });

  it("trims the oldest lines across multiple frames", () => {
    const first = Array.from({ length: 750 }, (_, index) => `old-${index + 1}\n`).join("");
    const second = Array.from({ length: 400 }, (_, index) => `new-${index + 1}\n`).join("");

    const output = appendConsoleOutput(first, second, 1000);

    expect(output.split("\n").filter(Boolean)).toHaveLength(1000);
    expect(output.startsWith("old-151\n")).toBe(true);
    expect(output.endsWith("new-400\n")).toBe(true);
  });

  it("joins an unfinished line across frames before counting it", () => {
    const output = appendConsoleOutput("alpha\nbet", "a\ngamma", 1000);

    expect(output).toBe("alpha\nbeta\ngamma");
  });
});
