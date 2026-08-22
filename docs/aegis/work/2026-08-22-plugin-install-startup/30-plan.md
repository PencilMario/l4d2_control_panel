# Plugin Install and Startup Repair Plan

**Goal:** Make plugin package installation and existing-instance startup work after root-owned package creation and OverlayFS unmounts.

**Architecture:** `panel-init` owns only package-tree ownership repair. `provisioning.Service` remains the canonical shared release and Overlay owner, exposing a per-instance ensure operation used by lifecycle startup. Panel process startup continues to recover all persisted overlays before reconciliation.

**Compatibility boundary:** Preserve the data layout, package semantics, container labels, desired-state behavior, overlay-helper privilege boundary, and public APIs.

**Verification:** RED/GREEN focused Go tests, `go test -count=1 -p 1 ./...`, `go vet ./...`, Compose configuration validation, image build, remote package readability, background plugin job, Overlay mount inspection, and instance start logs.

## Repair track

- Fix the root cause at the Compose initialization owner rather than changing package readers.
- Reuse the existing shared release resolver for both process-wide recovery and per-instance starts.
- Call the per-instance ensure operation before Accelerator and container start.

## Retirement track

- The manual remote `chown` is a one-time repair for existing files; future deploys use the scoped `panel-init` repair.
- The old path that starts an existing container without an Overlay ensure retires from the lifecycle start path.
- The startup `game/current` fallback remains only for failed/legacy shared-state recovery and is not a second release owner.
