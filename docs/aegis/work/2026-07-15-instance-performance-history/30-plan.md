# Instance Performance Information and History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add detailed live performance information and a one-hour chart to every overview instance while reliably accounting Host-network traffic by declared instance ports.

**Architecture:** Expand Docker inspect/stats, add a packet-counting metrics surface to the existing restricted socket proxy, and move observation ownership into a five-second Panel sampler with per-instance 720-point ring buffers. HTTP reads sampler snapshots; React renders compact metrics and Recharts history modes.

**Tech Stack:** Go 1.24+, Docker Engine API v1.44, Linux AF_PACKET, Chi, React, TypeScript, Recharts, Vitest and Playwright.

**Baseline / Authority Refs:** `CONTEXT.md`, `docs/aegis/specs/2026-07-15-instance-performance-history-design.md`, `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`, `docs/aegis/work/2026-07-15-overview-live-status/50-evidence.md`.

**Compatibility Boundary:** Preserve existing overview JSON fields/routes, Docker whitelist, lifecycle/player/content/update behavior, Host-network game containers, nullable metrics and the Panel-without-raw-Docker-socket invariant.

**Verification:** Focused RED/GREEN tests per task, `go test -count=1 ./...`, `go vet ./...`, `npm test -- --run`, `npm run build`, `npm run e2e`, Compose configuration checks and a disposable Linux capture smoke test.

---

### Task 1: Expand Canonical Docker Runtime Metrics

**Files:**
- Modify: `internal/docker/client.go`
- Test: `internal/docker/client_test.go`
- Modify: `cmd/e2e-fixture/main.go`

**Why this task exists:** Docker must remain the canonical owner of CPU, memory, block I/O, process count and the runtime start marker used to prevent cross-run rate calculations.

**Impact / Compatibility:** Extend `ResourceStats` additively and replace the Boolean-only runtime probe with a richer inspect result. Existing callers still receive the same running truth and existing JSON fields.

**Repair Track:** The current resource contract cannot distinguish memory capacity or counter sessions. Extend the owner instead of adding HTTP-side Docker parsing.

**Retirement Track:** `ResourceProvider.Running` retires after every caller uses `Runtime`; no compatibility adapter remains.

- [ ] **Step 1: Write failing Docker parsing tests**

Add cases asserting this contract:

```go
type RuntimeState struct {
    Running   bool
    StartedAt time.Time
}

type ResourceStats struct {
    CPUPercent       float64 `json:"cpu_percent"`
    MemoryBytes      uint64  `json:"memory_bytes"`
    MemoryLimitBytes uint64  `json:"memory_limit_bytes"`
    BlockReadBytes   uint64  `json:"block_read_bytes"`
    BlockWriteBytes  uint64  `json:"block_write_bytes"`
    PIDs              uint64  `json:"pids"`
}
```

The stats fixture must include multiple `io_service_bytes_recursive` entries and prove that `Read`/`Write` values are summed case-insensitively while unrelated operations are ignored. The inspect fixture must assert RFC3339Nano `StartedAt` parsing.

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 ./internal/docker -run 'Test(Runtime|Stats)' -v`

Expected: FAIL because the new fields and `Runtime` method do not exist.

- [ ] **Step 3: Implement minimal Docker decoding**

Extend `statsResponse` with `memory_stats.limit`, `blkio_stats.io_service_bytes_recursive`, and `pids_stats.current`. Add `Engine.Runtime(ctx, id)` using `/containers/{id}/json`. Keep CPU and cache-subtracted memory calculation unchanged.

- [ ] **Step 4: Verify GREEN and update fixture**

Run: `go test -count=1 ./internal/docker ./cmd/e2e-fixture`

Expected: PASS with CPU, memory, limits, block counters, PIDs and runtime timestamp covered.

- [ ] **Step 5: Commit**

```bash
git add internal/docker/client.go internal/docker/client_test.go cmd/e2e-fixture/main.go
git commit -m "feat(metrics): 扩展 Docker 运行资源指标"
```

### Task 2: Add Declared-Port Traffic Accounting to the Restricted Proxy

**Files:**
- Create: `internal/traffic/model.go`
- Create: `internal/traffic/counter.go`
- Create: `internal/traffic/counter_test.go`
- Create: `internal/traffic/capture_linux.go`
- Create: `internal/traffic/capture_other.go`
- Create: `internal/traffic/client.go`
- Create: `internal/traffic/client_test.go`
- Modify: `cmd/socket-proxy/main.go`
- Modify: `internal/socketproxy/policy.go`
- Test: `internal/socketproxy/policy_test.go`
- Modify: `socket-proxy/Dockerfile`

**Why this task exists:** Host-network containers lack trustworthy per-container Docker network counters; declared ports provide the narrowest useful ownership boundary without packet retention.

**Impact / Compatibility:** The proxy gains one internal metrics namespace and `NET_RAW`, but Docker requests remain governed by the existing whitelist. Packet parsing must never expose payload or address data.

**Repair Track:** Replace the unavailable Docker-network owner with a project-owned declared-port counter inside the existing host boundary.

**Retirement Track:** Do not add or retain a fallback to Docker `networks`; network values remain unavailable if capture fails.

- [ ] **Step 1: Write failing counter tests**

Define the internal protocol:

```go
type Session struct {
    InstanceID string `json:"instance_id"`
    RunID      string `json:"run_id"`
    Ports      []int  `json:"ports"`
}

type Totals struct {
    RunID    string `json:"run_id"`
    RXBytes  uint64 `json:"rx_bytes"`
    TXBytes  uint64 `json:"tx_bytes"`
}

type Packet struct {
    Length  uint64
    SrcPort uint16
    DstPort uint16
}
```

Test TCP/UDP-equivalent port matching, RX/TX direction, same-instance deduplication, unrelated ports, invalid port rejection, run-ID reset, stopped-session freezing and concurrent reads.

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 ./internal/traffic ./internal/socketproxy -v`

Expected: FAIL because the traffic package and internal proxy routes do not exist.

- [ ] **Step 3: Implement counter and internal HTTP client**

Implement `Counter.Register(Session)`, `Counter.Stop(instanceID, runID)`, `Counter.Observe(Packet)` and `Counter.Totals(instanceID)`. Add Unix-Socket client methods with bounded timeouts. Use `PUT /_panel/traffic/{id}`, `DELETE /_panel/traffic/{id}` and `GET /_panel/traffic/{id}`; reject these paths from the Docker whitelist and route them explicitly before the reverse proxy.

- [ ] **Step 4: Implement Linux capture boundary**

Use a Linux-only AF_PACKET reader to enumerate active non-loopback interfaces, decode Ethernet/VLAN/IPv4/IPv6 plus TCP/UDP headers, and emit only `Packet{Length, SrcPort, DstPort}`. The non-Linux build returns `traffic capture unsupported` without breaking Panel development tests. Add the selected pure-Go capture dependency to `go.mod`/`go.sum` only if standard `x/sys/unix` is insufficient.

- [ ] **Step 5: Verify GREEN**

Run: `go test -count=1 ./internal/traffic ./internal/socketproxy ./cmd/socket-proxy`

Expected: PASS; policy tests prove Docker access did not widen and traffic tests prove no payload-bearing type leaves capture parsing.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/traffic internal/socketproxy cmd/socket-proxy socket-proxy/Dockerfile
git commit -m "feat(metrics): 在受限代理统计实例端口流量"
```

### Task 3: Move Proxy Transport to a Shared Unix Socket

**Files:**
- Modify: `docker-compose.yml`
- Modify: `deployment_test.go`
- Modify: `internal/docker/client.go`
- Test: `internal/docker/client_test.go`
- Modify: `cmd/panel/main.go`
- Modify: `README.md`

**Why this task exists:** Host packet visibility and private proxy reachability must coexist without publishing the Docker proxy on a host TCP port.

**Impact / Compatibility:** Panel still never mounts `/var/run/docker.sock`; it mounts only the proxy-owned Unix Socket volume. Game containers and the public Panel port do not change.

**Repair Track:** Replace Compose-service TCP discovery with a filesystem capability shared only by Panel and proxy.

**Retirement Track:** `LISTEN_ADDR` and `tcp://socket-proxy:23750` retire completely; no TCP fallback remains.

- [ ] **Step 1: Write failing deployment and Unix transport tests**

Assert Compose contains a named `panel-proxy-run` volume, `network_mode: host` only for `socket-proxy`, `cap_add: [NET_RAW]`, `cap_drop: [ALL]`, and `DOCKER_HOST=unix:///run/l4d2-panel/proxy.sock`. Assert neither service publishes proxy port 23750.

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 . ./internal/docker -run 'Test(Control|Unix)' -v`

Expected: FAIL on the current private bridge/TCP transport.

- [ ] **Step 3: Implement Unix transport**

Teach `docker.NewEngine` to accept `unix://` by installing an HTTP transport whose `DialContext` opens the proxy socket while retaining the Docker API base URL internally. Configure proxy socket creation with mode `0660`, a shared group, read-only root filesystem and writable socket volume.

- [ ] **Step 4: Verify GREEN and Compose validity**

Run: `go test -count=1 . ./internal/docker ./cmd/panel ./cmd/socket-proxy`

Run: `docker compose --env-file .env.example config --quiet`

Expected: both exit 0; rendered Compose exposes only the Panel HTTP port.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml deployment_test.go internal/docker/client.go internal/docker/client_test.go cmd/panel/main.go README.md
git commit -m "refactor(proxy): 改用共享 Unix Socket"
```

### Task 4: Build the Five-Second Observation Sampler and Ring Buffer

**Files:**
- Create: `internal/metrics/sampler.go`
- Create: `internal/metrics/sampler_test.go`
- Modify: `cmd/panel/main.go`
- Modify: `internal/players/service.go`
- Test: `internal/players/service_test.go`

**Why this task exists:** One owner must calculate rates, enforce run boundaries and retain exactly one hour without browser-dependent duplicate sampling.

**Impact / Compatibility:** Sampling is read-only and must not block lifecycle operations. Existing A2S summary behavior stays intact while exposing measured latency.

- [ ] **Step 1: Write failing sampler tests with a fake clock**

Define:

```go
type Sample struct {
    At                   time.Time `json:"at"`
    RunID                string    `json:"run_id"`
    CPUPercent           *float64  `json:"cpu_percent"`
    MemoryBytes          *uint64   `json:"memory_bytes"`
    MemoryLimitBytes     *uint64   `json:"memory_limit_bytes"`
    MemoryPercent        *float64  `json:"memory_percent"`
    NetworkRXBytesPerSec *float64  `json:"network_rx_bytes_per_sec"`
    NetworkTXBytesPerSec *float64  `json:"network_tx_bytes_per_sec"`
    BlockReadBytesPerSec *float64  `json:"block_read_bytes_per_sec"`
    BlockWriteBytesPerSec *float64 `json:"block_write_bytes_per_sec"`
}
```

Cover first-sample null rates, five-second deltas, counter rollback, run changes, partial provider failures, stop gaps, startup lazy sample, exactly 720 retained points and safe concurrent `Latest`/`History` reads.

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 ./internal/metrics -v`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement sampler ownership**

Create provider interfaces for instance listing, Docker runtime/stats, traffic register/totals and timed A2S summary. Use `StartedAt.UTC().Format(time.RFC3339Nano)` as `RunID`. Sum rate deltas only when run IDs match and elapsed time is positive and bounded. Store immutable copies in a mutex-protected per-instance ring.

- [ ] **Step 4: Wire lifecycle and shutdown**

Start one sampler goroutine from `cmd/panel/main.go`, sample immediately, tick every five seconds, and stop it during graceful shutdown. Register current declared ports for running instances and freeze traffic sessions for stopped instances.

- [ ] **Step 5: Verify GREEN**

Run: `go test -count=1 ./internal/metrics ./internal/players ./cmd/panel`

Expected: PASS with deterministic fake-clock tests and no real Docker/A2S dependency.

- [ ] **Step 6: Commit**

```bash
git add internal/metrics internal/players cmd/panel/main.go
git commit -m "feat(metrics): 采样实例性能并保留一小时历史"
```

### Task 5: Expose Additive Overview and History Contracts

**Files:**
- Modify: `internal/httpapi/server.go`
- Test: `internal/httpapi/server_test.go`
- Modify: `cmd/e2e-fixture/main.go`
- Test: `cmd/e2e-fixture/main_test.go`

**Why this task exists:** The browser needs one coherent current snapshot and bounded authenticated history without owning rate calculations.

**Impact / Compatibility:** Keep `GET /api/instances/{id}/overview` and every existing field. Add `GET /api/instances/{id}/performance-history` as read-only authenticated JSON.

**Repair Track:** Retire request-time Docker/A2S fan-out from `instanceOverview`; the sampler becomes the canonical observation owner.

**Retirement Track:** `WithResources`/`WithPlayers` remain for resource/player endpoints, but overview no longer joins them directly after sampler wiring is complete.

- [ ] **Step 1: Write failing HTTP tests**

Assert overview returns nullable memory limit/percent, network rates/totals, disk rates/totals, PIDs, uptime and A2S latency while preserving current fields. Assert history requires auth, is chronological, contains at most 720 points and preserves null gaps and zeroes.

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 ./internal/httpapi -run 'TestInstance(Overview|PerformanceHistory)' -v`

Expected: FAIL on missing fields and route.

- [ ] **Step 3: Implement sampler-backed handlers**

Add a `PerformanceProvider` option with `Latest(id)` and `History(id)`. Map the latest sample into the additive overview DTO; copy history before encoding. Missing history returns an empty array, while unknown instances remain 404.

- [ ] **Step 4: Update real-HTTP fixture and verify GREEN**

Run: `go test -count=1 ./internal/httpapi ./cmd/e2e-fixture`

Expected: PASS for running, stopped, partial-failure, zero and history cases.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi cmd/e2e-fixture
git commit -m "feat(api): 提供实例性能快照与历史"
```

### Task 6: Render Detailed Metrics and Recharts History

**Files:**
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Create: `web/src/app/PerformancePanel.tsx`
- Create: `web/src/app/PerformancePanel.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles/app.css`

**Why this task exists:** Administrators need scan-friendly current values and a compact visual path to identify one-hour CPU, memory, network or disk spikes.

**Impact / Compatibility:** Preserve card actions, status, map, players and five-second freshness. The chart must have stable dimensions on desktop/mobile and must not resize cards while loading.

- [ ] **Step 1: Install Recharts and write failing component tests**

Run: `cd web && npm install recharts`

Test exported formatters for IEC bytes, bytes/sec, duration and latency. Render zero versus `null`, memory used/limit/percent, RX/TX current and totals, disk read/write current and totals, PIDs/uptime/latency, four segmented modes, two-series legends, and null-gap chart data.

- [ ] **Step 2: Verify RED**

Run: `cd web && npm test -- --run src/app/PerformancePanel.test.tsx src/app/App.test.tsx`

Expected: FAIL because `PerformancePanel` and new overview/history fields do not exist.

- [ ] **Step 3: Implement the focused component**

Create typed `PerformanceSnapshot` and `PerformanceHistoryPoint` props. Use `ResponsiveContainer`, `LineChart`, `Line`, `XAxis`, `YAxis` and `Tooltip`; set `connectNulls={false}`. Use button-based segmented controls with `aria-pressed`, fixed chart height, compact legends and unit-aware tooltip formatting.

- [ ] **Step 4: Wire App data without duplicate full-history downloads**

Extend instance normalization with additive overview fields. Fetch the bounded history once when the overview loads or is re-entered, then append each timestamped latest overview sample to a client-side 720-point ring during the existing five-second cycle. Deduplicate by timestamp and run ID. Keep previous history if the initial history request fails, while current unavailable fields remain `null`; do not download all 720 points every five seconds.

- [ ] **Step 5: Implement responsive styling and verify GREEN**

Run: `cd web && npm test -- --run src/app/PerformancePanel.test.tsx src/app/App.test.tsx`

Run: `cd web && npm run build`

Expected: tests and TypeScript/Vite build exit 0 with no overflow at 320px card width.

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/package-lock.json web/src/app web/src/styles/app.css
git commit -m "feat(web): 展示实例性能指标与历史图表"
```

### Task 7: Verify the Main Journey and Deployment Boundary

**Files:**
- Modify: `web/e2e/control-panel.spec.ts`
- Modify: `README.md`
- Modify: `docs/aegis/work/2026-07-15-instance-performance-history/40-atomic-tasks.md`
- Modify: `docs/aegis/work/2026-07-15-instance-performance-history/50-evidence.md`

**Why this task exists:** Unit coverage cannot prove the complete authenticated overview, responsive chart interaction or Linux capture deployment boundary.

**Impact / Compatibility:** Verification must not contact a shared Docker host or retain temporary capabilities, sockets or capture processes.

- [ ] **Step 1: Add failing Playwright assertions**

Assert a logged-in desktop and mobile user sees CPU, memory, network, disk, PIDs, uptime and latency; can switch all four chart modes; sees RX/TX legends; and retains actions without overlap.

- [ ] **Step 2: Verify browser RED then GREEN**

Run: `cd web && npm run e2e`

Expected before fixture/UI completion: FAIL on missing metrics. Expected after completion: PASS for desktop and mobile projects.

- [ ] **Step 3: Run full deterministic regression**

```bash
go test -count=1 ./...
go vet ./...
cd web
npm test -- --run
npm run build
npm run e2e
cd ..
docker compose --env-file .env.example config --quiet
```

Expected: every command exits 0; no diagnostics, test failures or unexpected warnings.

- [ ] **Step 4: Run disposable Linux smoke**

On a disposable Linux Docker host, start the stack, create or reuse a test instance with known declared ports, send bounded UDP/TCP traffic, and compare proxy totals before/after. Confirm the proxy has only `cap_net_raw`, its Docker endpoint is reachable only through the shared Unix Socket, no packet content endpoint exists, and a new `StartedAt` resets the run totals.

- [ ] **Step 5: Record evidence and side effects**

Write exact commands, exit codes, test counts, Linux kernel/Docker versions, observed counter deltas, unverified interfaces and cleanup confirmation to `50-evidence.md`. Mark every item in `40-atomic-tasks.md` with its final status.

- [ ] **Step 6: Commit**

```bash
git add web/e2e/control-panel.spec.ts README.md docs/aegis/work/2026-07-15-instance-performance-history
git commit -m "test(metrics): 验证实例性能总览流程"
```

## Rollback Surface

- Revert the UI/API sampler while keeping additive Docker fields harmless if chart rollout must pause.
- Revert Host capture and Unix transport together; do not restore a published unrestricted proxy TCP port.
- Removing `NET_RAW` disables only per-instance network values; other performance metrics remain available.
- SQLite has no migration, so rollback does not require data conversion.

## Residual Risk

- Packet visibility varies across Linux interfaces and virtual networking; the disposable-host smoke must cover the deployment's LAN/Tailscale path.
- Port-based attribution cannot include undeclared dynamically allocated plugin ports.
- Panel or proxy restart clears in-memory history and the current run's accumulated network counters by design.
