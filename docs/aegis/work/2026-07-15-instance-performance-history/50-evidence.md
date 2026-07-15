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
- Task 6 RED/GREEN: missing panel, history append, RFC3339 ordering, stale poll/session effects, StrictMode replay and history/overview coupling were reproduced before fixes.
- Task 6 regression: 46 frontend tests pass; three consecutive full-suite runs and 20 focused runs passed during async-race repair; production Vite build passes.
- Task 6 reviews: spec and quality approved after generation-safe history ownership, StrictMode cancellation, independent history bootstrap and memo-stable Recharts props.
- Task 6 residual: production JS is about 587 kB minified / 177 kB gzip and triggers Vite's 500 kB warning. Recharts is immediately visible core UI; no lazy split was added without measured startup evidence.
- Task 7 E2E RED: the first `npm run e2e` run failed both desktop and mobile before reaching the new metrics assertions because the preserved journey implicitly expected `coop`/8 while the current create form defaults to `versus`/32. This exposed test isolation drift, not a product defect. After explicitly selecting the intended values, a focused desktop run reached the new layout assertion and failed with expected fixed wrapper height 220 versus the deployed CSS contract of 210 pixels (190 on mobile). The assertion was corrected to the actual responsive fixed-height contract; no product file was changed.
- Task 7 E2E GREEN: fresh `npm run e2e` passed 2/2 projects/tests in 27.9 seconds (desktop 10.9 seconds, mobile 10.9 seconds). The real HTTP fixture journey verified login, overview navigation, running state, map `c2m1_highway`, players `1 / 8`, selected package, stop/config/console/player actions, CPU `12.5%`, memory `768 MiB / 2 GiB (37.5%)`, RX/TX rates and totals, disk read/write rates and totals, PID 24, uptime `1h 0s`, A2S `2.5 ms`, a two-point history preserving first-point null gaps and second-point zero rates, default CPU mode, all four `aria-pressed` mode transitions, network and disk legends, fixed chart heights, card/control/action bounds and absence of horizontal page overflow on desktop and 390-pixel mobile viewports.
- Task 7 overview-boundary follow-up GREEN: fresh `npm run e2e` passed 2/2 projects/tests in 27.7 seconds (desktop 11.0 seconds, mobile 10.7 seconds) without a product change. While still on Overview, the test individually proved all four mode buttons and every available action button stay within the instance card and current viewport with one-pixel tolerance, retain `scrollWidth <= clientWidth`, and remain accompanied by an in-card chart wrapper/control group. It also proved both `document.documentElement` and `document.body` have `scrollWidth <= clientWidth` in desktop and 390-pixel mobile projects; the later Jobs-page overflow check is retained as separate preserved-journey coverage.

## Regression evidence

- `go test -count=1 ./...` — exit 0; 30 packages passed and one package reported no test files.
- `go vet ./...` — exit 0 with no diagnostics.
- `go test -count=1 -tags=e2e ./cmd/e2e-fixture` — exit 0; one package passed.
- `cd web && npm test -- --run` — exit 0; 3/3 files and 46/46 tests passed in 7.61 seconds.
- `cd web && npm run build` — exit 0; 2,342 modules transformed; production JS 587.41 kB / 176.94 kB gzip and CSS 17.97 kB / 4.67 kB gzip. Vite emitted the known chunk-size warning.
- Latest `cd web && npm run e2e` — exit 0; 2/2 tests passed in 27.7 seconds across desktop and mobile projects.
- `docker compose --env-file .env.example config --quiet` — exit 0 with no diagnostics.
- Linux amd64, `CGO_ENABLED=0` cross-builds of `./cmd/panel` and `./cmd/socket-proxy` — exit 0; temporary outputs were 17,186,282 and 9,760,910 bytes and were removed immediately (`removed=True`).

## Linux capture smoke

- Not executed: no disposable Linux host was available and this Windows host cannot exercise Linux AF_PACKET, Host networking or Unix UID/GID semantics.
- `docker info` confirmed the local daemon is unavailable: connection to `//./pipe/docker_engine` failed because the file was not found. Therefore image build, container runtime, real Unix-Socket connectivity and capture smoke are not claimed.
- `CGO_ENABLED=1 go test -race -count=1 ./internal/metrics` was attempted and failed during toolchain setup because `gcc` is not present in `%PATH%`; no race result is claimed.
- Required manual acceptance on a disposable Linux host: start the stack; verify `/run/l4d2-panel` is `root:10001` mode `0750` and `proxy.sock` is `root:10001` mode `0660`; verify the proxy has only `CAP_NET_RAW` and no TCP listener on 23750; generate traffic across declared game, SourceTV and plugin ports and confirm only the matching RX/TX totals and rates change; stop the instance and confirm traffic/rates freeze; restart and confirm the Docker `StartedAt`/run boundary resets rate calculation; verify A2S, console, players, content and persistent instance paths; bring the stack down and confirm capture processes and the shared socket are cleaned up. These steps are instructions only, not executed evidence.

## Remove / restore and residual risk

- Remove/Restore: no product instrumentation or fallback was added. Playwright output remained under ignored `web/test-results`; Vite `web/dist` remained ignored. Cross-build binaries were written to `%TEMP%` and removed. No Task 7 fixture, Go, Playwright or capture process remained after commands exited.
- Residual risks: Linux runtime ownership/capability/socket/capture behavior is unverified; Windows race coverage is unavailable without `gcc`; the production bundle remains above Vite's 500 kB warning threshold. The real HTTP desktop/mobile administration journey and local static deployment contract have direct automated evidence.
- Confidence: B. Core local behavior has fresh regression and main-journey evidence, but Linux-only operational acceptance and race execution remain blocked by the environment. This evidence is advisory; the controller owns the authoritative completion decision.
