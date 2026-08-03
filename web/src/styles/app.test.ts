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
    const backdrop = css.match(/\.terminal-backdrop\s*\{([^}]*)\}/)?.[1] ?? "";
    const terminal = css.match(/\.terminal-modal\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(controls).toMatch(/border-radius:\s*[0-7]px/);
    expect(css).toMatch(/\.sidebar-nav\s+button\.active\s*\{/);
    expect(backdrop).toContain("position: fixed");
    expect(backdrop).toContain("backdrop-filter: blur(4px)");
    expect(terminal).toMatch(/width:\s*min\(1024px/);
    expect(terminal).toContain("height: 90vh");
    expect(terminal).toContain("border-radius: 16px");
    expect(css).toContain("@media (prefers-reduced-motion: reduce)");
  });

  it("defines reference-like compact commands and a distinct icon-only delete tool", () => {
    const tool = css.match(/\.tool-button\s*\{([^}]*)\}/)?.[1] ?? "";
    const primary = css.match(/\.command-primary\s*\{([^}]*)\}/)?.[1] ?? "";
    const danger = css.match(/\.command-danger\s*\{([^}]*)\}/)?.[1] ?? "";
    const status = css.match(/\.status-badge\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(tool).toContain("width: 28px");
    expect(tool).toContain("height: 28px");
    expect(primary).toContain("background: var(--accent)");
    expect(danger).toContain("background: transparent");
    expect(status).toContain("pointer-events: none");
    expect(css).toMatch(/\.instance-command\s*\{/);
    expect(css).toMatch(/\.tool-button\.command-danger\s*\{[^}]*border-color:\s*rgba\(127, 29, 29, \.4\)/);
    expect(css).toMatch(/\.instance-command\.command-danger\s*\{[^}]*border-color:\s*rgba\(153, 27, 27, \.4\)/);
    expect(css).toMatch(/\.instance-command,\s*\.tool-button\s*\{[^}]*border-radius:\s*8px/);
    expect(css).toMatch(/\.instance-command\s*\{[^}]*font-size:\s*12px/);
    expect(css).not.toMatch(/\.create\s*,\s*\.primary\s*\{/);
  });

  it("uses continuous overview work surfaces and a fixed eight-metric rail", () => {
    const panel = css.match(/\.instance-panel\s*\{([^}]*)\}/)?.[1] ?? "";
    const titleBar = css.match(/\.instance-command-bar\s*\{([^}]*)\}/)?.[1] ?? "";
    const metrics = css.match(/\.instance-metrics\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(panel).toContain("border-radius: 14px");
    expect(titleBar).toContain("background: #17201c");
    expect(metrics).toContain("grid-template-columns: repeat(8, minmax(0, 1fr))");
    expect(css).not.toMatch(/\.package-line\s*\{/);
    expect(css).not.toMatch(/\.performance-current\s*\{/);
  });

  it("matches the reference workspace framing", () => {
    const toolbar = css.match(/\.private-toolbar\s*\{([^}]*)\}/)?.[1] ?? "";
    const layout = css.match(/\.private-files-layout\s*\{([^}]*)\}/)?.[1] ?? "";
    const surfaces = css.match(/\.private-tree-pane,\s*\.private-workspace\s*\{([^}]*)\}/)?.[1] ?? "";
    const panel = css.match(/\.data-panel\s*,\s*\.control-form\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(toolbar).toContain("border-radius: 12px");
    expect(layout).toContain("gap: 16px");
    expect(surfaces).toContain("border-radius: 16px");
    expect(panel).toContain("border-radius: 10px");
  });

  it("wraps instance commands instead of clipping them on mobile", () => {
    expect(css).toContain(".instance-commands { order: 3; width: 100%; justify-content: flex-start; flex-wrap: wrap;");
  });

  it("keeps the expanded performance controls legible on mobile", () => {
    expect(css).toContain(".performance-chart-head { align-items: stretch; }");
    expect(css).toContain(".performance-modes { overflow-x: auto;");
    expect(css).toContain(".performance-modes::-webkit-scrollbar { display: none; }");
    expect(css).toContain(".performance-modes button { flex: 0 0 auto;");
    expect(css).toContain(".performance-chart-meta { display: none; }");
  });

  it("centers the empty online-player state", () => {
    const empty = css.match(/\.players-empty\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(empty).toContain("justify-content: center !important");
    expect(empty).toContain("width: 100%");
  });

  it("keeps the VPK processing mode controls at the reference height", () => {
    const modes = css.match(/\.content-vpk-upload-controls button\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(modes).toContain("height: 24px");
  });

  it("keeps the standalone VPK workspace on the control-panel palette", () => {
    const page = css.match(/\.self-vpk-page,\s*\.self-vpk-state\s*\{([^}]*)\}/)?.[1] ?? "";
    const queue = css.match(/\.self-vpk-page\s+\.vpk-upload-queue\s*\{([^}]*)\}/)?.[1] ?? "";
    const task = css.match(/\.self-vpk-page\s+\.vpk-upload-task\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(page).toContain("color: var(--text)");
    expect(page).toContain("background: var(--canvas)");
    expect(queue).toContain("background: var(--surface)");
    expect(task).toContain("background: var(--surface-raised)");
  });

  it("renders self-service settings checkboxes as compact switches", () => {
    const toggle = css.match(/\.settings-toggle\s*\{([^}]*)\}/)?.[1] ?? "";
    const input = css.match(/\.settings-toggle input\[type="checkbox"\]\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(toggle).toContain("grid-template-columns: 34px minmax(0, 1fr)");
    expect(input).toContain("appearance: none");
    expect(input).toContain("width: 34px");
    expect(css).toMatch(/\.settings-toggle input\[type="checkbox"\]:checked\s*\{[^}]*background:\s*#84cc16/);
  });

  it("matches the content repository tab strip geometry from the reference", () => {
    const tabs = css.match(/\.content-tabs\s*\{([^}]*)\}/)?.[1] ?? "";
    const button = css.match(/\.content-tabs button\s*\{([^}]*)\}/)?.[1] ?? "";
    const icon = css.match(/\.content-tabs button svg\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(tabs).toContain("gap: 8px");
    expect(tabs).toContain("padding-bottom: 1px");
    expect(button).toContain("display: inline-flex");
    expect(button).toContain("align-items: center");
    expect(button).toContain("justify-content: center");
    expect(button).toContain("height: 36px");
    expect(button).toContain("padding: 0 16px");
    expect(button).toContain("font-size: 12px");
    expect(icon).toContain("width: 16px");
    expect(icon).toContain("height: 16px");
    expect(css).toMatch(/\.content-tabs button:hover:not\(\.active\)\s*\{[^}]*background:\s*transparent/);
  });

  it("matches the shared-game reference panel structure", () => {
    const panel = css.match(/\.shared-game-panel\s*\{([^}]*)\}/)?.[1] ?? "";
    const details = css.match(/\.shared-game-details\s*\{([^}]*)\}/)?.[1] ?? "";
    const policies = css.match(/\.shared-game-policies\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(panel).toContain("gap: 24px");
    expect(panel).toContain("padding: 24px");
    expect(details).toContain("grid-template-columns: repeat(3, minmax(0, 1fr))");
    expect(policies).toContain("grid-template-columns: repeat(3, minmax(0, 1fr))");
    expect(css).toMatch(/\.shared-game-policy-card\s*\{[^}]*min-height:\s*62px/);
  });
});

describe("schedule help dialog layout", () => {
  it("overrides the compact modal width limit for long task descriptions", () => {
    const rule = css.match(/\.schedule-help-dialog\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(rule).toContain("max-width: none");
  });
});

describe("online players reference layout", () => {
  it("keeps player values aligned to the reference table columns", () => {
    expect(css).toContain(".player-operations-wrap { width: 100%; overflow-x: auto;");
    expect(css).toContain(".player-operations { width: 100%; min-width: 100%; table-layout: fixed;");
    expect(css).toContain(".player-operations { width: 650px; min-width: 650px;");
    expect(css).toContain(".player-operations th:nth-child(1) { width: 14%; }");
    expect(css).toContain(".player-operations th:nth-child(2) { width: 25%; }");
    expect(css).toContain(".player-operations th:nth-child(6) { width: 32%; }");
  });
});

describe("background jobs reference layout", () => {
  it("uses the reference filter and table geometry", () => {
    const filters = css.match(/\.job-filters\s*\{([^}]*)\}/)?.[1] ?? "";
    const search = css.match(/\.job-search\s*\{([^}]*)\}/)?.[1] ?? "";
    const table = css.match(/\.job-table\s*\{([^}]*)\}/)?.[1] ?? "";
    const head = css.match(/\.job-table-head\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(filters).toContain("padding: 16px");
    expect(filters).toContain("border-radius: 16px");
    expect(search).toContain("width: 256px");
    expect(table).toContain("border-radius: 16px");
    expect(head).toContain("min-height: 48px");
    expect(css).toContain("minmax(100px, 1fr) minmax(150px, 1fr) 100px 110px 105px");
    expect(css).toMatch(/\n\.job-row\s*\{[^}]*min-height:\s*58px/);
    expect(css).toMatch(/\.job-progress-track\s*\{/);
    expect(css).toContain(".job-time { grid-column: 1 / -1; grid-row: 4;");
    expect(css).toContain(".job-operation { grid-column: 1 / -1; grid-row: 5;");
  });

  it("matches the reference event drawer and full log viewer", () => {
    const events = css.match(/\.job-log-events\s*\{([^}]*)\}/)?.[1] ?? "";
    const page = css.match(/\.task-log-page\s*\{([^}]*)\}/)?.[1] ?? "";
    const output = css.match(/\.task-log-output\s*\{([^}]*)\}/)?.[1] ?? "";

    expect(events).toContain("gap: 8px");
    expect(page).toContain("gap: 16px");
    expect(output).toContain("height: 500px");
    expect(output).toContain("border-radius: 16px");
    expect(css).toMatch(/\.task-log-levels button\.active\s*\{/);
    expect(css).toContain(".job-operation button { display: inline-flex; align-items: center; gap: 4px; min-height: 0; padding: 4px 10px; border-radius: 4px;");
    expect(css).toContain(".job-full-log { border: 0; background: #84cc1633;");
    expect(css).toContain(".job-operation button svg { width: 12px; height: 12px;");
    expect(css).toContain(".task-log-head { align-items: stretch; flex-direction: column;");
    expect(css).toContain(".task-log-search { width: 100%; flex-basis: auto;");
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

  it("uses an independent reference-style game-log layout owner", () => {
    const layout = css.match(/\.game-log-layout\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(layout).toContain("grid-template-columns: minmax(0, 1fr) minmax(0, 3fr)");
    expect(layout).toContain("height: 560px");
    expect(css).toMatch(/\.game-log-toolbar\s*\{/);
    expect(css).toMatch(/\.game-log-tree-pane\s*\{/);
  });

  it("keeps long previews inside a bounded scrolling workspace", () => {
    const viewer = css.match(/\.game-log-viewer\s*\{([^}]*)\}/)?.[1] ?? "";
    const body = css.match(/\.game-log-body\s*\{([^}]*)\}/)?.[1] ?? "";
    expect(viewer).toContain("min-height: 0");
    expect(viewer).toContain("overflow: hidden");
    expect(body).toContain("min-height: 0");
    expect(body).toContain("overflow: auto");
  });

  it("styles numbered log lines without inheriting the old preformatted viewer", () => {
    expect(css).toMatch(/\.game-log-line\s*\{[^}]*grid-template-columns:\s*32px minmax\(0, 1fr\)/);
    expect(css).not.toMatch(/\.game-log-viewer\s+\.log-viewer\s*\{/);
  });
});
