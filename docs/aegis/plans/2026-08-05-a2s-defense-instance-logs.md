# Per-instance A2S Defense Logs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Write sampled A2S firewall drop events to each matching instance's `logs/game/a2s_protect.log` without granting Panel or game containers firewall or kernel-log access.

**Architecture:** Fixed NFLOG rules copy rate-limited drop-path packets to the existing `NET_ADMIN` Helper. A proven netlink library feeds a bounded typed event ring exposed by a strict cursor API over the existing Unix socket; Panel polling maps destination ports to instances and appends sanitized lines through the game-log manager.

**Tech Stack:** Go 1.25, `github.com/florianl/go-nflog/v2` v2.3.0, iptables-nft NFLOG, Unix-socket HTTP, SQLite instance store, existing game-log manager/API/UI, Docker Compose.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-08-05-a2s-defense-instance-logs-design.md`; `docs/aegis/specs/2026-08-05-a2s-defense-helper-design.md`; `internal/a2sdefense`; `internal/gamelogs/manager.go`; `internal/store`; `cmd/panel/main.go`; `cmd/a2s-defense-helper/main.go`; `docker-compose.yml`.

**Compatibility Boundary:** IPv4 only. Only the Helper keeps `NET_ADMIN`; no new capability, host log mount, Panel data mount, TCP listener, custom rule input, or game-container permission. Firewall DROP remains effective when event capture, Helper API, Panel polling, or file writes fail. Existing counters, blacklist, reconciliation, startup gate, game-log retention, preview, and download contracts remain stable.

**Verification:** Focused Go tests per task; `go test -count=1 . ./cmd/a2s-defense-helper ./cmd/panel ./internal/a2sdefense ./internal/gamelogs ./internal/httpapi`; `go vet ./...`; frontend tests/build; Compose config; final image and real NFLOG/drop/log test in an isolated namespace on `安可服`.

---

## File Ownership Map

- `internal/a2sdefense/events.go`: event/query types, packet normalization, ring cursor semantics.
- `internal/a2sdefense/nflog_linux.go`: thin adapter around `go-nflog`; build-tagged Linux event source.
- `internal/a2sdefense/server.go`, `client.go`: strict `GET /v1/events` contract over the existing socket.
- `internal/a2sdefense/rules.go`: fixed NFLOG sampling before terminal DROP; retires kernel `LOG` target.
- `internal/a2sdefense/eventlogger.go`: Panel polling, port fan-out, cursor/restart/loss handling.
- `internal/gamelogs/manager.go`: serialized, path-fixed append of `a2s_protect.log`.
- `cmd/a2s-defense-helper/main.go`: listener lifecycle independent from firewall serving.
- `cmd/panel/main.go`: event logger construction and shutdown.
- `a2s-defense-helper/go.mod`, `Dockerfile`: pinned NFLOG dependency and required runtime sources.

### Task 1: Fixed NFLOG Rule And Bounded Event Domain

**Files:**
- Create: `internal/a2sdefense/events.go`
- Create: `internal/a2sdefense/events_test.go`
- Modify: `internal/a2sdefense/rules.go`
- Modify: `internal/a2sdefense/rules_test.go`
- Modify: `internal/a2sdefense/types.go`

**Why this task exists:** Define the safe typed boundary before adding netlink or filesystem behavior.

**Impact / Compatibility:** The old kernel `LOG` target retires. A fixed `NFLOG --nflog-group 100 --nflog-prefix L4D2_A2S_DROP` sampling rule replaces it immediately before DROP with the same `5/minute`, burst 5 limit. Terminal DROP stays unconditional.

**Verification:** `go test ./internal/a2sdefense -run 'Test(Event|BuildEnableRestore)' -count=1`

- [ ] Write failing rule tests requiring `limit -> NFLOG -> DROP`, fixed group/prefix, no kernel `LOG`, and blacklist drops routed through the sampled drop chain.
- [ ] Run focused tests and confirm missing NFLOG behavior.
- [ ] Change only fixed rule generation; keep mark-before-sample and terminal DROP ordering.
- [ ] Write failing event-ring tests for 256 capacity, increasing sequence, boot ID, cursor pagination, overwrite loss, and immutable copies.
- [ ] Implement `Event`, `EventBatch`, `EventRing`, fixed query names, and cursor errors without raw payload fields.
- [ ] Run focused tests and commit: `feat(a2s): 定义防御事件与 NFLOG 采样`.

### Task 2: Linux NFLOG Source And Helper Event API

**Files:**
- Create: `internal/a2sdefense/nflog_linux.go`
- Create: `internal/a2sdefense/nflog_linux_test.go`
- Modify: `internal/a2sdefense/server.go`, `server_test.go`
- Modify: `internal/a2sdefense/client.go`, `client_test.go`
- Modify: `cmd/a2s-defense-helper/main.go`, `main_test.go`
- Modify: `a2s-defense-helper/go.mod`, add generated `go.sum`
- Modify: `a2s-defense-helper/Dockerfile`

**Why this task exists:** Convert privileged packet metadata into a narrow read-only typed API while keeping firewall application independent.

**Impact / Compatibility:** Adds only `GET /v1/events?boot=<id>&after=<sequence>`. Existing endpoints stay unchanged. Listener failure is logged and retried with bounded backoff; it never exits the firewall server or changes rules.

**Repair Track:** Use `go-nflog/v2` as canonical netlink owner; do not hand-roll netlink framing.

**Retirement Track:** Kernel logs and `/dev/kmsg` remain unused; there is no fallback requiring broader privilege.

**Verification:** `go test ./internal/a2sdefense ./cmd/a2s-defense-helper -run 'Test(NFLog|Event|Server|Client|Serve)' -count=1`

- [ ] Add failing packet-parser tests for IPv4 UDP headers, optional IPv4 header length, five signatures, malformed/truncated input, IPv6, and unknown opcodes.
- [ ] Add v2.3.0 to the Helper module and implement the Linux source adapter with fixed group 100 and bounded callback work.
- [ ] Add failing strict API/client tests for boot/cursor parsing, unsupported fields/methods, ring loss, response limits, and cancellation.
- [ ] Extend server/client with the typed events endpoint.
- [ ] Add failing process tests proving listener start/stop, listener failure isolation, and no extra socket/capability requirement.
- [ ] Wire the ring and retrying listener into Helper main; copy only required runtime sources in the dedicated image.
- [ ] Run focused tests and commit: `feat(a2s): 通过 Helper 提供 NFLOG 防御事件`.

### Task 3: Safe Per-instance Log Append And Panel Polling

**Files:**
- Modify: `internal/gamelogs/manager.go`, `manager_test.go`
- Create: `internal/a2sdefense/eventlogger.go`, `eventlogger_test.go`
- Modify: `cmd/panel/main.go`, `cmd/panel/main_test.go`

**Why this task exists:** Place events in the requested existing game-log surface with correct instance ownership and no caller-controlled path.

**Impact / Compatibility:** Adds fixed `AppendA2SDefense(ctx, instanceID, event)` behavior. Existing cleanup locking serializes append/trim/delete. Polling uses current SQLite instances and game/SourceTV ports only; plugin ports remain excluded.

**Verification:** `go test ./internal/gamelogs ./internal/a2sdefense ./cmd/panel -run 'Test(A2S|EventLogger)' -count=1`

- [ ] Write failing manager tests for exact ASCII format, `0640` file, directory preparation, concurrent append serialization, symlink rejection, fixed filename, and cancellation.
- [ ] Implement append under the existing per-instance lock using secure directory validation and `O_APPEND|O_CREATE`.
- [ ] Write failing logger tests for game/SourceTV fan-out, duplicate port owners, stopped instances, plugin exclusion, unknown ports, Helper restart, overwritten cursor, one-instance write failure, cancellation, and no duplicate successful events.
- [ ] Implement a polling `EventLogger` with injected client/store/sink and bounded retry delay.
- [ ] Wire logger lifecycle in Panel main after the existing game-log manager and A2S client.
- [ ] Run focused tests and commit: `feat(a2s): 写入实例防御日志`.

### Task 4: Existing Game-log API And Browser Journey

**Files:**
- Modify: `internal/httpapi/server_test.go`
- Modify: `web/e2e/control-panel.spec.ts`
- Modify: `cmd/e2e-fixture/main.go` only to seed a representative fixed log event.

**Why this task exists:** Prove the requested file is visible through the existing operator workflow without building a second log UI.

**Impact / Compatibility:** No new Panel HTTP endpoint or UI component. Existing game-log tree, preview, download, retention, and size behavior own the file.

**Verification:** `go test ./internal/httpapi -run Test.*A2SProtectLog -count=1`; `cd web && npx playwright test -g 'A2S defense log'`

- [ ] Add an HTTP integration test that appends an event and reads `game/a2s_protect.log` through tree, preview, and download.
- [ ] Add an E2E fixture event and a desktop/mobile Playwright journey selecting `a2s_protect.log` and verifying the sanitized line without overflow.
- [ ] Run tests and commit: `test(a2s): 覆盖防御日志查看流程`.

### Task 5: Linux Proof, Operations, And Deployment

**Files:**
- Modify: `internal/a2sdefense/integration_linux_test.go`
- Modify: `README.md`
- Modify: `docs/aegis/INDEX.md`
- Create: `docs/aegis/work/2026-08-05-a2s-defense-instance-logs/50-evidence.md`

**Why this task exists:** Unit tests cannot prove kernel NFLOG delivery or production file visibility.

**Impact / Compatibility:** Test only in an isolated namespace/container. Production deployment keeps defense disabled until the administrator enables it; no host test traffic is generated.

**Verification:** Full commands from the plan header plus remote image test and deployed authenticated UI/manual log check.

- [ ] Extend tagged Linux test to start NFLOG source, apply sixteen-port rules, flood 27015, assert a typed event, assert packet drops, disable, and prove cleanup.
- [ ] Build final Helper image and run the tagged test on `安可服` with only `CAP_NET_ADMIN`; compare host rules before/after.
- [ ] Document sampled/non-audit semantics, file path/format, loss marker, and existing retention ownership.
- [ ] Run focused/full Go checks, vet, frontend tests/build, Compose config, and `git diff --check`.
- [ ] Review capability/mount diff and record exact evidence/residual risks.
- [ ] Commit: `docs(a2s): 记录实例防御日志运维与验证`.
- [ ] Merge to `main`, push, fast-forward `/opt/l4d2-control-panel`, rebuild services, wait for health, enable defense only with administrator intent, and verify a real or controlled event appears in the matching instance log.

## Rollback Surface

- Disable defense first to remove NFLOG and project rules; log files remain ordinary retained game logs.
- Rolling back Panel stops polling but never changes terminal firewall DROP behavior.
- Rolling back Helper loses only its ephemeral event ring; the existing A2S config API remains version 1.
- Image rollback must disable/reconcile before using a Helper version whose fixed rules still contain kernel LOG instead of NFLOG.

## Remaining Unknowns To Resolve During Execution

- Confirm `xt_NFLOG` and netfilter multicast delivery work inside the same disposable namespace on the supported 5.15 kernel with only `CAP_NET_ADMIN`.
- Confirm `go-nflog/v2` runtime dependency set can be built through the server's configured `goproxy.cn` cache.
- Confirm whether a real production attack sample is available; otherwise deployment proof stops at controlled isolated traffic plus file/API fixture verification.
