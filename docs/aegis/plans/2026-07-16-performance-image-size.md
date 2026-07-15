# Performance Image Size Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the duplicated player metric with the current Docker image size and rename network RX/TX presentation to download/upload.

**Architecture:** The Docker adapter resolves a container's image ID and reads that image's local `Size`. The metrics sampler carries the nullable byte value into the current overview contract; the history projection remains unchanged. React formats the current value with the existing IEC formatter and changes only user-facing network labels.

**Tech Stack:** Go 1.25, Docker Engine HTTP API v1.44, React, TypeScript, Vitest.

**Baseline / Authority Refs:** `CONTEXT.md`; `docs/aegis/specs/2026-07-16-performance-image-size-design.md`.

**Compatibility Boundary:** Existing player fields and player workflows remain available outside the performance panel. Existing performance-history JSON and CPU/memory/network/disk charts remain unchanged. Image lookup failure yields a nullable metric and an issue without blocking other observations.

**Verification:** Focused Go adapter/sampler/API tests, focused Vitest panel/app tests, full Go tests, web tests, web build, and a visual smoke check of the overview panel.

---

### Task 1: Docker image size adapter

**Files:**
- Modify: `internal/docker/client.go`
- Test: `internal/docker/client_test.go`

**Why this task exists:** Docker is the canonical owner of local image metadata; callers should not construct Docker API requests themselves.

**Impact / Compatibility:** Adds one read-only adapter method and leaves container lifecycle behavior unchanged.

**Verification:** `go test -count=1 ./internal/docker -run TestImageSize -v`

- [ ] **Step 1: Write the failing test**

Add an HTTP fixture expectation for `GET /containers/container/json`, returning `{"Image":"sha256:image"}`, followed by `GET /images/sha256:image/json`, returning `{"Size":5368709120}`. Assert `engine.ImageSize(ctx, "container") == 5368709120`, and add an image-inspect failure case.

- [ ] **Step 2: Run test to verify it fails**

Run `go test -count=1 ./internal/docker -run TestImageSize -v` and expect a compile failure because `ImageSize` does not exist.

- [ ] **Step 3: Write minimal implementation**

Add `func (e *Engine) ImageSize(ctx context.Context, containerID string) (uint64, error)` that inspects the container for `Image`, rejects an empty image ID, then inspects `/images/{escaped ID}/json` and returns `Size`.

- [ ] **Step 4: Run test to verify it passes**

Run `go test -count=1 ./internal/docker -run TestImageSize -v`; expect PASS.

### Task 2: Current snapshot and overview contract

**Files:**
- Modify: `internal/metrics/sampler.go`
- Test: `internal/metrics/sampler_test.go`
- Modify: `internal/httpapi/server.go`
- Test: `internal/httpapi/server_test.go`
- Modify: `cmd/e2e-fixture/main.go`

**Why this task exists:** The current observation must expose the adapter value without expanding the historical chart contract.

**Impact / Compatibility:** Adds nullable `image_size_bytes` to current snapshots and overview JSON. The performance-history projection must continue omitting it.

**Repair Track:** The duplicated player metric is replaced at its display consumer, while player sampling remains canonical for lifecycle health and player views.

**Retirement Track:** Only the performance panel's player display retires. `players` and `max_players` stay active in overview cards, A2S health, and player management.

**Verification:** `go test -count=1 ./internal/metrics ./internal/httpapi ./cmd/e2e-fixture`

- [ ] **Step 1: Write failing sampler and API tests**

Extend the fake runtime with `ImageSize`, assert a successful sample stores `ImageSizeBytes`, assert lookup failure records source `docker_image`, and assert overview JSON includes `image_size_bytes` while history JSON still excludes it.

- [ ] **Step 2: Run tests to verify they fail**

Run `go test -count=1 ./internal/metrics ./internal/httpapi ./cmd/e2e-fixture`; expect compile/assertion failures for the missing method and field.

- [ ] **Step 3: Implement minimal propagation**

Extend `RuntimeProvider`, `Snapshot`, cloning/merging, `sampleStats`, `instanceOverview`, and `overviewFromSnapshot` with `ImageSizeBytes *uint64`. Query image size alongside container stats and record `issue("docker_image", err)` on failure. Update the e2e fixture runtime method.

- [ ] **Step 4: Run tests to verify they pass**

Run `go test -count=1 ./internal/metrics ./internal/httpapi ./cmd/e2e-fixture`; expect PASS.

### Task 3: Performance panel presentation

**Files:**
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/PerformancePanel.tsx`
- Test: `web/src/app/PerformancePanel.test.tsx`
- Test: `web/src/app/App.test.tsx`

**Why this task exists:** Administrators should see useful storage information and understand network direction without Docker terminology.

**Impact / Compatibility:** Changes current metric text and network labels only; chart values and history loading remain unchanged.

**Verification:** `npm test -- --run src/app/PerformancePanel.test.tsx src/app/App.test.tsx` from `web`.

- [ ] **Step 1: Write failing UI tests**

Add `image_size_bytes: 5 * 1024 ** 3` to the snapshot fixture. Assert “镜像大小” and “5 GiB” render, “玩家” does not render inside the performance panel, and network mode renders “下载” and “上传” without “网络 RX” or “网络 TX”. Add the overview mapping assertion for `image_size_bytes`.

- [ ] **Step 2: Run tests to verify they fail**

Run `npm test -- --run src/app/PerformancePanel.test.tsx src/app/App.test.tsx`; expect failures for missing image size and old labels.

- [ ] **Step 3: Implement minimal UI change**

Add nullable `image_size_bytes` to `Instance`, `InstanceOverview`, and `PerformanceSnapshot`; propagate it in overview polling and panel props. Replace the player metric with `<Metric label="镜像大小" value={formatBytes(snapshot.image_size_bytes)} />`. Rename the network series labels to “下载” and “上传”.

- [ ] **Step 4: Run focused UI tests and build**

Run `npm test -- --run src/app/PerformancePanel.test.tsx src/app/App.test.tsx` and `npm run build`; expect PASS.

### Task 4: Regression and journey verification

**Files:**
- Modify only if a directly related failure proves a defect in this change.

**Why this task exists:** The cross-layer contract must work together and preserve existing history and player workflows.

**Impact / Compatibility:** No scope expansion; unrelated Windows temporary-file flakes are reported rather than masked.

**Verification:** Full test/build commands and overview visual smoke check.

- [ ] **Step 1: Run backend verification**

Run `go test -p 1 -count=1 ./...` and `go vet ./...`; expect no product failures.

- [ ] **Step 2: Run frontend verification**

From `web`, run `npm test -- --run` and `npm run build`; expect PASS.

- [ ] **Step 3: Verify the main user journey**

Run the app or existing e2e fixture, open the overview, and confirm the performance current row displays formatted “镜像大小”; switch to network mode and confirm “下载/上传” legend and tooltip labels while the chart remains populated.

- [ ] **Step 4: Commit implementation**

Stage only implementation/test files and commit with `feat(metrics): 显示实例镜像大小并简化网络文案`.

## Risks and rollback

- Docker image inspect adds one local API request per running instance per five-second sample. If this proves material, cache by image ID in a later measured optimization; no cache is added preemptively.
- Rollback is the implementation commit only. The additive JSON field does not require data migration.
- Unknown: visual verification depends on the local fixture exposing a representative image size; automated contract and component tests remain authoritative for null and formatted states.
