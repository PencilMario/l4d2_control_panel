# Todo Checkpoint

## Current state

- Active slice: final evidence and branch handoff.
- Completed: user-approved design; isolated worktree created from `897937e`; upload now queues Stackwalk-only work; manual analysis is AI-enabled only when `ai=true`; stale instance setting copy removed; Worker and empty-request regressions added; implementation commits `491dd6f` and `61f87e3` deployed to 安可服.
- Remaining: hand off the verified branch state.
- Blocked on: nothing. Production upload end-to-end was not re-triggered during acceptance; this remains a bounded residual risk, not a deployment blocker.

## Evidence refs

- `00-intent.md` and `10-baseline-readset.md` define scope and compatibility.
- Base commit: `897937e`.
- Local focused evidence: crash-related Go packages, `go vet ./...`, frontend `npm test -- --run` (21 files, 216 tests), TypeScript check, production build, and `git diff --check` passed. The crash-related Worker and HTTP default-Stackwalk regressions also passed independently.
- Remote deployment evidence: source revision `61f87e3`; Panel image `sha256:436aa218...`; Panel healthy; `/api/health` returned `status: ok` and `database: ok`; game container IDs were unchanged.
- Remote browser evidence: a fresh Playwright page loaded the real reports `d93e70ad...` and `d98b6e83...`, with no `/analyze` request while loading or selecting a report. A real `ai:false` request created Stackwalk-only job `a51f2b0d-a581-4752-917f-196941c536dc`, which succeeded without changing the report's existing AI completion timestamp. The manual button request was intercepted and captured as `POST` with body `{"ai":true}` without creating another task.
- The earlier `cccc...` Playwright result came from a stale fixture/cache page; closing it and opening a new page restored the live authenticated data source.

## Drift check

- Scope: one upload callback plus backend/frontend regression coverage and deployment evidence.
- Compatibility: Accelerator upload response, automatic Stackwalk, manual analyze API, and `AutoCrashAnalysis` storage remain intact.
- Retirement: only the upload-side automatic AI enqueue branch retires; the manual AI path remains canonical.
- Decision: proceed with the evidence commit and final review; no scope drift or blocking issue remains.
