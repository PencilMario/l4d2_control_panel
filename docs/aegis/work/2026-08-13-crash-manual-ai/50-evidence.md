# Evidence

## Repair Track

- Upload composition in `cmd/panel/main.go` now always enqueues `requestAI=false`, so an Accelerator upload still receives automatic Stackwalk processing without starting an AI request.
- `POST /api/crash-reports/{id}/analyze` defaults to Stackwalk-only and enables AI only when the JSON field `ai` is explicitly `true`.
- The frontend no longer analyzes on report load or selection. The `重新分析` action sends `{"ai":true}`.
- `AutoCrashAnalysis` storage and API compatibility remain intact; its old upload-side AI trigger is retired.

## Local Verification

- Crash-related Go tests passed for `cmd/panel`, `internal/crashanalysis`, `internal/crashreports`, and `internal/httpapi`.
- `go vet ./...` passed.
- `npm test -- --run` passed: 21 test files, 216 tests.
- `npx tsc --noEmit -p tsconfig.json` passed.
- `npm run build` completed successfully; Vite emitted only the existing chunk-size warning.
- `git diff --check` passed.
- Regression coverage includes the upload Stackwalk-only composition, Worker Stackwalk-only execution without AI calls, empty analyze requests, explicit `ai=true`, and frontend no-request-until-click behavior.

The aggregate Windows `go test ./...` command remains subject to unrelated temporary-directory cleanup races observed in existing tests. The affected tests are outside the changed crash behavior; the crash-related packages passed independently. `go test -race` was not run because CGO is unavailable in this environment.

## Remote Deployment

- SSH target: `安可服`.
- Source revision: `61f87e3` in `/home/steam/l4d2_control_panel`.
- Panel image: `sha256:436aa218...`.
- Panel service: running and healthy.
- `/api/health`: `{"containers_running":11,"database":"ok","docker_version":"29.2.1","status":"ok"}`.
- Only the Panel service was rebuilt/recreated. The two game container IDs were unchanged.
- Remote `.env`, `/home/steam/l4d2-panel-data`, and rollback source copies were preserved.

## Remote Browser/API Verification

A fresh Playwright page at `http://100.73.249.118:18081/` loaded the live authenticated data source. The report list contained real reports `d93e70ad...` and `d98b6e83...`; the stale `cccc...` fixture result disappeared after the old page was closed and a new page was opened.

- `GET /api/crash-reports` and the selected report detail returned `200`.
- Stackwalk download returned `200`.
- Loading and selecting the report emitted no `/analyze` request.
- A real `POST /api/crash-reports/d93e70ad079aab21bd2246522afd40a35dc3666796c6d7088576df7963eb67b9/analyze` with `{"ai":false}` returned `202` and created job `a51f2b0d-a581-4752-917f-196941c536dc`.
- That job finished `succeeded` with events `analyzing` and `persisted`; the report retained `stackwalk_status=succeeded`, and its existing `ai_status=succeeded`, `ai_model=deepseek-v4-flash`, and `ai_completed_at=2026-08-12T15:20:52.306999809Z` were unchanged.
- The explicit `重新分析` button was tested with the request intercepted before it reached the server. Playwright captured `POST` body `{"ai":true}`; no additional AI task was created.
- Browser console errors and warnings were zero during the successful live-page load and selection check.

## Retirement Track

- Retired object: the upload-side branch that read `AutoCrashAnalysis` and requested AI work.
- Retained boundary: automatic Stackwalk, Accelerator-compatible upload response, report persistence, the instance setting/API/database field, and explicit manual AI analysis.
- Future retirement trigger: remove `AutoCrashAnalysis` only in a separately approved schema/API compatibility change after existing clients and stored data have been migrated.

## Residual Risk

- A new production Accelerator upload was not generated during this acceptance slice, so the live upload transport itself was not exercised end-to-end. The production callback is covered by `cmd/panel` regression tests and was deployed unchanged except for the `requestAI=false` boundary.
- AI provider latency and timeout behavior were not exercised in this slice; the manual AI request was intentionally intercepted to avoid another external AI call.
- The Windows aggregate test cleanup race and unavailable CGO/race run remain environmental verification gaps.

## Evidence Boundary

- Evidence used: existing task summary, fresh local status/source inspection, fresh SSH health/container/API checks, fresh Playwright live-page network/request inspection, and the fresh real `ai:false` job result.
- Not loaded: full remote logs, full AI payloads, and historical session transcripts; only the report IDs, statuses, timestamps, request bodies, and required health fields were read.
- Confidence: B. The changed trigger boundary has direct regression coverage and live API/browser evidence; the only material gap is a fresh production Accelerator upload.
- Authority: this is verified development and deployment evidence, not an external production approval signal.
