# Evidence Bundle Draft

Updated: 2026-07-16 03:27 +08:00

## Acceptance Coverage

- Content tests cover exact-path ZIP round trips, empty directories, complete workspace replacement, sole top-level directory preservation, unchanged snapshots/applied manifest, rollback and archive security/limit failures.
- HTTP tests cover authenticated GET/POST routes, destructive confirmation, media type, stable errors, missing instances, auditing and download headers.
- React tests cover warning text, cancel, raw ZIP upload, no automatic apply, Blob download/name/revocation and in-flight instance ownership.
- Playwright downloads a non-empty `private-files-<id>.zip`, imports a fixed ZIP containing `imported/new.cfg`, verifies old staged files disappear and proves the game directory retains the previously applied file while the imported file remains staged only.

## Final Commands

```text
go test ./internal/content ./internal/httpapi -count=1
```

Exit 0 in 15.7 seconds: `internal/content` passed in 13.518 seconds and `internal/httpapi` passed in 7.487 seconds.

```text
go test ./... -count=1
```

Exit 0 in 35.1 seconds. Every package passed; packages without tests were reported normally.

```text
cd web
npm test -- --run
npm run build
```

Vitest passed 5 files and 87 tests in 8.91 seconds. TypeScript and Vite production build passed, transforming 2,344 modules in 768ms. Vite retained the existing advisory that the main minified chunk exceeds 500 kB.

```text
npm run e2e -- --project=desktop
npm run e2e -- --project=mobile
```

Run sequentially because both fixtures bind `127.0.0.1:18082`. Desktop passed 1/1 in 24.2 seconds (journey 18.0 seconds); mobile passed 1/1 in 25.6 seconds (journey 20.5 seconds).

## Recovery Regression

The first broad run exposed a real same-process recovery deadlock: `TestRecoverRollsBackUncommittedDeployment` retained the private transaction lease while the new startup scan tried to acquire that lease for an instance with no private recovery work. `PrivateManager.Recover` now discovers `apply-*` or `restore-*` work before taking the instance lease. The private recovery implementation remains the sole owner once a candidate exists.

The regression test failed before the fix with `recovery waited for a clean instance lease`, then passed after the fix. The formerly hanging update test passed in 0.868 seconds. The related content/update recovery matrix passed, and `go vet ./internal/content ./internal/updates` exited 0.

Two earlier broad attempts also observed the repository's intermittent Windows `testing.TempDir` cleanup failure (`directory is not empty`). No behavior assertion failed in those cases, and the final exact focused and full commands above both passed without retries.

## Ownership And Scope Audit

- `internal/content/private_archive.go` is the sole ZIP import/export behavior owner.
- Import and snapshot restore both publish through `replacePrivateWorkspaceLocked`.
- HTTP handlers delegate archive behavior and do not duplicate extraction or replacement logic.
- Archive import contains no `ApplyChanges` call; `/private/apply` remains only on the explicit React apply control.
- Warning text states that ZIP-absent files and unapplied changes are deleted, snapshots remain, and import does not automatically apply to the game directory.
- `git diff --check` exited 0.

## QA Closure

- No temporary instrumentation remains and no E2E server process was left running.
- Compatibility boundary stayed intact: single-file operations, resumable upload, snapshots, applied manifest, lower-layer restoration and manual apply remain active.
- Residual risk is limited to the pre-existing Vite bundle-size advisory and the intermittent Windows cleanup behavior observed before the final passing matrix.
- Confidence: A. Direct unit, integration and desktop/mobile real-HTTP evidence covers the main journey and destructive boundary.
