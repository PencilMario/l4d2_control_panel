# EvidenceBundleDraft

## Baseline

- `npm test -- --run`: PASS, 6 files and 96 tests.
- `go test -p 1 ./internal/scheduler ./internal/httpapi -count=1`: PASS.

## Implementation Evidence

- Task 1 RED: `npm test -- --run src/app/SchedulesPage.test.tsx` failed because `./SchedulesPage` did not exist.
- Task 1 first GREEN attempt: 3/5 passed; two failures exposed ambiguous accessible names rather than behavior defects.
- Task 1 final GREEN: `npm test -- --run src/app/SchedulesPage.test.tsx` passed 1 file, 5 tests.
- Pending: App integration, full regression, build, browser, layout, and cleanup evidence.
