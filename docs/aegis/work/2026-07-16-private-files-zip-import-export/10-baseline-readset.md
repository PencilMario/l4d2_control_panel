# Baseline Read Set

Updated: 2026-07-16 +08:00

- `CONTEXT.md`: canonical game-instance terminology.
- `docs/aegis/specs/2026-07-16-private-files-zip-import-export-design.md`: approved replacement, archive-root and manual-apply semantics.
- `docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md`: workspace, manifest, snapshot and apply ownership.
- `internal/content/private.go`: shared instance lock and CRUD path safety.
- `internal/content/private_state.go`: manifest, diff, journaled apply/restore and startup recovery.
- `internal/content/private_uploads.go`: 2 GiB private upload ceiling and shared-lock behavior.
- `internal/archive/inspect.go`: existing package ZIP limits; common-root stripping is explicitly not reusable for private ZIPs.
- `internal/httpapi/server.go`: Chi routing, confirmation, stable errors and mutation audit.
- `web/src/app/PrivateFilesPage.tsx`: staged workspace UI, busy state, reload ownership and existing toolbar.
- Corresponding Go, Vitest and Playwright tests: executable compatibility baseline.

Baseline evidence:

- `go test ./... -count=1` initially hit the documented Windows temporary-file lock in `TestPrivateFileAPIContract`; all other packages passed.
- `go test ./internal/httpapi -run '^TestPrivateFileAPIContract$' -count=5` passed.
- With a dedicated `GOTMPDIR`, `go test -p 1 ./... -count=1` passed all packages.
- `cd web && npm test -- --run` passed 5 files and 83 tests.
