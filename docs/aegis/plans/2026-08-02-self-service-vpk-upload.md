# Self-service VPK Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide an independently accessible `/uploadvpk` page that lists and uploads self-service VPKs with optional password authorization and configurable expiry.

**Architecture:** Persist settings and self-service provenance in SQLite, keep upload mechanics in the existing `content.UploadManager`, and expose a separately authorized public API from `httpapi.Server`. A focused cleanup service owns expiry deletion and an hourly scheduler; a standalone React route reuses the existing browser upload protocol through an endpoint adapter.

**Tech Stack:** Go, chi, SQLite, bcrypt, robfig/cron, React, TypeScript, Vitest, Testing Library, Playwright.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-08-02-self-service-vpk-upload-design.md`, `CONTEXT.md`, existing contracts in `internal/content/uploads.go`, `internal/httpapi/server.go`, `internal/store/job_history.go`, and `web/src/vpk/uploadQueue.ts`.

**Compatibility Boundary:** Existing authenticated shared-VPK routes, shared storage layout, administrator content UI, and game mounts remain unchanged. Public authorization grants only self-service list and upload operations; administrator-created VPKs never expire through this feature.

**Verification:** `go test ./internal/store ./internal/content ./internal/httpapi ./cmd/panel`, `go test ./...`, `npm test -- --run` and `npm run build` from `web`, followed by Playwright checks of `/uploadvpk` at desktop and mobile sizes.

---

### Task 1: Persist settings and self-service upload metadata

**Files:**
- Create: `internal/store/self_service_vpk.go`
- Modify: `internal/store/migrations.go`
- Test: `internal/store/store_test.go`

**Why this task exists:** Settings must survive restarts, passwords must never be stored in plaintext, and only self-service uploads may expire.

**Impact / Compatibility:** Additive schema and store methods only; existing `system_settings` keys and shared VPK files remain valid.

**Verification:** `go test ./internal/store -run 'SelfServiceVPK'`

- [ ] Write failing store tests for defaults, settings round-trip, bcrypt password verification/version increment, metadata CRUD, rename, size update, newest-first pagination, and expired selection.
- [ ] Run the focused tests and confirm failures are caused by missing store contracts.
- [ ] Add the metadata table migration and focused store types/methods, using transactions for settings updates and metadata mutations.
- [ ] Run focused and complete store tests until green.
- [ ] Commit `feat(store): 持久化自助传图设置与文件元数据`.

### Task 2: Add expiry cleanup ownership

**Files:**
- Create: `internal/content/self_service_vpk.go`
- Create: `internal/content/self_service_vpk_test.go`
- Modify: `internal/content/uploads.go`

**Why this task exists:** Expired self-service files require safe direct deletion, retryable metadata, and synchronization when administrators rename, clean, or delete files.

**Impact / Compatibility:** The existing `UploadManager` publication contract stays stable. Optional lifecycle hooks synchronize metadata without changing administrator responses.

**Verification:** `go test ./internal/content -run 'SelfService|Upload'`

- [ ] Write failing tests proving cleanup deletes strictly expired files regardless of instance references, preserves metadata after filesystem failure, pauses when disabled, and updates metadata after admin rename/clean/delete.
- [ ] Run tests and confirm expected behavioral failures.
- [ ] Implement a `SelfServiceVPKManager` with `Complete`, `List`, `CleanupExpired`, `Rename`, `UpdateSize`, and `Delete` ownership, plus narrow UploadManager hooks.
- [ ] Run focused and full content tests until green.
- [ ] Commit `feat(content): 管理自助 VPK 生命周期与过期清理`.

### Task 3: Expose settings, authorization, list, and upload APIs

**Files:**
- Create: `internal/httpapi/self_service_vpk.go`
- Modify: `internal/httpapi/server.go`
- Test: `internal/httpapi/server_test.go`

**Why this task exists:** Public clients need a least-privilege boundary distinct from administrator authentication while retaining resumable uploads and cleanup-before-publication.

**Impact / Compatibility:** New public endpoints are additive. Existing `/api/content/vpk/*` routes and session cookies retain their exact behavior.

**Verification:** `go test ./internal/httpapi -run 'SelfServiceVPK'`

- [ ] Write failing integration tests for disabled access, empty-password access, incorrect password, 24-hour scoped cookie, password-version revocation, list isolation/pagination, begin/recover/write/cancel/complete, server cleanup, collision rejection, and absence of administrator privileges.
- [ ] Run focused tests and confirm failures reflect missing routes and handlers.
- [ ] Add authenticated settings GET/PUT handlers and public `/api/self-service/vpk/*` handlers with a signed HttpOnly cookie distinct from `l4d2_panel_session`.
- [ ] Ensure every public operation rechecks enabled state and password version; publish metadata only after successful atomic VPK completion.
- [ ] Run focused and complete HTTP API tests until green.
- [ ] Commit `feat(api): 增加自助传图授权与上传接口`.

### Task 4: Start and stop hourly cleanup with the panel

**Files:**
- Create: `internal/content/self_service_vpk_scheduler.go`
- Create: `internal/content/self_service_vpk_scheduler_test.go`
- Modify: `cmd/panel/main.go`
- Modify: `cmd/panel/main_test.go`
- Modify: `cmd/e2e-fixture/main.go`

**Why this task exists:** Expiry must continue across restarts and retry failed deletions without relying on page traffic.

**Impact / Compatibility:** Scheduler lifecycle follows the existing game-log scheduler pattern and participates in graceful shutdown.

**Verification:** `go test ./internal/content ./cmd/panel`

- [ ] Write failing scheduler tests for hourly registration, idempotent start, blocking stop, disabled cleanup, and retry on the next run.
- [ ] Run tests and confirm missing scheduler behavior.
- [ ] Implement the cron-backed scheduler, wire it into panel and fixture construction, and stop it during shutdown.
- [ ] Run content, panel, fixture, and full Go tests until green.
- [ ] Commit `feat(vpk): 定时清理过期自助上传文件`.

### Task 5: Build endpoint-adaptable upload queue behavior

**Files:**
- Modify: `web/src/vpk/uploadQueue.ts`
- Modify: `web/src/vpk/uploadQueue.test.ts`

**Why this task exists:** The standalone page needs the proven chunking, hashing, recovery, and client/server cleanup logic without copying the queue implementation.

**Impact / Compatibility:** Default endpoint configuration remains the administrator API, so the content repository behavior does not change.

**Verification:** `npm test -- --run src/vpk/uploadQueue.test.ts`

- [ ] Write failing tests for a self-service endpoint adapter covering begin, recover, patch, complete, cancel, and cleanup URLs.
- [ ] Run the focused test and confirm the queue still hardcodes administrator endpoints.
- [ ] Inject an endpoint set and queue storage namespace while retaining existing defaults.
- [ ] Run queue tests until green.
- [ ] Commit `refactor(web): 复用 VPK 上传队列支持自助接口`.

### Task 6: Add the standalone `/uploadvpk` page and administrator settings

**Files:**
- Create: `web/src/app/SelfServiceVPKPage.tsx`
- Create: `web/src/app/SelfServiceVPKPage.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles/app.css`

**Why this task exists:** Visitors need a complete independent upload journey, while administrators need clear control over exposure and retention.

**Impact / Compatibility:** `/uploadvpk` bypasses the administrator shell but no other path does. Existing system-settings controls and content repository remain present.

**Verification:** `npm test -- --run src/app/SelfServiceVPKPage.test.tsx src/app/App.test.tsx && npm run build`

- [ ] Write failing component tests for disabled, password, empty-password, authorization-expired, empty/list/paginated, direct upload, cleanup upload, collision, and settings validation states.
- [ ] Run focused frontend tests and confirm the page and settings are absent.
- [ ] Implement route selection before the administrator app bootstrap, the self-service page, read-only list, password form, upload queue adapter, and settings controls.
- [ ] Add restrained responsive styling with stable queue/list dimensions and accessible form/status semantics.
- [ ] Run focused and full frontend tests plus the production build until green.
- [ ] Commit `feat(web): 增加独立自助传图页面与设置`.

### Task 7: End-to-end verification and documentation evidence

**Files:**
- Modify: `docs/aegis/INDEX.md`
- Create: `docs/aegis/work/2026-08-02-self-service-vpk-upload/50-evidence.md`
- Test: existing Go and web suites

**Why this task exists:** The user-visible path crosses persistence, authorization, upload, cleanup, and responsive UI boundaries.

**Impact / Compatibility:** Verification must exercise both the public journey and the unchanged administrator flow.

**Verification:** Full commands listed in the plan header plus Playwright screenshots and interaction checks.

- [ ] Run `gofmt` on changed Go files and `go test ./...`.
- [ ] Run `npm test -- --run` and `npm run build` in `web`.
- [ ] Start the local fixture/server on an unused port and verify public mode, password mode, both upload modes, list refresh, collision rejection, and administrator visibility with Playwright at desktop and mobile sizes.
- [ ] Record commands, outputs, screenshots, and any residual risk in the evidence document; add the plan to the Aegis index.
- [ ] Inspect `git diff --check` and the final focused diff.
- [ ] Commit `test(vpk): 记录自助传图端到端验证证据`.

## Risks and rollback

- An empty password intentionally exposes upload capacity; settings copy must state this directly.
- File publication and metadata persistence cross filesystem and SQLite boundaries. If metadata insertion fails, completion must compensate by removing the newly published file or return an explicit repairable failure without silently creating an immortal self-service file.
- Automatic deletion is intentionally reference-blind. Rollback consists of disabling automatic deletion; deleted files require restoration from an external backup.
- The public cookie must be signed, HttpOnly, SameSite strict, and secure whenever the administrator cookie is secure. Password version checks provide revocation without server-side public sessions.
- Existing administrator routes, upload queue defaults, and shared VPK files are retained; no old owner is retired. Only endpoint hardcoding in the browser queue converges into configurable defaults.
