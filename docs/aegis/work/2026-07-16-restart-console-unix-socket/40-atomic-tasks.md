# Atomic Task Status

- [x] RED: Unix Socket supervisor attach fails through the current TCP-only hijack path.
- [x] GREEN: Unix and TCP supervisor attach both use the configured transport successfully.
- [x] RED: Docker 304 stop aborts restart and persists faulted state.
- [x] GREEN: Already-stopped is idempotent for Stop and restart reaches running state.
- [x] REGRESSION: Docker, lifecycle, HTTP API, full Go, frontend unit/build, desktop E2E, and mobile E2E pass.
- [x] CLEANUP: No temporary instrumentation, listeners, generated files, or fallback paths remain.
