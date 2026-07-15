# Baseline read set

- Approved specification: `docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md`
- Implementation plan: `docs/aegis/plans/2026-07-15-private-file-manager-console-follow.md`
- Parent architecture: `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`
- Current owners: `internal/content/private.go`, `internal/content/overlay.go`, `internal/updates/pipeline.go`, `internal/httpapi/server.go`, `web/src/app/App.tsx`
- Verification owners: Go package tests, Vitest/Testing Library, Playwright desktop/mobile projects.

Compatibility boundary: preserve existing private paths, package/shared/private priority, per-instance Job serialization, update rollback, authenticated API conventions, PTY WebSocket protocol, and unrelated content flows.
