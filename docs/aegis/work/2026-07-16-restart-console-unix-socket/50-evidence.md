# Verification Evidence

## Baseline

- `go test ./... -count=1`: all packages except `runtime` passed; Windows could not start the generated `runtime.test.exe` because another process briefly held the file.
- `go test ./runtime -count=1`: passed on immediate isolated rerun.
- `npm test -- --run`: 6 files and 96 tests passed.

## RED / GREEN

- Unix attach RED: `go test ./internal/docker -run TestUnixEngineAttachSupervisorUsesSocketTransport -count=1 -v` failed with `dial tcp: lookup docker: no such host`, proving hijack bypassed the configured Unix Socket.
- Unix attach GREEN: `go test ./internal/docker -run "Test(UnixEngineAttachSupervisorUsesSocketTransport|AttachSupervisorHijacksFixedExecStream)$" -count=1 -v` passed both Unix and TCP attach tests.
- Restart RED: Docker client and lifecycle restart tests failed on `docker POST /containers/container-1/stop: 304 Not Modified`, while the 502 propagation assertion passed.
- Restart GREEN: the expanded Docker/lifecycle target passed 4 tests covering 304 success, 502 propagation, existing supervisor-first Stop, and Restart reaching running.

## Regression

- `go test ./internal/docker ./internal/lifecycle ./internal/httpapi -count=1`: passed all three related packages.
- First `go test ./... -count=1`: assertions passed except Windows failed to remove a non-empty `internal/updates` temporary backup directory during cleanup.
- `go test ./internal/updates -run TestPipelineJournalAvoidsUnrelatedNestedDirectoryBackup -count=5`: passed all 5 repetitions.
- Clean isolated `go test ./... -count=1` rerun: passed every package in 25.5 seconds.
- `npm test -- --run`: 6 files and 96 tests passed.
- `npm run build`: passed; retained the existing advisory that the 619.16 kB JS chunk exceeds 500 kB.
- Desktop E2E exposed the ZIP import status strict-locator race twice. Both screenshots showed the correct replacement tree, success text, and 3 staged changes. The assertion was narrowed to that exact visible text without changing production behavior.
- `npm run e2e -- --project=desktop`: 1/1 passed after the locator repair.
- `npm run e2e -- --project=mobile`: 1/1 passed after the locator repair.
- Repaired-path stress check: Unix/TCP attach, 304/502 Stop, and lifecycle Restart targets passed 10 consecutive repetitions.
- Cleanup: `git diff --check` passed, port 18082 had no owning process, and ignored `web/dist` plus `web/test-results` outputs were removed.

## Residual risk

- The local Windows Docker daemon is unavailable, so final verification cannot inspect or restart the user's deployed Linux containers directly.
