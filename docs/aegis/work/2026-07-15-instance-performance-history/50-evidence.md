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
- Task 3 RED/GREEN: deployment/Unix listener tests failed before the shared socket transport, cleanup safety, scoped Compose assertions, unique Host-network ownership and protected directory behavior existed; all focused tests passed after fixes.
- Task 3 regression: `go test -count=1 ./...`, `go vet ./...` and `docker compose --env-file .env.example config --quiet` passed after the final security change.
- Task 3 Linux build: amd64 `CGO_ENABLED=0` Panel and socket-proxy binaries compiled.
- Task 3 reviews: spec and quality approved after removing unsafe post-close deletion, scoping deployment assertions, proving unique Host networking, enforcing `root:10001` directory/socket permissions, preserving active sockets and removing `EXPOSE 2375`.
- Task 4 RED/GREEN: missing sampler, failure isolation, bounded workers/deadlines, cleanup retries and traffic ownership transitions were each reproduced before fixes; focused metrics tests pass.
- Task 4 regression: `go test -count=1 ./...` and `go vet ./...` passed after the final state-machine fix.
- Task 4 stress: selected timeout, worker, stop/register transition and cleanup tests passed 20-100 repetitions.
- Task 4 reviews: spec and quality approved after runtime unknown semantics, fixed workers, deadline coverage, stopped fast path and retryable traffic ownership were verified.
- Task 4 residual: history is a bounded chronological 720-entry slice and copies entries when full rather than using a circular index; behavior and memory bounds are correct, with small steady-state allocation overhead.
- Task 5 RED/GREEN: missing performance provider/route and unordered history were reproduced before sampler-backed handlers and stable newest-720 mapping.
- Task 5 regression: focused HTTP tests, `go test -count=1 ./...`, tagged e2e fixture tests and `go vet ./...` passed.
- Task 5 reviews: spec and quality approved; overview no longer calls request-time Docker/A2S providers, while `/resources` and `/players` remain.
- Task 5 residual: the explicit history-provider-missing `503` branch is implemented but lacks a dedicated test; production and e2e fixture both inject the provider.

## Regression evidence

- Full post-implementation regression is intentionally not claimed by the planning baseline.

## Linux capture smoke

- Required during Task 7 because the Windows development host cannot exercise Linux AF_PACKET/Host-network capture.
- Docker image build was unavailable during Task 2 because the local Docker daemon is stopped. Race instrumentation was unavailable because Windows race builds require CGO and no `gcc` is installed.
- Linux runtime ownership and real Unix-Socket connectivity remain part of Task 7 smoke because Windows does not enforce Unix UID/GID semantics.

## Remove / restore and residual risk

- No implementation-side temporary instrumentation exists at planning time.
