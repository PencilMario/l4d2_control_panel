# Baseline Read Set

- `web/src/app/stackwalk.ts`: current frame/log parser; ordinary log entries are currently returned to the UI.
- `web/src/app/StackwalkView.tsx`: current list renderer; it renders both frame and log entries.
- `web/src/app/CrashReportsPage.tsx`: owns Stackwalk loading, frame count, and recent-report row summary.
- `web/src/app/CrashReportsPage.test.tsx`: current page-level Stackwalk and report-list assertions.
- `web/src/app/stackwalk.test.ts`: parser regression coverage for compact frames, Thread text, `Found by`, and unrecognised output.
- `web/src/styles/app.css`: current diagnostic/list layout and responsive rules.
- `docs/aegis/work/2026-08-13-crash-manual-ai/00-intent.md`: existing compatibility boundary; this slice adds no backend or artifact changes.
