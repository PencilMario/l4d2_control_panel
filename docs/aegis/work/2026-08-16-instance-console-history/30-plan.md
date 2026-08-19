# Instance Console History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep a bounded console history for each game instance without reattaching to the game supervisor whenever an administrator reopens the console.

**Architecture:** `internal/httpapi` will own a process-local console session hub keyed by instance ID and validated against the current container ID. A session has one upstream supervisor attachment, one bounded byte history, serialized command writes, and bounded subscriber queues. Every browser WebSocket receives a snapshot and subsequent frames from that session; changing containers or an upstream EOF retires the session.

**Tech Stack:** Go, Gorilla WebSocket, `net.Pipe` integration tests, Playwright fixture console.

**Baseline / Authority Refs:** User-approved design in this task; `CONTEXT.md`; `runtime/supervisor.py`; `internal/httpapi/server.go`; `internal/httpapi/server_test.go`; `web/e2e/control-panel.spec.ts`.

**Compatibility Boundary:** Keep the authenticated `/api/instances/{id}/console` WebSocket endpoint, text/binary command input, frontend protocol, and follow behavior. History remains process-local and is discarded on Panel restart; it is neither persisted nor exposed by a new API.

**Verification:** `go test ./internal/httpapi -run Console -count=1`; `go test ./...`; `npm --prefix web test -- --run src/app/consoleBuffer.test.ts src/app/App.test.tsx`; `npm --prefix web run build`; `npm --prefix web run e2e -- --grep 'console'` with the fixture server when available.

---

### Task 1: Console Session Regression Tests

**Files:**
- Modify: `internal/httpapi/server_test.go`
- Create: `internal/httpapi/console.go`

**Why this task exists:** An administrator must be able to close and reopen an instance console without causing the supervisor to replay its source buffer or losing command delivery.

**Impact / Compatibility:** The public WebSocket URL and frame protocol remain unchanged. Tests exercise the handler with an attacher that counts upstream attachments and separately controls each instance stream.

**Repair Track:**
- Root cause: `consoleSocket` creates and closes an `AttachSupervisor` stream for every browser WebSocket.
- Canonical owner: a package-private HTTP API console session hub.
- Smallest necessary change: subscribe browser clients to a shared session instead of directly attaching per request.

**Retirement Track:**
- Retire the direct per-request `AttachSupervisor` path from `consoleSocket`.
- Do not retain it as a fallback; a missing or failed shared session returns the existing attach failure response.

- [ ] **Step 1: Write failing reuse and isolation tests**

```go
func TestConsoleWebSocketReusesInstanceHistoryWithoutReattaching(t *testing.T) {
    // Open, receive upstream output, close, reopen, and assert AttachSupervisor was called once.
    // The second client receives the bounded snapshot and new output.
}

func TestConsoleWebSocketDoesNotShareHistoryAcrossInstances(t *testing.T) {
    // Attach two instance IDs and assert frames from one never reach the other.
}
```

- [ ] **Step 2: Run tests to verify RED**

Run: `go test ./internal/httpapi -run 'TestConsoleWebSocket(Reuses|DoesNotShare)' -count=1`

Expected: FAIL because the handler opens a fresh upstream attachment for the second browser connection.

### Task 2: Bounded Shared Console Sessions

**Files:**
- Create: `internal/httpapi/console.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`

**Why this task exists:** The Panel needs one canonical, bounded owner for each instance's live console history and command stream.

**Impact / Compatibility:** The hub is private to `httpapi`; no persisted data, frontend API, authentication rule, or Docker client contract changes. A container-ID mismatch closes the old session before a new attachment is created.

- [ ] **Step 1: Implement bounded history and subscription lifecycle**

```go
const maxConsoleHistoryBytes = 1 << 20

type consoleHub struct { /* sessions by instance ID and ConsoleAttacher */ }
type consoleSession struct { /* container ID, stream, history, subscribers, write lock */ }

func (h *consoleHub) Subscribe(ctx context.Context, instanceID, containerID string) ([]byte, <-chan []byte, func(), error)
func (s *consoleSession) Write(payload []byte) error
```

The reader appends each upstream frame to a byte-limited rolling buffer before broadcasting copied frames. Slow subscribers are disconnected rather than allowing an unbounded queue. Upstream EOF retires its session and closes subscribers.

- [ ] **Step 2: Replace direct stream proxying in `consoleSocket`**

```go
history, updates, unsubscribe, err := s.consoles.Subscribe(r.Context(), instance.ID, instance.ContainerID)
// Upgrade once, write history, forward updates, and serialize command writes through the session.
```

- [ ] **Step 3: Run targeted tests to verify GREEN**

Run: `go test ./internal/httpapi -run 'TestConsoleWebSocket' -count=1`

Expected: PASS, including the existing command-proxy test plus session reuse and isolation tests.

### Task 3: Regression Verification

**Files:**
- No production file changes unless verification exposes a defect.

**Why this task exists:** Console WebSockets cross backend state ownership and the React terminal; verification must prove the same public journey still works.

- [ ] **Step 1: Run Go package and repository regressions**

Run: `go test ./internal/httpapi -count=1` then `go test ./...`

- [ ] **Step 2: Run frontend unit/build regressions**

Run: `npm --prefix web test -- --run src/app/consoleBuffer.test.ts src/app/App.test.tsx` then `npm --prefix web run build`

- [ ] **Step 3: Run the console browser journey**

Run the fixture server and execute `npm --prefix web run e2e -- --grep 'console'`.

The journey must still render initial output, preserve follow intent, forward `status`, and reopen the same modal successfully.

**Residual Risk:** The new history is intentionally process-local. A Panel restart or game container replacement starts a fresh session and requires the game supervisor's bounded buffer to rehydrate history once.
