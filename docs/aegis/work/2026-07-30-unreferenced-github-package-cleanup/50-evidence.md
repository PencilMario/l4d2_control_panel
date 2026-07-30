# Verification Evidence

## Implemented Scope

- `content.PackageManager` deterministically retains each GitHub repository's latest package and deletes only unreferenced older source packages.
- Both instance `SelectedPackageID` and `PackageVersion` protect concrete packages.
- Regular uploaded packages remain outside cleanup scope.
- Paired archive/metadata deletion continues after independent failures, reports actual released bytes, preserves retry metadata, honors cancellation, and redacts managed paths.
- Scheduled maintenance receives the real instance store and package manager; release synchronization no longer calls the historical no-op cleanup hook.
- Schedule help text describes the new local cleanup behavior.

## Evidence

- RED content test: `go test ./internal/content -run TestCleanupUnreferencedSourceVersions -count=1` failed because `CleanupUnreferencedSourceVersions` was undefined.
- RED maintenance test: `go test ./internal/maintenance -run TestCleanupPackages -count=1` failed because `WithPackageCleanup` was undefined.
- Target regression: `go test ./internal/content ./internal/releases ./internal/maintenance ./internal/automation ./cmd/panel -count=1` passed after final edits.
- Review repair regression: `go test ./internal/releases ./internal/content ./internal/maintenance ./internal/automation ./cmd/panel -count=1` passed after adding dynamic mid-pair cancellation, metadata-ID validation, scan/repository path redaction, and v1-to-v2 synchronization retention coverage.
- Focused UI regression: `npm test -- --run src/app/SchedulesPage.test.tsx` passed 7 tests after final edits.
- Full frontend regression: `npm test -- --run` passed 191 tests in 15 files.
- Frontend production compile: `npm run build:web` completed successfully; Vite emitted only the existing large-chunk advisory.
- Known Windows race checks: the initial failing content test passed 5 consecutive isolated runs; two later failing httpapi tests each passed 5 isolated runs; `internal/joblogs` passed 5 isolated runs after a Windows executable file-lock failure.

## Full Go Suite Environment Result

`go test ./... -count=1` and `go test -p 1 ./... -count=1` did not produce a clean aggregate exit on this Windows host. Failures moved between unrelated tests and occurred only during `t.TempDir` removal (`directory is not empty`) or before test execution (`test.exe: process cannot access the file because it is being used by another process`). All product assertions shown by those runs passed, and focused reruns passed. This is the repository's documented Windows temporary-directory/file-lock race, not treated as hidden success.

## Drift Check

- Scope remains local `packages/releases`; no remote GitHub mutation exists.
- Schedule/API payloads and retention semantics remain unchanged.
- No regular package deletion path was added.
- The synchronization-time no-op owner retired; scheduled maintenance is the only cleanup owner.
- Advisory review findings were repaired: cancellation is rechecked between archive and metadata removal; malformed metadata IDs fail before deletion; scan and instance-source errors redact managed roots; release synchronization explicitly proves prior-version retention.
- Decision: verification evidence is sufficient for the requested behavior, with the aggregate Windows suite exit retained as residual environmental risk.
