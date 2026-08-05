# A2S Log Details Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add sampled per-IP 60-second context to A2S logs and repair blacklist packet counters.

**Architecture:** Parse immutable UDP metadata into typed events, aggregate sample timestamps in the Helper before ring insertion, and serialize additive JSON fields through the existing API. Keep iptables as the counter authority and recognize its two fixed sampled drop targets.

**Tech Stack:** Go, iptables-save parsing, NFLOG, existing Unix-socket HTTP API.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-08-05-a2s-defense-instance-logs-design.md` and `docs/aegis/specs/2026-08-05-a2s-defense-log-details-design.md`.

**Compatibility Boundary:** IPv4 only; unchanged rule generation, sampling limit, event cursor semantics, permissions, mounts, and DROP independence.

**Verification:** Focused red-green tests, related package tests, `go test ./...`, `go vet ./...`, and disposable Linux integration on 安可服.

---

### Task 1: Repair blacklist counters

**Files:** Modify `internal/a2sdefense/runner.go`; test `internal/a2sdefense/runner_test.go`.

**Repair Track:** The parser incorrectly requires `-j DROP`; the canonical parser will accept both fixed drop-chain targets and legacy direct DROP.

**Retirement Track:** No fallback is added. Legacy direct DROP recognition stays only for live-rule compatibility and can retire when no supported deployment can retain old rules.

- [ ] Add a regression fixture with both slot drop targets and expect their packet sum.
- [ ] Run the focused test and confirm the old parser fails.
- [ ] Implement fixed-target recognition and confirm the test passes.

### Task 2: Add packet metadata and sampled rolling count

**Files:** Modify `internal/a2sdefense/events.go`, `nflog_linux.go`; create `internal/a2sdefense/sample_window.go`; test `events_test.go`, `sample_window_test.go`.

- [ ] Require `source_port` and validated IPv4 `packet_bytes` in a failing parser test.
- [ ] Add failing tests for cross-port aggregation, IP isolation, 60-second expiry, and out-of-order input.
- [ ] Implement the minimal parser fields and bounded rolling window.
- [ ] Attach the count before ring insertion and run focused tests.

### Task 3: Render the extended game-log line

**Files:** Modify `internal/gamelogs/manager.go`; test `internal/gamelogs/manager_test.go` and affected event API tests.

- [ ] Add a failing expected-line test with all three new fields.
- [ ] Update the fixed formatter and verify producer/consumer tests.

### Task 4: Regression, integration, and delivery

**Files:** Update task evidence and any integration assertions that consume event fields.

- [ ] Run formatting, related tests, full Go tests, and vet.
- [ ] Cross-compile and run the disposable Linux integration on 安可服.
- [ ] Verify Panel log output and blacklist counter on 安可服.
- [ ] Commit, merge into `main`, push, deploy, and recheck service health.
