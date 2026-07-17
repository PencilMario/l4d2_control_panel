# SteamCMD First Install Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make anonymous App 222860 first installation recover from Valve's transient `Missing configuration` response while preserving independent instance game directories.

**Architecture:** `internal/docker` remains the sole owner. Maintenance execution returns a bounded result containing the exit code and recent output; every maintenance container mounts a Panel-owned Steam metadata directory, and only anonymous `InstallGame` classifies the exact configuration error and performs up to three fresh-container attempts.

**Tech Stack:** Go 1.25, Docker Engine HTTP API v1.44, SteamCMD, `httptest` integration-style unit tests.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-17-steamcmd-first-install-recovery-design.md`, `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`, `internal/docker/client.go`, `internal/docker/client_test.go`, and the 2026-07-16 安可 SteamCMD job logs.

**Compatibility Boundary:** Keep the existing Windows-then-Linux anonymous command, credentialed Linux-only install, Linux-only `UpdateGame validate`, maintenance-writer adoption, task log streaming, labels, bridge networking, and per-instance `game/` ownership unchanged.

**Verification:** `go test -count=1 ./internal/docker`, `go test -count=1 ./internal/provisioning ./internal/lifecycle`, `go test -count=1 ./...`, `go vet ./...`, `git diff --check`, then deploy and retry instance `7c991725-7345-42ad-a030-61c8e59a7746` on 安可.

---

### Task 1: Persist Steam metadata and classify maintenance exits

**Files:**
- Modify: `internal/docker/client.go:234`
- Modify: `internal/docker/client_test.go:230`

**Why this task exists:**
- A disposable Steam home makes every new instance depend on Valve returning complete appinfo in that single task.
- Retry decisions need bounded, typed evidence instead of parsing a formatted `error` after logs have been discarded.

**Impact / Compatibility:**
- `runMaintenance` stays private to `internal/docker` and retains existing adoption, wait, log drain, and cleanup behavior.
- The new shared binding stores Steam client metadata only; `instances/<id>/game` remains the install target.

**Repair Track:**
- Root cause: `/home/steam/Steam` is lost with every maintenance container.
- Canonical owner: Docker maintenance-container construction and result classification.
- Smallest change: add one bind and return a bounded `maintenanceResult`.

**Retirement Track:**
- Retire the assumption that a nonzero SteamCMD exit can be represented only as `steamcmd exited with code N` inside `runMaintenance`.
- Keep the one-container writer/adoption path and generic update behavior.

**Verification:** `go test -count=1 ./internal/docker -run 'TestGame(UpdateUsesFixedSteamCMDMaintenanceContainer|InstallBootstrapsWindowsBeforeLinuxWithoutValidate|MaintenanceResult)'`

- [ ] **Step 1: Add failing bind and result tests**

Extend the existing create-request assertions and add a focused result test equivalent to:

```go
wantCache := filepath.Join(root, "panel", "steamcmd", "Steam") + ":/home/steam/Steam"
if !slices.Contains(created.HostConfig.Binds, wantCache) {
    t.Fatalf("binds=%v", created.HostConfig.Binds)
}

result, err := engine.runMaintenance(context.Background(), root, instance, command)
if err != nil || result.StatusCode != 8 || !strings.Contains(result.Output, "Missing configuration") {
    t.Fatalf("result=%#v err=%v", result, err)
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test -count=1 ./internal/docker -run 'TestGame(UpdateUsesFixedSteamCMDMaintenanceContainer|InstallBootstrapsWindowsBeforeLinuxWithoutValidate|MaintenanceResult)'`

Expected: FAIL because the cache bind and `maintenanceResult` API do not exist.

- [ ] **Step 3: Implement the minimal maintenance result and cache bind**

Add private bounded state in `client.go`:

```go
const maintenanceOutputLimit = 64 << 10

type maintenanceResult struct {
    StatusCode int
    Output     string
}
```

Create `<data-root>/panel/steamcmd/Steam` with `0750`, add its bind to `HostConfig.Binds`, append emitted lines to a tail-limited buffer while still calling `jobs.LogContext`, and return `maintenanceResult{StatusCode: result.StatusCode, Output: tail.String()}` after classified container cleanup. Docker/list/start/wait/log errors remain ordinary `error` returns.

- [ ] **Step 4: Run the target tests and verify GREEN**

Run: `go test -count=1 ./internal/docker -run 'TestGame(UpdateUsesFixedSteamCMDMaintenanceContainer|InstallBootstrapsWindowsBeforeLinuxWithoutValidate|MaintenanceResult)'`

Expected: PASS.

- [ ] **Step 5: Commit Task 1**

```bash
git add internal/docker/client.go internal/docker/client_test.go
git commit -m "refactor(steamcmd): expose bounded maintenance result"
```

### Task 2: Retry only anonymous first-install configuration failures

**Files:**
- Modify: `internal/docker/client.go:234`
- Modify: `internal/docker/client_test.go:286`

**Why this task exists:**
- 安可 reproduced the exact Windows-bootstrap error five times in an empty instance, while installed instances continued updating from their manifest.
- The Panel should recover from that specific external transient without hiding authentication, disk, network, cancellation, or Docker failures.

**Impact / Compatibility:**
- Only the anonymous branch of `InstallGame` retries.
- Credentialed install and `UpdateGame` execute exactly once.

**Repair Track:**
- Root cause handling: rebuild the disposable writer after the exact external configuration failure while retaining shared metadata.
- Canonical owner: `InstallGame`, because it knows this is anonymous first installation.

**Retirement Track:**
- Retire operator-driven repeated clicks as the only recovery mechanism.
- Do not add generic SteamCMD retries or copy another instance's manifest.

**Verification:** `go test -count=1 ./internal/docker -run 'TestAnonymousInstall|TestCredentialedGameInstall|TestGameUpdate'`

- [ ] **Step 1: Add failing retry behavior tests**

Use an `httptest.Server` whose first maintenance container logs the exact error and exits 8, then whose second exits 0. Assert two create/start/wait/delete cycles. Add separate tests asserting one attempt for a generic exit and three attempts for repeated exact failures:

```go
const missingConfiguration = "ERROR! Failed to install app '222860' (Missing configuration)"

if createCount != 2 {
    t.Fatalf("createCount=%d", createCount)
}
if err := engine.InstallGame(ctx, root, instance); err != nil {
    t.Fatal(err)
}
```

For exhaustion, assert the returned error contains `after 3 attempts` and `code 8`. For a generic `Disk write failure`, assert `createCount == 1`. Add a credentialed assertion and preserve the existing update path assertion.

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test -count=1 ./internal/docker -run 'TestAnonymousInstall|TestCredentialedGameInstall|TestGameUpdate'`

Expected: FAIL because `InstallGame` currently runs once and converts every nonzero result immediately.

- [ ] **Step 3: Implement exact bounded retry**

Add private constants and predicate:

```go
const anonymousInstallAttempts = 3
const missingConfigurationMessage = "Failed to install app '222860' (Missing configuration)"

func isMissingConfiguration(result maintenanceResult) bool {
    return result.StatusCode != 0 && strings.Contains(result.Output, missingConfigurationMessage)
}
```

Loop only in the anonymous `InstallGame` branch. Before attempts 2 and 3, write a warning through `jobs.LogContext`. Return immediately on context/Docker errors, success, or any nonmatching output. On exhaustion return `steamcmd first install failed after 3 attempts: exited with code 8`.

`UpdateGame` and credentialed `InstallGame` convert their single `maintenanceResult` with a shared private helper so their public error wording remains `steamcmd exited with code N`.

- [ ] **Step 4: Run target and related tests**

Run: `go test -count=1 ./internal/docker`

Expected: PASS with all maintenance adoption, command, log, and retry regressions green.

- [ ] **Step 5: Commit Task 2**

```bash
git add internal/docker/client.go internal/docker/client_test.go
git commit -m "fix(steamcmd): retry anonymous missing configuration"
```

### Task 3: Verify, document evidence, and deploy to 安可

**Files:**
- Create: `docs/aegis/work/2026-07-17-steamcmd-first-install-recovery/50-evidence.md`
- Modify only if verification reveals a defect: `internal/docker/client.go`, `internal/docker/client_test.go`

**Why this task exists:**
- Unit tests prove classification and container construction; the user requested a real repair attempt on the affected host.
- The first instance must remain byte-for-byte outside the deployment and retry path.

**Impact / Compatibility:**
- Deployment rebuilds and recreates only the Panel service and its supporting compiled image.
- The existing first game container and directory are observed before and after; no third instance is created.

**Verification:** Local release checks plus remote job, manifest, size, and container evidence.

- [ ] **Step 1: Run local verification**

Run:

```bash
go test -count=1 ./internal/docker
go test -count=1 ./internal/provisioning ./internal/lifecycle
go test -count=1 ./...
go vet ./...
git diff --check
```

Expected: all commands pass. If the documented Windows temp cleanup race reappears, rerun the exact failed package/test in isolation and record both outputs without weakening tests.

- [ ] **Step 2: Review the diff against the approved design**

Confirm cache binding is metadata-only, output is bounded, retries are exact and anonymous-only, cancellation/Docker errors do not retry, and update/adoption contracts remain unchanged.

- [ ] **Step 3: Commit verification evidence before deployment**

Record commands, results, residual risk, and pre-deployment remote identifiers in `50-evidence.md`, then:

```bash
git add docs/aegis/work/2026-07-17-steamcmd-first-install-recovery/50-evidence.md
git commit -m "docs(steamcmd): record first-install recovery evidence"
```

- [ ] **Step 4: Deploy only the Panel change**

Synchronize the reviewed branch to `/home/steam/l4d2_control_panel` without replacing `.env` or data, run `docker compose build panel`, and run `docker compose up -d --no-deps --force-recreate panel`. Verify `/api/health`, the socket proxy, and the first game container ID/state remain unchanged.

- [ ] **Step 5: Retry the existing failed instance**

Use the authenticated Panel API or existing UI action for instance `7c991725-7345-42ad-a030-61c8e59a7746`. Follow its Job logs until terminal state. Success requires its own `game/steamapps/appmanifest_222860.acf`, reasonable installed size, and a runnable game container. Failure requires preserving all three attempt logs and reporting the exact remaining external condition.

- [ ] **Step 6: Record remote evidence and commit**

Append Panel health, first-instance invariants, retry count, final SteamCMD output, second-instance manifest/size, and cleanup state to `50-evidence.md`.

```bash
git add docs/aegis/work/2026-07-17-steamcmd-first-install-recovery/50-evidence.md
git commit -m "docs(steamcmd): record 安可 recovery verification"
```
