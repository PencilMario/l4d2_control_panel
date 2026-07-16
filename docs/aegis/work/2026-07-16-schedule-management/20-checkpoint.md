# TodoCheckpointDraft

- Current todo: integrate the concurrent `main` Job-history fix and run final combined verification.
- Active slice: branch integration and completion review.
- Completed: investigation/design/plan, Task 1 component RED/GREEN, Task 2 App integration/styles, Task 3 backend regression and desktop/mobile E2E.
- Evidence refs: component 5/5; component plus App 46/46; full frontend 101/101; focused/full Go; desktop 1/1; mobile 1/1; build exit 0.
- Blocked on: nothing.
- Next: merge `main` commit `dd0a096`, resolve any overlapping App/test edits without dropping either feature, and rerun fresh verification.

# DriftCheckDraft

- Original intent: preserved.
- Compatibility boundary: existing schedule API and dispatcher remain canonical.
- New owner/fallback: `SchedulesPage.tsx` is now the only schedule UI owner; the old inline implementation and arrow row are retired.
- Retirement track: explicit in the plan.
- Evidence state: feature evidence is complete before concurrent-main integration; combined evidence remains.
- Decision: continue.
