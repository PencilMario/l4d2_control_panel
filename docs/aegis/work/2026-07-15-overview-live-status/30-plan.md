# Overview Live Status and Metrics Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make overview state and metrics come from fresh Docker/A2S observations, while showing unavailable values as unavailable rather than fake zeroes.

**Architecture:** Keep `internal/a2s` as a project-owned adapter over `github.com/rumblefrog/go-a2s`. Add an authenticated per-instance overview response that combines Docker liveness/stats with an A2S_INFO-only player summary; the React page polls that response and keeps observed state separate from persisted lifecycle state.

**Tech Stack:** Go 1.25, Docker Engine API v1.44, `github.com/rumblefrog/go-a2s v1.0.3`, React 19, TypeScript, Vitest.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`, `README.md`, and `10-baseline-readset.md` in this work record.

**Compatibility Boundary:** Preserve instance CRUD/action payloads, the detailed `/players` response and UserID mapping, lifecycle persistence, existing injected React test data and restricted Docker proxy policy.

**Verification:** Focused Go/Vitest red-green runs, `go test -count=1 ./...`, `go vet ./...`, `npm test -- --run`, `npm run build`, `git diff --check`, and deterministic browser/API observation coverage.

---

### Task 1: Correct Docker resource sampling

**Files:**
- Modify: `internal/docker/client.go`
- Test: `internal/docker/client_test.go`

**Why this task exists:** Docker must supply a real two-sample CPU delta; a missing metric must not be confused with a legitimate zero.

**Impact / Compatibility:** Keep `ResourceStats` JSON names and the socket-proxy allowlist unchanged.

**Repair Track:** Remove the `one-shot=true` query that skips Docker's second CPU sample; `Engine.Stats` remains the canonical resource owner.

**Retirement Track:** The one-shot sampling branch is removed with no fallback. Verification checks the exact query and calculation.

- [ ] Add an assertion that `/stats` sends `stream=false` without `one-shot`:

```go
if r.URL.Query().Get("stream") != "false" || r.URL.Query().Has("one-shot") {
    t.Fatalf("query=%s", r.URL.RawQuery)
}
```

- [ ] Run `go test -count=1 ./internal/docker -run TestStatsCalculatesCPUAndMemory` and confirm failure reports `one-shot=true`.
- [ ] Remove `one-shot` from the query in `Engine.Stats`.
- [ ] Re-run the focused Docker test and confirm PASS.

### Task 2: Replace packet parsing and add an A2S_INFO summary

**Files:**
- Modify: `go.mod`, `go.sum`, `internal/a2s/client.go`, `internal/a2s/client_test.go`, `internal/players/service.go`, `internal/players/service_test.go`

**Why this task exists:** Use a proven Source-query implementation and let the overview consume the count already present in A2S_INFO without depending on console UserID mapping.

**Impact / Compatibility:** Preserve the local `a2s.Client`, `a2s.Info`, `a2s.Player` and `players.Snapshot` contracts.

**Repair Track:** `internal/a2s.Client` becomes a thin adapter; `players.Service.Summary` calls only `Info` and returns map/count/capacity.

**Retirement Track:** Hand-written challenge, C-string and packet parsing retires. The detailed player path remains active solely for names, scores, durations and UserID joins.

- [ ] Add a split-packet A2S_PLAYER regression and a service test proving `Summary` does not call A2S_PLAYER or console `status`.
- [ ] Run `go test -count=1 ./internal/a2s ./internal/players` and confirm the split-packet/undefined-summary failures.
- [ ] Add `github.com/rumblefrog/go-a2s v1.0.3` and adapt its results into local structs.
- [ ] Implement:

```go
type Summary struct { Map string; Players, MaxPlayers int }
func (s *Service) Summary(ctx context.Context, id string) (Summary, error)
```

- [ ] Re-run both packages and confirm PASS.

### Task 3: Expose one live overview observation

**Files:**
- Modify: `internal/httpapi/server.go`, `internal/httpapi/server_test.go`, `cmd/e2e-fixture/main.go`

**Why this task exists:** The browser needs one contract that distinguishes persisted state, Docker liveness, A2S health and nullable metrics.

**Impact / Compatibility:** Add `GET /api/instances/{id}/overview`; do not change existing routes.

**Repair Track:** Docker `Running` owns container liveness, A2S_INFO success owns the live `running` state and player values, and resource/A2S failures produce nullable fields plus issues.

**Retirement Track:** The browser's independent `/resources` + detailed `/players` overview join retires; both routes remain for compatible direct consumers and player management.

- [ ] Add authenticated HTTP tests for stale-persisted-running/actually-stopped state and running responses with nonzero stats plus A2S_INFO count.
- [ ] Run `go test -count=1 ./internal/httpapi` and confirm the new route returns 404.
- [ ] Add `Summary` and `Running` to provider contracts and return:

```go
type instanceOverview struct {
    ActualState domain.InstanceState `json:"actual_state"`
    ContainerRunning bool `json:"container_running"`
    Map string `json:"map,omitempty"`
    Players *int `json:"players"`
    MaxPlayers *int `json:"max_players"`
    CPUPercent *float64 `json:"cpu_percent"`
    MemoryBytes *uint64 `json:"memory_bytes"`
    Issues []string `json:"issues,omitempty"`
}
```

- [ ] Run the focused HTTP tests and then `go test -count=1 ./internal/httpapi ./cmd/...`.

### Task 4: Poll and render observed values truthfully

**Files:**
- Modify: `web/src/app/App.tsx`, `web/src/app/App.test.tsx`

**Why this task exists:** The visible main journey must update without a lifecycle action and must render failed observations as `--`, not `0.00`.

**Impact / Compatibility:** Keep injected test/demo instances valid and keep card actions keyed to live container liveness when available.

**Repair Track:** The status label/class and running aggregate use `observed_state`; numeric fields accept `null`; a cleaned-up interval refreshes observations.

**Retirement Track:** Persisted-state-only status and silent `.catch(() => null)` metric zeroing retire. Persisted state remains as the fallback for configuration and actions when no observation exists.

- [ ] Add Vitest coverage where persisted `running` is observed as `stopped`, and where A2S_INFO count/stats render then refresh.
- [ ] Run `npm test -- --run web/src/app/App.test.tsx` (or `npm test -- --run src/app/App.test.tsx` from `web/`) and confirm failure.
- [ ] Fetch `/overview` for every instance concurrently, poll every five seconds with cleanup, and render nullable values with `--`.
- [ ] Re-run focused Vitest and confirm PASS.

### Task 5: Regression and handoff evidence

**Files:**
- Modify: `docs/aegis/work/2026-07-15-overview-live-status/40-atomic-tasks.md`, `50-evidence.md`

**Why this task exists:** The cross-layer contract needs fresh, reviewable evidence and an explicit live-host residual risk.

**Impact / Compatibility:** No product behavior change in this task.

- [ ] Run `go test -count=1 ./...` and `go vet ./...`.
- [ ] Run `npm test -- --run` and `npm run build` from `web/`.
- [ ] Run `git diff --check` and inspect `git status --short` plus the complete diff.
- [ ] Record exact red/green/regression results, side-effect review and the unavailable local Docker/SRCDS live check.
