# Todo Checkpoint Draft

Updated: 2026-07-16 +08:00

## Current Todo

- Active: Task 2, implement the secure content-layer ZIP owner with test-first coverage.
- Pending: HTTP routes, React toolbar, desktop/mobile acceptance and final evidence.
- Completed: approved design and plan, isolated baseline, and Task 1 canonical workspace replacement helper.

## Evidence Refs

- Design commit: `e220bad`.
- Plan commit: `1b90e14`.
- Baseline: targeted HTTP test passed 5 consecutive runs; serial full Go suite passed with dedicated `GOTMPDIR`; Vitest passed 5 files and 83 tests.
- Task 1: `go test ./internal/content -run 'TestPrivateSnapshot|TestPrivateRestore' -count=1` and `go test ./internal/content -count=1` passed.

## Blockers

None. The first exact parallel Go baseline hit the repository's documented Windows temporary-file lock; deterministic diagnostic reruns passed.

## Drift Check Draft

- Scope remains private workspace ZIP import/export only.
- Compatibility boundary remains staged import, manual apply, unchanged snapshots/applied manifest and exact archive root paths.
- Canonical owner remains `PrivateManager`; the inline snapshot publication block retired into `replacePrivateWorkspaceLocked` with no second transaction owner.
- Decision: `continue`.

## Next Step

Write the ZIP round-trip, complete-replacement and invalid-archive tests, run them RED, then add `private_archive.go`.
