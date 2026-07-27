// @ts-nocheck -- Vitest runs this Node-side file contract outside the browser bundle.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(resolve(process.cwd(), "src/styles/app.css"), "utf8");

describe("approved control-panel shell", () => {
  it("defines the reference palette and fixed desktop geometry", () => {
    for (const token of ["--canvas:", "--surface:", "--surface-raised:", "--accent:", "--success:", "--warning:", "--danger:"]) {
      expect(css).toContain(token);
    }

    const topbar = css.match(/\.topbar\s*\{([^}]*)\}/)?.[1] ?? "";
    const sidebar = css.match(/\.sidebar\s*\{([^}]*)\}/)?.[1] ?? "";
    const content = css.match(/\.page-content\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(topbar).toContain("height: 46px");
    expect(sidebar).toContain("width: 200px");
    expect(content).toMatch(/max-width:\s*1000px/);
  });

  it("uses low-radius controls, an explicit active navigation state and a large console overlay", () => {
    const controls = css.match(/button,\s*input,\s*select,\s*textarea\s*\{([^}]*)\}/)?.[1] ?? "";
    const terminal = css.match(/\.terminal-modal\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(controls).toMatch(/border-radius:\s*[0-7]px/);
    expect(css).toMatch(/\.sidebar-nav\s+button\.active\s*\{/);
    expect(terminal).toContain("position: fixed");
    expect(terminal).toMatch(/width:\s*min\(1024px/);
    expect(css).toContain("@media (prefers-reduced-motion: reduce)");
  });
});

describe("schedule help dialog layout", () => {
  it("overrides the compact modal width limit for long task descriptions", () => {
    const rule = css.match(/\.schedule-help-dialog\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(rule).toContain("max-width: none");
  });
});

describe("game log highlighting", () => {
  it("styles structural tokens and every normal/bright ANSI foreground distinctly", () => {
    for (const token of ["timestamp", "plugin", "module", "emphasis", "player", "exception", "stack"]) {
      expect(css).toMatch(new RegExp(`\\.log-token-${token}\\s*\\{`));
    }

    const colors = ["black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"];
    for (const color of colors) {
      const normal = css.match(new RegExp(`\\.log-ansi-${color}\\s*\\{([^}]*)\\}`))?.[1] ?? "";
      const bright = css.match(new RegExp(`\\.log-ansi-bright-${color}\\s*\\{([^}]*)\\}`))?.[1] ?? "";
      expect(normal).toMatch(/color:/);
      expect(bright).toMatch(/color:/);
      expect(bright).not.toBe(normal);
    }
  });

  it("uses the private workspace as the only game-log layout owner", () => {
    expect(css).not.toMatch(/\.game-logs-layout\s*\{/);
    expect(css).not.toMatch(/\.game-logs-tree-trigger\s*[,\{]/);
    expect(css).toMatch(/\.game-log-workspace\s*\{/);
  });

  it("keeps long previews inside a bounded scrolling workspace", () => {
    const workspace = css.match(/\.game-log-workspace\s*\{([^}]*)\}/)?.[1] ?? "";
    const preview = css.match(/\.game-log-preview\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(workspace).toContain("min-height: 0");
    expect(workspace).toContain("overflow: hidden");
    expect(preview).toContain("min-height: 0");
    expect(preview).toContain("overflow: auto");
  });

  it("bounds the game-log workspace in its parent grid track", () => {
    const layout = css.match(/\.game-logs-page\s+\.private-files-layout\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(layout).toContain("grid-template-rows: minmax(0, 1fr)");
  });
});
