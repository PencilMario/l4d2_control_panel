# A2S Defense Helper Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an IPv4-only A2S firewall Helper that can be enabled from system settings, automatically protects every game and SourceTV port, and prevents newly exposed instances from starting without confirmed protection.

**Architecture:** A host-networked, `NET_ADMIN`-only Go Helper accepts a strict versioned configuration over a group-restricted Unix socket and owns only project-named iptables chains. The Panel persists desired state, derives protected ports from instances, reconciles Helper state, gates lifecycle start while enabled, and exposes status through the existing system settings UI.

**Tech Stack:** Go 1.24+, net/http over Unix sockets, iptables 1.8 (`nf_tables` backend supported), Docker Compose, SQLite, React 19, TypeScript, Vitest, Playwright.

**Baseline / Authority Refs:** `CONTEXT.md`; `docs/aegis/specs/2026-08-05-a2s-defense-helper-design.md`; `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`; `docker-compose.yml`; `deployment_test.go`; `internal/overlayfs`; `internal/lifecycle`; `internal/store`; `internal/httpapi/server.go`; `web/src/app/App.tsx`.

**Compatibility Boundary:** Disabled installations behave exactly as before. Only the new Helper receives `NET_ADMIN`; Panel and game containers remain unprivileged. Only project-owned IPv4 chains and one INPUT jump may change. Existing instance, update, A2S client, Docker, overlay, and general health contracts remain stable except for the enabled-defense start gate.

**Verification:** `go test -count=1 ./...`; `go vet ./...`; `npm test -- --run`; `npm run build`; `go test -tags=integration ./internal/a2sdefense` inside an isolated network namespace on 安可服; `docker compose --env-file .env.example config --quiet`; remote Compose capability and live rule checks.

---

## File Ownership Map

- `internal/a2sdefense/types.go`: versioned configuration, status, counters, policy constants, validation.
- `internal/a2sdefense/rules.go`: deterministic project-owned iptables restore programs and status parsing.
- `internal/a2sdefense/runner.go`: fixed-command execution boundary and transactional apply/cleanup.
- `internal/a2sdefense/server.go`: strict Helper Unix-socket HTTP API.
- `internal/a2sdefense/client.go`: Panel Unix-socket client.
- `internal/a2sdefense/coordinator.go`: desired-state persistence, port derivation, reconciliation, status, and start gate.
- `cmd/a2s-defense-helper/main.go`, `a2s-defense-helper/Dockerfile`: isolated Helper process and image.
- `internal/store/a2s_defense.go`: desired setting and revision persistence in `system_settings`.
- `internal/lifecycle/service.go`: optional defense gate before container creation/start.
- `internal/httpapi/server.go`: authenticated settings API and reconciliation after instance mutations.
- `web/src/app/A2SDefenseSettings.tsx`: focused settings control and status panel.
- `docker-compose.yml`, `deployment_test.go`: capability and socket isolation contract.

### Task 1: Define The Fixed Policy And Deterministic Rules

**Files:**
- Create: `internal/a2sdefense/types.go`
- Create: `internal/a2sdefense/rules.go`
- Create: `internal/a2sdefense/rules_test.go`

**Why this task exists:** The privileged boundary must be generated from fixed product policy, not user-controlled rule text. The original sample has unreachable tracking, logs normal traffic, and omits `0x56`.

**Impact / Compatibility:** No runtime integration yet. Generated rules must reference only project chains and preserve unrelated firewall state.

**Verification:** `go test ./internal/a2sdefense -run 'Test(Config|Rules)' -count=1`

- [ ] Write failing table tests for strict version/port/revision validation and normalized sorted ports.
- [ ] Run the focused tests and confirm missing types/functions fail compilation.
- [ ] Add `Config`, `Status`, `Counters`, `Policy`, `NormalizeConfig`, and fixed version 1 constants.
- [ ] Write failing golden assertions that require signatures `54/55/56/57/69`, protected-port matching, aggregate `dstport` hashlimit, per-source hashlimits, recent marking before drop, drop-only logging, and final RETURN.
- [ ] Implement `BuildEnableRestore(Config)`, `BuildDisableRestore()`, and stable project chain/name constants without accepting raw rule fragments.
- [ ] Run focused tests and commit: `feat(a2s): 生成固定 IPv4 防御规则`.

Representative validation contract:

```go
type Config struct {
    Version  int   `json:"version"`
    Enabled  bool  `json:"enabled"`
    Ports    []int `json:"ports"`
    Revision int64 `json:"revision"`
}

func NormalizeConfig(input Config) (Config, error) {
    if input.Version != 1 || input.Revision < 1 { return Config{}, ErrInvalidConfig }
    // Validate 1..65535, reject duplicates, sort a copy.
}
```

### Task 2: Build The Transactional Helper And Unix API

**Files:**
- Create: `internal/a2sdefense/runner.go`
- Create: `internal/a2sdefense/runner_test.go`
- Create: `internal/a2sdefense/server.go`
- Create: `internal/a2sdefense/server_test.go`
- Create: `internal/a2sdefense/client.go`
- Create: `internal/a2sdefense/client_test.go`
- Create: `cmd/a2s-defense-helper/main.go`
- Create: `cmd/a2s-defense-helper/main_test.go`
- Create: `a2s-defense-helper/Dockerfile`

**Why this task exists:** Only a narrow, testable process may translate Panel configuration into host firewall mutations.

**Impact / Compatibility:** The Helper exposes only `PUT /v1/config` and `GET /v1/status`; it has no TCP listener, database, Docker, or arbitrary execution API.

**Verification:** `go test ./internal/a2sdefense ./cmd/a2s-defense-helper -count=1`

- [ ] Write failing runner tests with an injected executor for preflight, apply, stale revision, idempotency, failed apply preserving status, disable, and fixed executable arguments.
- [ ] Implement `Executor`, `CommandExecutor`, and `Manager` using absolute `iptables`, `iptables-restore`, and `iptables-save` paths with context timeouts and `--wait`.
- [ ] Write failing HTTP tests for strict JSON, body limits, unknown fields, extra JSON values, method/path rejection, status codes, and response status.
- [ ] Implement `Server` and `Client` over the same typed contract.
- [ ] Write failing process tests for socket cleanup, mode `0660`, graceful shutdown, and environment defaults.
- [ ] Implement Helper main and an Alpine image containing `iptables`; run as `0:10001` because Linux capability checks and socket ownership require root UID with a restricted group.
- [ ] Run focused tests and commit: `feat(a2s): 添加防御 Helper 与 Unix API`.

The only manager entrypoints are:

```go
type Firewall interface {
    Apply(context.Context, Config) (Status, error)
    Status(context.Context) (Status, error)
}
```

### Task 3: Add Compose Isolation And Deployment Contracts

**Files:**
- Modify: `docker-compose.yml`
- Modify: `deployment_test.go`
- Modify: `cmd/panel/main.go`

**Why this task exists:** The feature is safe only if `NET_ADMIN` remains exclusive to the purpose-built Helper and communication stays on a Unix socket.

**Impact / Compatibility:** Existing socket-proxy, overlay-helper, Panel networking, and published ports remain unchanged.

**Verification:** `go test . -run 'TestControlServices|TestA2SDefense' -count=1`; `docker compose --env-file .env.example config --quiet` on 安可服.

- [ ] Extend deployment tests first to require `a2s-defense-init`, a dedicated named volume, no host mounts, host networking, read-only root, `NET_ADMIN` only, no `privileged`, no published ports, and a Panel socket mount.
- [ ] Run tests and confirm Compose assertions fail.
- [ ] Add init and Helper services, writable socket/xtables-lock volume, health dependency, build definition, and Panel socket environment.
- [ ] Construct the Helper client in `cmd/panel/main.go` without changing existing clients.
- [ ] Run deployment tests and commit: `build(a2s): 隔离防御 Helper 权限与套接字`.

### Task 4: Persist Desired State And Reconcile Ports

**Files:**
- Create: `internal/store/a2s_defense.go`
- Modify: `internal/store/store_test.go`
- Create: `internal/a2sdefense/coordinator.go`
- Create: `internal/a2sdefense/coordinator_test.go`
- Modify: `cmd/panel/main.go`

**Why this task exists:** SQLite owns intent while the Helper owns actual state; reconciliation must bridge them without giving the Helper database access.

**Impact / Compatibility:** Reuse `system_settings`; no schema migration. Plugin ports remain excluded. Failed post-mutation sync becomes pending rather than rolling back instances.

**Verification:** `go test ./internal/store ./internal/a2sdefense -run 'Test(A2SDefense|Coordinator)' -count=1`

- [ ] Write failing store tests for default disabled state, monotonic revision, successful summary, pending/error state, and independent settings keys.
- [ ] Implement `A2SDefenseSettings`, getters, and transactional setters in a focused store file.
- [ ] Write failing coordinator tests for port derivation, enable apply-before-persist, disable, reconciliation, failed sync persistence, stale Helper revision recovery, and periodic retry cancellation.
- [ ] Implement `Coordinator` with `Enable`, `Disable`, `Reconcile`, `Status`, `Start`, and `Stop`.
- [ ] Wire coordinator startup and shutdown in `cmd/panel/main.go`.
- [ ] Run focused tests and commit: `feat(a2s): 持久化并对账防御状态`.

### Task 5: Gate Instance Start And Trigger Reconciliation

**Files:**
- Modify: `internal/lifecycle/service.go`
- Modify: `internal/lifecycle/service_test.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`

**Why this task exists:** Enabled defense must fail closed for newly exposed ports while ordinary instance mutations remain recoverable during Helper outages.

**Impact / Compatibility:** The gate is optional; nil/disabled behavior is identical to today. Existing running instances are never automatically stopped.

**Verification:** `go test ./internal/lifecycle ./internal/httpapi -run 'Test.*A2SDefense' -count=1`

**Repair Track:** The old start path creates/starts a host-networked container without a firewall readiness owner. Add the smallest optional gate immediately before exposure.

**Retirement Track:** No old fallback is removed. The unguarded path remains active only when defense is disabled or no coordinator is configured in tests/fixtures.

- [ ] Write failing lifecycle tests for enabled/protected success, enabled/unprotected rejection before Engine start, Helper failure, disabled compatibility, and already-running behavior.
- [ ] Add `DefenseGate` plus `WithDefenseGate`, and call it before container creation/start.
- [ ] Write failing HTTP tests proving create/update/delete trigger reconciliation after persistence and do not roll back on sync error.
- [ ] Add a narrow mutation callback/interface to the server and wire the coordinator.
- [ ] Run focused and full package tests; commit: `feat(a2s): 启动实例前确认防御端口`.

### Task 6: Add Authenticated Settings API And UI

**Files:**
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`
- Create: `web/src/app/A2SDefenseSettings.tsx`
- Create: `web/src/app/A2SDefenseSettings.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles.css`

**Why this task exists:** Administrators need one reliable control surface that distinguishes desired state from effective firewall state.

**Impact / Compatibility:** Adds only `/api/settings/a2s-defense` GET/PUT and one settings section. Fixed thresholds are not exposed.

**Verification:** `go test ./internal/httpapi -run TestA2SDefenseSettings -count=1`; `npm test -- --run A2SDefenseSettings App`; `npm run build`.

- [ ] Write failing API tests for authenticated GET, strict PUT `{enabled:boolean}`, apply-before-response, Helper unavailable, and confirmed state after failure.
- [ ] Implement API handlers and typed response containing desired/effective state, sync state, ports, revision, policy version, counters, blacklist size, applied time, compatibility, and last error.
- [ ] Write component tests for loading, enable/disable, busy lock, failed save retaining confirmed toggle, protected ports, counters, incompatibility, and pending reconciliation.
- [ ] Implement `A2SDefenseSettings` using the existing `api` helper, toggle conventions, icon buttons, and settings-card layout.
- [ ] Mount it on the system settings page and add scoped responsive CSS without nesting cards.
- [ ] Run frontend tests/build and commit: `feat(settings): 管理 A2S 防御状态`.

### Task 7: Verify Real IPv4 Rules On 安可服

**Files:**
- Create: `internal/a2sdefense/integration_linux_test.go`
- Create: `scripts/test-a2s-defense-linux.sh`

**Why this task exists:** Mocked command tests cannot prove `u32`, `hashlimit`, `recent`, nft-backed iptables, and chain cleanup work on the supported kernel.

**Impact / Compatibility:** Tests run only with build tag `integration` and inside a disposable network namespace. They must refuse the root host namespace.

**Verification:** On 安可服, `sudo scripts/test-a2s-defense-linux.sh`; expected PASS and no `L4D2_A2S_*` chains in the host namespace before or after.

- [ ] Write a failing integration test that requires an explicit namespace sentinel and tests apply/status/traffic/blacklist/disable.
- [ ] Add a shell harness that creates a uniquely named namespace, configures veth IPv4 addresses, runs tagged Go tests inside it, and removes the namespace in an EXIT trap.
- [ ] Upload/checkout the feature branch on 安可服 and run the harness.
- [ ] Inspect the host `iptables-save` before and after and record that no host project chain was added.
- [ ] Commit: `test(a2s): 覆盖真实 IPv4 防御规则`.

### Task 8: Documentation, Full Verification, And Evidence

**Files:**
- Modify: `README.md`
- Modify: `.env.example` only if the final socket path requires documentation; do not add a user-facing enable env var.
- Modify: `docs/aegis/INDEX.md`
- Create: `docs/aegis/work/2026-08-05-a2s-defense-helper/50-evidence.md`

**Why this task exists:** Operators must understand capabilities, explicit cleanup, persistence after container removal, and volumetric limitations.

**Impact / Compatibility:** Documentation must not claim upstream DDoS protection, IPv6 support, or automatic kernel-module installation.

**Verification:** All commands in the plan header plus clean `git diff --check` and `git status --short`.

- [ ] Document enablement, required matches, status interpretation, explicit cleanup, uninstall behavior, and limitations.
- [ ] Add the reusable plan and evidence links to the documentation index.
- [ ] Run `gofmt` and all Go tests/vet locally.
- [ ] Run frontend tests and production build locally.
- [ ] Run deployment syntax/behavior, Compose validation, and tagged Linux integration tests on 安可服.
- [ ] Review the final diff for privilege creep, arbitrary command input, unrelated firewall mutation, secret leakage, and disabled-mode regressions.
- [ ] Record exact commands, commit IDs, outputs, and residual risks in the evidence file.
- [ ] Commit: `docs(a2s): 记录防御 Helper 运维与验证`.

## Rollback Surface

- Application rollback: disable through settings while the current Helper is available; this removes only the project jump/chains.
- Emergency cleanup: invoke the fixed Helper disable configuration or documented fixed cleanup command; never flush INPUT.
- Image rollback: keep API version 1 and existing kernel rules until the old Helper confirms reconciliation.
- Database rollback: `system_settings` keys are additive and ignored by older Panel binaries.

## Remaining Unknowns To Resolve During Execution

- Exact Alpine iptables binary paths and `XTABLES_LOCKFILE` behavior in the final image must be proven on 安可服.
- Counter/status parsing across iptables-nft output must use structured `iptables-save -c` grammar tests, not column scraping from `iptables -L`.
- The isolated namespace test must establish whether the host kernel permits all required extensions inside a non-initial namespace; inability is a test-environment limitation, not permission to test against production INPUT.
