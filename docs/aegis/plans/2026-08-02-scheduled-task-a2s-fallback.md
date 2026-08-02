# Scheduled Task A2S Failure Fallback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every instance-scoped scheduled task using the `wait` player policy execute when player discovery fails instead of blocking indefinitely.

**Architecture:** Keep `Dispatcher.waitForPlayers` as the single policy owner. Change only its query-error outcome for `wait`; successful player queries, `skip`, `force`, task dispatch, locking, and persistence remain unchanged.

**Tech Stack:** Go, standard `context`, existing `players.Service`, Go testing, Docker Compose.

**Baseline / Authority Refs:** `CONTEXT.md`; `docs/aegis/specs/2026-08-02-scheduled-task-a2s-fallback-design.md`; `internal/automation/dispatcher.go`; `internal/automation/dispatcher_test.go`.

**Compatibility Boundary:** Do not change task types, schedule/API schemas, instance locks, task status, or operation behavior. `skip` must remain conservative and `force` must remain unconditional.

**Verification:** `go test ./internal/automation -count=1`; `go test ./...`; remote container health; deployed binary marker; active job count.

---

### Task 1: Change The Wait-Policy Query-Error Outcome

**Files:**
- Modify: `internal/automation/dispatcher_test.go`
- Modify: `internal/automation/dispatcher.go`

**Why this task exists:**
- A failed A2S/player query currently loops forever and holds the instance job lock.

**Impact / Compatibility:**
- Only the `wait` policy's query-error branch changes. A successful query with players still waits one minute.

**Repair Track:**
- Root cause: `waitForPlayers` treats query errors like positive online-player results.
- Canonical owner: `Dispatcher.waitForPlayers`.
- Smallest change: return success after logging a warning when `Online` returns an error under `wait`.
- Verification: targeted red/green test and automation regression suite.

**Retirement Track:**
- Retire the unbounded query-error retry branch.
- Keep the one-minute retry only for successful queries that report online players.
- Keep the stopped-instance bypass because it is an explicit lifecycle fact and avoids an unnecessary query.

- [ ] **Step 1: Write the failing test**

Add a test using a running instance, `failedPlayerQuery`, and a canceled context:

```go
func TestWaitForPlayersForcesWaitPolicyAfterQueryFailure(t *testing.T) {
	instance := domain.Instance{ID: "instance", ActualState: domain.StateRunning, ContainerID: "container", GamePort: 27015}
	playerService := players.NewService(fakeInstanceRepo{instance: instance}, failedPlayerQuery{}, nil, "127.0.0.1")
	d := Dispatcher{Instances: fakeInstanceRepo{instance: instance}, Players: playerService}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := d.waitForPlayers(ctx, domain.ScheduledTask{InstanceID: instance.ID, OnlinePolicy: "wait"}); err != nil {
		t.Fatalf("query failure should force a waiting scheduled task to run: %v", err)
	}
}
```

- [ ] **Step 2: Verify the regression test fails**

Run: `go test ./internal/automation -run TestWaitForPlayersForcesWaitPolicyAfterQueryFailure -count=1`

Expected: FAIL because the existing loop returns `context canceled`.

- [ ] **Step 3: Implement the minimal policy change**

In the `waitForPlayers` loop, separate query errors before checking the player count:

```go
snapshot, err := d.Players.Online(ctx, task.InstanceID)
if err != nil {
	jobs.Logf(ctx, "schedule", joblogs.Warn, "player query failed; forcing scheduled execution target=%s policy=%s error=%q", task.InstanceID, task.OnlinePolicy, err.Error())
	return nil
}
if len(snapshot.Players) == 0 {
	jobs.Logf(ctx, "schedule", joblogs.Info, "player check passed target=%s players=0", task.InstanceID)
	return nil
}
```

- [ ] **Step 4: Verify target and related behavior**

Run: `gofmt -w internal/automation/dispatcher.go internal/automation/dispatcher_test.go`

Run: `go test ./internal/automation -count=1`

Expected: PASS, including the stopped-instance regression and existing scheduler behavior.

- [ ] **Step 5: Run full regression**

Run: `go test ./...`

Expected: all Go packages PASS.

### Task 2: Deploy And Verify The Policy

**Files:**
- Deploy: `internal/automation/dispatcher.go` into `/opt/l4d2-control-panel`
- Build: Linux amd64 `cmd/panel` binary and existing hotfix-derived panel image

**Why this task exists:**
- The remote server must run the new policy before the next scheduled execution.

**Impact / Compatibility:**
- Replace only the panel image; retain the data volume, environment, frontend, socket proxy, overlay helper, and game containers.

- [ ] **Step 1: Build and upload the tested Linux binary**

Run: `$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'; go build -trimpath -ldflags='-s -w' -o "$env:TEMP\l4d2-panel-hotfix" ./cmd/panel`

- [ ] **Step 2: Replace the binary in a temporary container and commit a new panel image**

Use the currently running panel image as the base, copy the binary to `/usr/local/bin/panel`, set mode `0755`, and preserve `USER panel` plus `ENTRYPOINT ["panel"]`.

- [ ] **Step 3: Recreate only the panel service**

Run existing Compose project `l4d2-control-panel` with `--no-build`, injecting the current container's environment without printing secrets.

- [ ] **Step 4: Verify production state**

Check container health is `healthy`, `/api/health` reports `status=ok`, the deployed binary contains `player query failed; forcing scheduled execution`, and SQLite reports zero unexpected `pending` or `running` jobs.
