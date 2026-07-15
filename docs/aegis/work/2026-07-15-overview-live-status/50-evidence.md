# Evidence bundle draft

## Baseline

- `go test -count=1 ./...` — exit 0 on 2026-07-15 before implementation.
- `npm test -- --run` — exit 0, 2 files and 24 tests passed before implementation.

## Red / green evidence

- Docker RED: `go test -count=1 ./internal/docker -run TestStatsCalculatesCPUAndMemory -v` failed because the request contained `one-shot=true&stream=false`.
- Docker GREEN: the same focused command passed after retaining only `stream=false`.
- A2S/player RED: `go test -count=1 ./internal/a2s ./internal/players` failed with `invalid A2S response` for a split packet and `service.Summary undefined`.
- A2S/player GREEN: the same package command passed after adopting `github.com/rumblefrog/go-a2s v1.0.3` and adding the INFO-only summary.
- HTTP RED: the two live overview cases returned 404 before the route existed.
- HTTP GREEN: `go test -count=1 ./internal/httpapi -run TestInstanceOverviewUsesLiveDockerAndA2SObservations -v` passed running and stale-stopped cases; `go test -count=1 ./internal/httpapi ./cmd/...` also passed.
- React RED: focused App tests kept the stale green `running` card and rendered `0 / 8`, `0.0%`, `0.00 GB` instead of live stopped/unavailable values.
- React GREEN: `npm test -- --run src/app/App.test.tsx` passed 22 tests after switching to the overview contract, nullable rendering and interval refresh.
- React aggregate RED/GREEN: the unavailable A2S sample first remained `0` in the top metric; the focused App suite passed after the aggregate became `--` while a real all-zero sample stayed `0`.
- Missing-container RED/GREEN: the overview first returned stale `running` with an empty container ID; the focused HTTP suite passed after mapping that recovery state to `orphaned`.

## Regression evidence

- `go test -count=1 ./...` — exit 0 across all production Go packages.
- `go vet ./...` — exit 0 with no diagnostics.
- `go test -count=1 -tags=e2e ./cmd/e2e-fixture` — exit 0.
- `npm test -- --run` — exit 0; 2 files and 26 tests passed.
- `npm run build` — exit 0; TypeScript and Vite production build passed.
- `npm run e2e` — exit 0; desktop and mobile real-browser journeys passed against the local HTTP fixture.

## Remove / restore and side effects

- No temporary instrumentation or product fallback remains.
- Existing `/resources` and `/players` routes remain compatible; only their use as the overview's client-side join was retired.
- The build regenerated ignored `web/dist` output only. No Docker daemon or external SRCDS was contacted.

## EvidenceBundleDraft

- Claim checked: persisted overview state and silent zero fallbacks are replaced by live, nullable Docker/A2S observations.
- Direct evidence: protocol, service, HTTP and React RED/GREEN tests plus full regression and browser journeys above.
- Unknown: deployment-host Docker stats timing and a real L4D2 A2S endpoint were not available locally.
- Confidence: B pending a deployment-host smoke; A for deterministic contracts and local regressions.
- Authority: verified development evidence, not a production deployment completion signal.

## Risk / unknown

- Docker is not running on this Windows host, so final live Docker stats and a real SRCDS A2S query require deployment-host confirmation.
- Go race instrumentation could not build locally because Windows race mode requires CGO and no `gcc` is installed; the overview concurrency uses disjoint result variables followed by `WaitGroup` synchronization.
