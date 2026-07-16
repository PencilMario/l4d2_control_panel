# TaskIntentDraft

- Outcome: existing schedules can be edited and deleted; all eight task types have detailed Chinese descriptions in a help dialog.
- Confirmed edit boundary: only Cron, online-player policy, and enabled state are mutable.
- Required repair: preserve task identity and payload on edit; add type-specific creation payloads for package, Release, and cleanup tasks.
- Non-goals: new task types, Job cancellation, batch editing, schedule detail routes, API version changes.
- Risk hints: a partial task object sent through the existing upsert would erase payload or last-run data; deleting a schedule must not imply cancellation of an already-created Job.
