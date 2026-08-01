# Aria2 Release Download Evidence

## Implemented

- aria2 process adapter with dynamic 1-16 connection settings, default 8.
- URL passed through stdin; GitHub token is exchanged by Go and never passed to aria2 arguments.
- Existing package validation, hashing, publication, reuse, cancellation cleanup, size limit, proxy behavior, and task logging retained.
- Shared configured Release client wired to manual, source, instance, and scheduled paths.
- Strict GET/PUT `/api/settings/downloads` and System Settings control.
- Runtime Alpine image installs aria2.

## Fresh Verification

- `go test ./internal/releases ./internal/store ./internal/httpapi ./internal/automation ./internal/updates ./cmd/panel . -run 'Test(Aria2|FetchLatest|ClientTrusts|ClientRejects|ProxyEnvironment|ReleaseDownloadConnections|DownloadSettings|WithReleaseClient|PanelRuntimeImage)|^$' -count=1`: PASS.
- `npm test -- --run`: PASS, 15 files and 192 tests.
- `npm run build:web`: PASS; existing bundle-size warning remains.
- `git diff --check`: PASS.
- TDD red runs observed missing store/API, downloader, GitHub integration, UI control, Docker dependency, proxy aliases, official asset host, and scheme rejection before each implementation.

## Residual Risk

- This Windows host has neither Docker nor aria2c, so the built Alpine image and a real aria2 transfer could not be executed locally. Unit tests execute the complete adapter contract through an injected runner, and the Dockerfile contract verifies package installation text.
- `go test ./...` passed earlier in the task, but later fresh full runs intermittently failed in unchanged Windows atomic-file tests under `internal/content` and `internal/updates` with file-lock errors. The first failure passed three targeted repetitions; no unrelated production fix was made.
- `go test -race` was unavailable because the current Go environment has CGO disabled.

## Drift Check

- Scope remains limited to GitHub Release asset transfer and its setting.
- Existing Release routes, package identity, deployment semantics, and source terminology are unchanged.
- Direct Go asset-body copying is retired from production. Go HTTP remains for Release metadata and authenticated redirect exchange.
- Decision: implementation evidence is sufficient with bounded runtime-image residual risk.
