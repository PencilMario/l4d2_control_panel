# Evidence

## Baseline

- `22f36b7`: related Go package tests passed before edits.

## Red-green evidence

- Counter fixture changed from direct `DROP` to both sampled drop chains and failed until `parseCounters` recognized the fixed targets; expected blacklist sum is 18.
- Packet parser/log tests failed to compile before `source_port`, `packet_bytes`, and `sampled_drops_60s` existed, then passed after the additive event contract and formatter were implemented.
- The first rolling-window tests covered cross-port aggregation, independent IPs, expiry, and stale samples. Independent review found an in-window out-of-order gap; `TestSampleWindowCountsOutOfOrderSampleInsideLatestWindow` failed with count 1 before ordered insertion and passed with count 2 after the fix.

## Verification evidence

- `go test ./internal/a2sdefense ./internal/gamelogs ./cmd/a2s-defense-helper ./cmd/panel -count=1`: passed.
- `go test ./internal/httpapi -run TestGameLogsHTTPContract -count=1`: passed with the complete extended log line.
- `go vet ./...`: passed.
- `go test . -run 'Test(A2SDefenseImageBuildsOnlyItsDependencyClosure|ControlServices)' -count=1`: passed.
- `git diff --check`: passed.
- Full `go test ./... -count=1` passed all changed and related packages but retained unrelated Windows `testing.TempDir` cleanup flakes in content/httpapi/updates. The initially failing updates behavior test passed five consecutive focused runs; the related HTTP test also passed alone.
- Cross-compiled Linux integration ran on `安可服` in a disposable Docker network namespace with `--cap-drop ALL --cap-add NET_ADMIN`. It passed while requiring non-zero PLAYER and blacklist counters, one active attacker, non-zero UDP source port, IPv4 packet length 37, a positive sampled 60-second count, terminal DROP behavior, and complete project-chain cleanup.
- The temporary local and remote integration binaries were removed after the run.

## Review evidence

- Independent review found no Critical issues. Its Important in-window out-of-order finding was reproduced with a failing test and fixed. Its Minor Linux metadata assertion gap was closed before the clean remote PASS.

## TodoCheckpointDraft

- Active slice: merge, push, deploy, and production verification.
- Completed: scope, baseline read, design, implementation, red-green cycles, related regression, review, and disposable Linux integration.
- Next: integrate the verified branch and inspect live Helper/Panel behavior.

## DriftCheckDraft

- Scope: unchanged.
- Compatibility: additive event fields; firewall and security boundaries unchanged.
- Retirement: legacy direct-DROP counter recognition remains only for live upgrade compatibility; no duplicate counter owner was introduced.
- Decision: continue to delivery.
