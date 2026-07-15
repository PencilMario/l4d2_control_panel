# Evidence bundle draft

## Baseline

- `go test -count=1 ./...` — exit 0 on 2026-07-15 before implementation; all Go packages passed.
- `npm test -- --run` — exit 0 before implementation; 2 files and 27 tests passed.

## RED / GREEN evidence

- Implementation has not started; each task in `30-plan.md` defines its required failing and passing command.

## Regression evidence

- Full post-implementation regression is intentionally not claimed by the planning baseline.

## Linux capture smoke

- Required during Task 7 because the Windows development host cannot exercise Linux AF_PACKET/Host-network capture.

## Remove / restore and residual risk

- No implementation-side temporary instrumentation exists at planning time.
