# Todo Checkpoint

## Current state

- Active slice: deployed to 安可服 and completed remote acceptance.
- Completed: baseline read, approved vertical-row design, TDD red/green, pure Stackwalk parser, same-line and separate-line `Found by` handling, four-row UI, responsive CSS, focused tests, full frontend tests, TypeScript check, production build, desktop browser check and 390px mobile browser check.
- Completed: review, isolated commit, main-branch integration, remote deployment, desktop/mobile Playwright acceptance, API/health checks, and cleanup of deployment temporaries.
- Remaining: none for this slice; the remote rollback source backup remains intentionally retained for short-term recovery.
- Blocked on: nothing.

## Evidence refs

- `50-evidence.md` contains the command results and browser measurements.
- Base commit: `a99ec79`.

## Drift check

- Scope: remains limited to `CrashReportsPage`, Stackwalk parsing/rendering, focused tests and crash-page CSS.
- Compatibility: existing crash API/download routes and AI reader focus path remain intact; no backend or data format changes.
- Retirement: the old two-column analysis/data grids and oversized AI action are retired only within this page.
- Deployment evidence: remote revision `b988383`; Panel image `sha256:d9b9ee2b7be790c9578285b076034c28ad0b130429364ec30f8cdd029a23b121`; Panel container healthy; `/api/health` returned `status: ok` and `database: ok`.
- Browser evidence: production page at `http://100.73.249.118:18081/` showed four vertical diagnostic rows, two independent Stackwalk frames plus preserved log rows, no console errors/warnings, and no horizontal overflow at 390px.
- Decision: verified evidence collected; no blocking drift.
