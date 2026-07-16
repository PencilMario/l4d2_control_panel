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
- Backend contract: `go test -p 1 ./internal/scheduler ./internal/httpapi -count=1` passed after adding update/disable/delete regression tests; no backend production code changed.
- Desktop E2E: the first run exposed a Playwright substring-label ambiguity; the second proved the schedule flow but an intentional page reload cleared the existing in-memory Job strip; after using page leave/re-entry for persistence, the final desktop run passed 1/1.
- Mobile E2E: passed 1/1, including help-dialog and schedule-page horizontal-overflow checks.
- Full Go: `go test -p 1 ./... -count=1` passed all production packages.
- Static Go: `go vet ./...` exited 0.
- Fresh frontend: `npm test -- --run` passed 7 files, 101 tests; `npm run build` exited 0 with the existing chunk advisory.
- Concurrent state: `main` advanced to `dd0a096 fix(jobs): 统一限制所有已结束任务记录`; final combined verification is pending after integration.
