// @ts-nocheck -- Vitest runs this Node-side file contract outside the browser bundle.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(resolve(process.cwd(), "src/styles/app.css"), "utf8");

describe("shared interaction motion", () => {
  it("defines layered hover, busy and reduced-motion states", () => {
    expect(css).toContain("--motion-fast:");
    expect(css).toMatch(/\.card:hover/);
    expect(css).toMatch(/\[aria-busy=["']true["']\]/);
    expect(css).toContain("@media (prefers-reduced-motion: reduce)");
  });
});

describe("schedule help dialog layout", () => {
  it("overrides the compact modal width limit for long task descriptions", () => {
    const rule = css.match(/\.schedule-help-dialog\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(rule).toContain("max-width: none");
  });
});
