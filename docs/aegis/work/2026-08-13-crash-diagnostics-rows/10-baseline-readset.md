# Baseline Read Set

## Read

- `docs/aegis/specs/2026-08-09-accelerator-crash-report-receiver-design.md`: crash report domain and API compatibility boundary.
- `docs/frontend-redesign-brief.md`: control-panel visual language, Chinese UI terminology, responsive and accessibility constraints.
- `web/src/app/CrashReportsPage.tsx`: current detail structure, download actions, AI reader integration and module table.
- `web/src/app/CrashReportsPage.test.tsx`: current report loading, Stackwalk download, AI reader and filtering coverage.
- `web/src/styles/app.css`: current crash report layout and responsive rules.
- `web/package.json` and `web/vite.config.ts`: test/build commands and React/Vitest stack.

## Baseline facts

- The current detail diagnostics use `.crash-analysis-grid` and `.crash-data-grid`, both two-column grids.
- Stackwalk is rendered as one raw `<pre>` block.
- AI analysis opens `CrashAnalysisReader` and must not be rendered as long Markdown in the detail panel.
- The API exposes Stackwalk and AI state through existing `CrashReport` fields and raw downloads through the existing download endpoint.

## Worktree boundary

- Implementation worktree: `feature/crash-diagnostics-rows`.
- Base commit: `a99ec79`.
- Main worktree has unrelated user change `deployment_test.go`; it must not be modified, staged, reverted, or committed by this task.
