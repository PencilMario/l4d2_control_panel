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

## Final Combined Verification

- `git merge main` created `9e836d5` with no conflicts. Schedule component wiring, Go regression tests, and E2E assertions remained present after the merge.
- First combined `go test -p 1 ./... -count=1` had only Windows filesystem failures in unchanged `content`, `updates`, and `runtime` tests: non-empty temporary directories, locked temporary files, and one locked test executable. No behavior assertion failed.
- Exact reproductions passed: private upload restart metadata 10/10, private snapshot rollback 10/10, and runtime package 10/10.
- A first isolated-temp retry used a long Windows path and caused Unix Socket `bind: invalid argument`; this proved a path-length environment issue and was not counted as product evidence.
- Final full Go with short unique `C:\\gts<PID>` TEMP/TMP/GOTMPDIR passed every package and cleaned the temporary root.
- Final `go vet ./...` exited 0.
- Final frontend `npm test -- --run` passed 7 files and 101 tests.
- Final production `npm run build` exited 0; only the pre-existing >500 kB chunk advisory remains.
- Final combined `npm run e2e` passed desktop and mobile, 2/2. The run verified schedule creation, all-type help, restricted edit, disable persistence after page leave/re-entry, confirmed delete, and horizontal layout bounds.
- `git diff --check` remained clean throughout the implementation slices.

## Review And Residual Risk

- Direct review scope: full diff from `3bf37bd`, approved schedule design, existing API/SQLite compatibility, removal of the old inline UI owner, preserved schedule identity/payload, async error states, accessible dialogs, and desktop/mobile layout.
- No Critical or Important issue remained after review. Subagent review was unavailable under the active no-delegation constraint.
- Residual risk: the existing production bundle-size advisory; Windows temporary-directory/file-lock noise requires a short isolated temp root for the most reliable full Go run.
