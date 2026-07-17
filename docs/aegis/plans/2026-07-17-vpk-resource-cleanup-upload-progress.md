# VPK Resource Cleanup and Upload Progress Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:executing-plans to implement this plan task-by-task.

**Goal:** Add safe server-resource cleanup for uploaded shared VPKs and show byte progress plus transfer speed during uploads.

**Architecture:** Add a content-layer cleanup method that extracts a VPK into a temporary directory, removes the established server-side-unneeded extensions, repacks and atomically replaces the source. Expose it through an authenticated HTTP action and a VPK-list UI button. Extend the existing chunk uploader's status calculation with elapsed-byte throughput.

**Tech Stack:** Go, existing VPK/archive helpers, Chi HTTP API, React/TypeScript, Vitest.

**Baseline / Authority Refs:** `internal/content/uploads.go`, `internal/httpapi/server.go`, `web/src/app/PrivateFilesPage.tsx`, existing upload and VPK tests.

**Compatibility Boundary:** Existing VPK upload/download/rename/delete routes and package/instance references remain stable; failed cleanup leaves the original file untouched; upload resume protocol remains unchanged.

**Verification:** Go content/API tests, frontend component tests, `go test ./...`, and `npm test -- --run` in `web`.

---

### Task 1: Add VPK cleanup owner and tests

**Files:** Modify `internal/content/uploads.go`; test `internal/content/uploads_test.go`.

- Write a failing test that builds a small VPK containing `maps/test.bsp`, `materials/test.vtf`, `sound/test.wav`, `edit.vmf`, and `cfg/server.cfg`, calls cleanup, then asserts only server-needed files remain and the source was replaced.
- Add cleanup with a temporary extraction directory, extension filter, repack to a temporary output beside the source, sync/close, and atomic replacement using the existing platform helper.
- Return before/after sizes and removed count; reject non-VPK paths and preserve the source on extraction/repack failure.

### Task 2: Expose authenticated cleanup action

**Files:** Modify `internal/httpapi/server.go`; test `internal/httpapi/server_test.go`.

- Add `POST /api/content/vpk/{name}/clean`, requiring the existing confirmation convention.
- Resolve the escaped VPK name through the existing upload manager, invoke cleanup, and return JSON statistics; map missing files, invalid VPKs, and cleanup failures to stable error responses.
- Add tests for confirmation, success replacement, and failure preservation.

### Task 3: Add UI action and upload byte/speed status

**Files:** Modify VPK list component and upload component in `web/src/app/PrivateFilesPage.tsx` (or the current VPK-owning component), plus its tests.

- Add a confirmed cleanup button to each shared VPK row and refresh the list after success.
- During chunk upload calculate elapsed time and display uploaded bytes, total bytes, percentage, and MiB/s; retain resume behavior and reset timing when a new upload starts.
- Add tests for cleanup request/confirmation and visible byte/speed status.

### Task 4: Regression verification

- Run focused Go and frontend tests, then full Go and frontend suites.
- Check formatting and report any environment-blocked browser/e2e verification separately.
