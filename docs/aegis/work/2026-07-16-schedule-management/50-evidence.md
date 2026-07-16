# EvidenceBundleDraft

## Baseline

- `npm test -- --run`: PASS, 6 files and 96 tests.
- `go test -p 1 ./internal/scheduler ./internal/httpapi -count=1`: PASS.

## Implementation Evidence

- Task 1 RED: `npm test -- --run src/app/SchedulesPage.test.tsx` failed because `./SchedulesPage` did not exist.
- Task 1 first GREEN attempt: 3/5 passed; two failures exposed ambiguous accessible names rather than behavior defects.
- Task 1 final GREEN: `npm test -- --run src/app/SchedulesPage.test.tsx` passed 1 file, 5 tests.
- Task 2 focused integration: `npm test -- --run src/app/SchedulesPage.test.tsx src/app/App.test.tsx` passed 2 files, 46 tests.
- Task 2 full frontend: `npm test -- --run` passed 7 files, 101 tests.
- Task 2 build: `npm run build` exited 0; the existing >500 kB chunk advisory remains.
- Pending: backend, browser, layout, full Go, and cleanup evidence.
