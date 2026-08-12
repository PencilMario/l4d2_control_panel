# Todo Checkpoint

## Current state

- Active slice: local implementation and regression verification complete; preparing review and deployment.
- Completed: user-approved design; isolated worktree created from `897937e`; upload now queues Stackwalk-only work; manual analysis remains AI-enabled; stale instance setting copy removed.
- Remaining: complete independent review, run final local verification, commit, deploy, verify remotely, and record evidence.
- Blocked on: nothing.

## Evidence refs

- `00-intent.md` and `10-baseline-readset.md` define scope and compatibility.
- Base commit: `897937e`.
- Local focused evidence: `npm test -- --run src/app/InstanceConfigModal.test.tsx` -> 10 passed.

## Drift check

- Scope: one upload callback plus backend/frontend regression coverage and deployment evidence.
- Compatibility: Accelerator upload response, automatic Stackwalk, manual analyze API, and `AutoCrashAnalysis` storage remain intact.
- Retirement: only the upload-side automatic AI enqueue branch retires; the manual AI path remains canonical.
- Decision: continue to independent review and final verification.
