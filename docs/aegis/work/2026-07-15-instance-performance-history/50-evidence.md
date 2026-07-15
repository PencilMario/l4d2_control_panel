# Evidence bundle draft

## Baseline

- `go test -count=1 ./...` — exit 0 on 2026-07-15 before implementation; all Go packages passed.
- `npm test -- --run` — exit 0 before implementation; 2 files and 27 tests passed.

## RED / GREEN evidence

- Task 1 RED: `go test -count=1 ./internal/docker -run 'Test(Runtime|Stats)' -v` failed because `Runtime` and the new resource fields did not exist.
- Task 1 GREEN: the same focused command passed after adding runtime timestamp, memory limit, Block I/O and PID parsing.
- Task 1 regression: `go test -count=1 ./internal/docker` and `go test -count=1 -tags e2e ./cmd/e2e-fixture` passed.
- Task 1 review: spec compliance approved; quality review found no Critical or Important issues. Minor residual risk: impossible/extreme `uint64` Block I/O sums are not saturated.
- Task 2 RED/GREEN: missing traffic package/APIs, Stop validation, parser protocol-length bounds and retry-test synchronization were each reproduced before their fixes; expanded pre-existing parser boundary coverage passed without product changes where behavior was already correct.
- Task 2 focused/regression: `go test -count=1 ./internal/traffic ./internal/socketproxy ./cmd/socket-proxy`, `go test -count=1 ./...` and `go vet ./...` passed after the final fix.
- Task 2 stress: parser and capture-retry corrective tests passed 100 repetitions.
- Task 2 Linux build: amd64 `CGO_ENABLED=0` test binaries and `cmd/socket-proxy` executable compiled on Windows; execution remains a Linux-host smoke item.
- Task 2 reviews: spec compliance and code quality approved after fixes for ID validation, parser boundary coverage, test synchronization, IP declared lengths and error-body draining.

## Regression evidence

- Full post-implementation regression is intentionally not claimed by the planning baseline.

## Linux capture smoke

- Required during Task 7 because the Windows development host cannot exercise Linux AF_PACKET/Host-network capture.
- Docker image build was unavailable during Task 2 because the local Docker daemon is stopped. Race instrumentation was unavailable because Windows race builds require CGO and no `gcc` is installed.

## Remove / restore and residual risk

- No implementation-side temporary instrumentation exists at planning time.
