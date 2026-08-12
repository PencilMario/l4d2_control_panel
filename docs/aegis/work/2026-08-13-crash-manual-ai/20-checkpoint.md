# Todo Checkpoint

## Current state

- Active slice: review fixes are implemented; preparing final local verification and redeployment.
- Completed: user-approved design; isolated worktree created from `897937e`; upload now queues Stackwalk-only work; manual analysis is AI-enabled only when `ai=true`; stale instance setting copy removed; Worker and empty-request regressions added.
- Remaining: run final local verification, commit review fixes, deploy the corrected revision, verify remotely with Playwright, and record evidence.
- Blocked on: nothing.

## Evidence refs

- `00-intent.md` and `10-baseline-readset.md` define scope and compatibility.
- Base commit: `897937e`.
- Local focused evidence: `npm test -- --run src/app/InstanceConfigModal.test.tsx` -> 10 passed.
- Review evidence: empty analyze request failed before the default change and passed after; Stackwalk-only Worker regression passed after the test was added.

## Drift check

- Scope: one upload callback plus backend/frontend regression coverage and deployment evidence.
- Compatibility: Accelerator upload response, automatic Stackwalk, manual analyze API, and `AutoCrashAnalysis` storage remain intact.
- Retirement: only the upload-side automatic AI enqueue branch retires; the manual AI path remains canonical.
- Decision: continue to final verification and deployment; prior review concerns are addressed in the working tree.
