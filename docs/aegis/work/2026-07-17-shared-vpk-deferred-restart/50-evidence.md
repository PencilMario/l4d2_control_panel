# Evidence: Shared VPK Deferred Restart

## Scope

Implemented the approved shared VPK workflow: one durable pending restart per active instance, indefinite empty-server waiting, three consecutive player-query failures as the abnormal-state trigger, stopped-instance cancellation, changed-container completion, serialized restart Jobs, upload completion integration, recovery persistence, and task-list visibility.

## Verification

- `go test ./internal/store ./internal/vpkrestart ./internal/httpapi ./cmd/panel -count=1`: passed on focused rerun; one earlier aggregate run hit the repository's known Windows `testing.TempDir` cleanup-only failure in `internal/store/TestOpenEnablesWALAndMigrates`.
- `go test ./internal/store -count=1`: passed.
- `go test ./internal/content ./internal/updates ./runtime -count=1`: passed.
- `go test ./internal/vpkrestart -count=1`: passed.
- `go test ./internal/httpapi -run 'CompleteVPK|VPK' -count=1`: passed.
- `go test ./cmd/panel -count=1`: passed.
- `go test -race ./internal/vpkrestart -count=1`: unavailable because the Windows Go environment has CGO disabled (`-race requires cgo`).
- `npm test -- --run`: 10 files, 119 tests passed.
- `npm run build`: passed; Vite emitted the repository's existing bundle-size advisory.
- `git diff --check`: passed.

## Behavioral Coverage

- Store tests prove upsert deduplication, original container preservation, persistence after database reopen, conditional claim, and pending-task projection.
- Coordinator tests prove active-instance filtering, one-item merge, empty-server restart, three-failure restart, stopped cancellation, and changed-container completion.
- HTTP tests prove new publication registration, duplicate suppression, and non-rollback registration warnings.

## Known Risks

- The full Go suite intermittently reports Windows `TempDir` cleanup failures caused by file handles/directories still being released; this was already documented before the feature and did not produce behavior assertion failures.
- Pending restart projections are intentionally non-expandable rows; the actual restart creates a normal Job with structured logs.
- `cmd/e2e-fixture` has build constraints excluding all files in the default command; it was not a runnable verification target in this environment.
