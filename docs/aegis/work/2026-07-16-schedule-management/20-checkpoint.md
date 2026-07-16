# TodoCheckpointDraft

- Current todo: add backend contract regression and real desktop/mobile browser coverage.
- Active slice: Task 3 backend and E2E verification.
- Completed: investigation/design/plan, isolated baseline, Task 1 component RED/GREEN, Task 2 App integration and responsive styling.
- Evidence refs: focused component 5/5; component plus App 46/46; full frontend 101/101; production build exit 0.
- Blocked on: nothing.
- Next: add schedule update/delete Go tests, extend Playwright journey, then run the full verification matrix.

# DriftCheckDraft

- Original intent: preserved.
- Compatibility boundary: existing schedule API and dispatcher remain canonical.
- New owner/fallback: `SchedulesPage.tsx` is now the only schedule UI owner; the old inline implementation and arrow row are retired.
- Retirement track: explicit in the plan.
- Evidence state: frontend integration/build evidence present; backend and browser evidence remain.
- Decision: continue.
