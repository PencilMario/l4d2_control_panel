# TodoCheckpointDraft

- Current todo: integrate the tested schedule component into `App.tsx` and add responsive styles.
- Active slice: Task 2 App integration.
- Completed: repository investigation, design/plan, isolated baseline, Task 1 RED/GREEN component implementation.
- Evidence refs: `10-baseline-readset.md`; focused Vitest RED on missing component; focused GREEN with 5/5 tests.
- Blocked on: nothing.
- Next: replace the inline schedule owner, add CSS, and run App/full frontend verification.

# DriftCheckDraft

- Original intent: preserved.
- Compatibility boundary: existing schedule API and dispatcher remain canonical.
- New owner/fallback: `SchedulesPage.tsx` is tested but not yet on the main render path; the old inline page retires in Task 2.
- Retirement track: explicit in the plan.
- Evidence state: Task 1 RED/GREEN present; integration, build, backend, and browser evidence remain.
- Decision: continue.
