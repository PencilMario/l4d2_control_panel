# TodoCheckpointDraft

- Current todo: finish branch integration into `main` and deploy the verified revision.
- Active slice: completion and deployment handoff.
- Completed: all implementation tasks, concurrent-main integration, direct self-review, and final combined verification.
- Evidence refs: component 5/5; component plus App 46/46; frontend 101/101; full Go pass; `go vet`; desktop/mobile E2E 2/2; build exit 0.
- Blocked on: nothing.
- Next: integrate the feature branch into `main`, verify the merged result, then update `sirphomesv` with backup and real UI/API checks.

# DriftCheckDraft

- Original intent: preserved.
- Compatibility boundary: existing schedule API and dispatcher remain canonical.
- New owner/fallback: `SchedulesPage.tsx` is now the only schedule UI owner; the old inline implementation and arrow row are retired.
- Retirement track: explicit in the plan.
- Evidence state: combined verification covers the feature and `dd0a096`; deployment evidence remains external to this branch checkpoint.
- Decision: continue.
