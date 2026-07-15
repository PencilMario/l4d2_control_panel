# Todo Checkpoint Draft

Updated: 2026-07-16 +08:00

## Current Todo

- Active: Task 3, expose and verify the authenticated archive HTTP contract.
- Pending: React toolbar, desktop/mobile acceptance and final evidence.
- Completed: approved design and plan, isolated baseline, Task 1 transaction helper and Task 2 secure ZIP content owner.

## Evidence Refs

- Design commit: `e220bad`.
- Plan commit: `1b90e14`.
- Baseline: targeted HTTP test passed 5 consecutive runs; serial full Go suite passed with dedicated `GOTMPDIR`; Vitest passed 5 files and 83 tests.
- Task 1: `go test ./internal/content -run 'TestPrivateSnapshot|TestPrivateRestore' -count=1` and `go test ./internal/content -count=1` passed.
- Task 2 RED: `go test ./internal/content -run 'TestPrivateZIP' -count=1` failed because the archive API did not exist.
- Task 2 GREEN: `go test ./internal/content -run 'TestPrivateZIP|TestPrivateSnapshot|TestPrivateRestore' -count=1` and `go vet ./internal/content` passed.
- Task 2 broad run: the full content package intermittently failed the pre-existing Windows manifest atomic-reader test and one temporary-directory cleanup; the focused archive/restoration matrix stayed green.

## Blockers

None. The first exact parallel Go baseline hit the repository's documented Windows temporary-file lock; deterministic diagnostic reruns passed.

## Drift Check Draft

- Scope remains private workspace ZIP import/export only.
- Compatibility boundary remains staged import, manual apply, unchanged snapshots/applied manifest and exact archive root paths.
- Canonical owner remains `PrivateManager`; `private_archive.go` delegates publication to `replacePrivateWorkspaceLocked` and contains no apply/game/snapshot writer.
- Decision: `continue`.

## Next Step

Add authenticated GET/POST archive contract tests, run them RED, then register handlers with stable error mapping.
